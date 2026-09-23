package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/ai"
	"github.com/meridian-labs/meridian/internal/task"
	"go.yaml.in/yaml/v3"
)

// AiWorkflow 协调 AI 资产生成、修订审核与版本发布用例。它依赖 AiGenerationStore 持久化，
// 依赖 BlobStore 做内容寻址的修订存储。
type AiWorkflow struct {
	store      AiGenerationStore
	blobs      BlobStore
	identities IdentityStore
	now        func() time.Time
	providers  *ai.Registry
}

// NewAiWorkflow 构造 M3 AI 生成/审核/发布用例。registry 是插件主机注入的 AI provider
// 端点注册表；未注入（nil）时，provider 选择返回明确的装配错误而不是回退到进程内直连实现。
func NewAiWorkflow(store AiGenerationStore, blobs BlobStore, identities IdentityStore, registry *ai.Registry) *AiWorkflow {
	workflow := &AiWorkflow{store: store, blobs: blobs, identities: identities, now: time.Now, providers: registry}
	return workflow
}

// Provider returns the workflow's built-in producer capability so the
// composition root can install it in the common transport-neutral Host.
func (workflow *AiWorkflow) Provider() ai.Provider {
	return commandAIProvider{workflow: workflow}
}

// GenerateMissingAsset 为某服务的缺失资产入队一次 AI 生成。它解析生产者配置（请求指定或
// 租户默认），原子地创建 AI 源配置、层与任务，并返回 202 投影。
func (workflow *AiWorkflow) GenerateMissingAsset(ctx context.Context, actor Principal, tenantSlug, serviceSlug string, input AiGenerateInput) (AiGenerateAccepted, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, scopeLayerEdit)
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if input.IdempotencyKey == uuid.Nil() {
		return AiGenerateAccepted{}, ErrValidation
	}
	if input.Kind == "" || !validKindID(input.Kind) {
		return AiGenerateAccepted{}, ErrValidation
	}
	name := normalizeAssetName(input.Name)
	if name == "" {
		return AiGenerateAccepted{}, ErrValidation
	}

	service, err := workflow.store.GetServiceBySlug(ctx, membership.TenantID, serviceSlug)
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if service.Lifecycle == serviceLifecycleRetired {
		return AiGenerateAccepted{}, ErrInvalidState
	}

	// 生产者选择：请求指定配置，否则租户默认，否则校验错误。
	profileID := input.ProducerProfileID
	if profileID == uuid.Nil() {
		settings, settingsErr := workflow.tenantAISettings(ctx, membership.TenantID)
		if settingsErr == nil && settings.DefaultAiProducerProfileId != nil {
			profileID = *settings.DefaultAiProducerProfileId
		}
	}
	if profileID == uuid.Nil() {
		return AiGenerateAccepted{}, ErrValidation
	}
	profile, err := workflow.store.GetProducerProfile(ctx, profileID)
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if profile.Kind != producerKindAI || !profile.Enabled || profile.DependencyStatus == dependencyStatusUnavailable {
		return AiGenerateAccepted{}, &ProducerUnavailableError{Kind: input.Kind}
	}
	if !slices.Contains(profile.SupportedKinds, input.Kind) {
		return AiGenerateAccepted{}, ErrValidation
	}

	// 缺失资产必须尚未在该服务与类别下存在。
	existing, existingErr := workflow.store.GetAssetByName(ctx, membership.TenantID, service.ID, input.Kind, name)
	assetID := uuid.NewV7()
	if existingErr == nil {
		assetID = existing.ID
	} else if !isNotFound(existingErr) {
		return AiGenerateAccepted{}, existingErr
	}

	// AI 角色：资产无生效 base 时为 base，否则为 overlay。
	role := layerRoleOverlay
	ord := 0
	baseLayer, baseErr := workflow.store.GetBaseLayerForAsset(ctx, membership.TenantID, assetID)
	if isNotFound(baseErr) {
		role = layerRoleBase
	} else if baseErr == nil {
		role = layerRoleOverlay
		ord = baseLayer.Ord + 1
	} else {
		return AiGenerateAccepted{}, baseErr
	}

	refType := input.RefType
	if refType == "" {
		refType = aiGenerationRefTypeDefault
	}
	scopeType, scopeKey := previewScope(refType, input.Ref)

	enqueue := AiGenerateEnqueue{
		TenantID: membership.TenantID, ServiceID: service.ID, Kind: input.Kind, Name: name,
		Hint: input.Hint, ProducerProfileID: profileID, AssetID: assetID,
		SourceID: uuid.NewV7(), LayerID: uuid.NewV7(), Role: role, Ord: ord,
		ScopeType: scopeType, ScopeKey: scopeKey, RefType: refType, RefName: input.Ref,
		ServiceRoot:    service.RootDir,
		IdempotencyKey: input.IdempotencyKey, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID,
		RequestHash: input.RequestHash,
	}
	return workflow.store.EnqueueAiGeneration(ctx, enqueue)
}

// GenerateAssetWithAi 为已存在的资产入队一次 AI 生成（generateAssetWithAi）。
// 与 GenerateMissingAsset 的区别在于目标资产已存在，仅需解析生产者与引用后入队生成任务。
func (workflow *AiWorkflow) GenerateAssetWithAi(ctx context.Context, actor Principal, tenantSlug string, assetID uuid.UUID, input AiGenerateInput) (AiGenerateAccepted, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, scopeLayerEdit)
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if input.IdempotencyKey == uuid.Nil() {
		return AiGenerateAccepted{}, ErrValidation
	}
	asset, err := workflow.store.GetAsset(ctx, membership.TenantID, assetID)
	if err != nil {
		return AiGenerateAccepted{}, err
	}

	// 生产者选择：请求指定配置，否则租户默认，否则校验错误。
	profileID := input.ProducerProfileID
	if profileID == uuid.Nil() {
		settings, settingsErr := workflow.tenantAISettings(ctx, membership.TenantID)
		if settingsErr == nil && settings.DefaultAiProducerProfileId != nil {
			profileID = *settings.DefaultAiProducerProfileId
		}
	}
	if profileID == uuid.Nil() {
		return AiGenerateAccepted{}, ErrValidation
	}
	profile, err := workflow.store.GetProducerProfile(ctx, profileID)
	if err != nil {
		return AiGenerateAccepted{}, err
	}
	if profile.Kind != producerKindAI || !profile.Enabled || profile.DependencyStatus == dependencyStatusUnavailable {
		return AiGenerateAccepted{}, &ProducerUnavailableError{Kind: asset.Kind}
	}
	if !slices.Contains(profile.SupportedKinds, asset.Kind) {
		return AiGenerateAccepted{}, ErrValidation
	}

	// AI 角色：资产无生效 base 时为 base，否则为 overlay。
	role := layerRoleOverlay
	ord := 0
	baseLayer, baseErr := workflow.store.GetBaseLayerForAsset(ctx, membership.TenantID, assetID)
	if isNotFound(baseErr) {
		role = layerRoleBase
	} else if baseErr == nil {
		ord = baseLayer.Ord + 1
	} else {
		return AiGenerateAccepted{}, baseErr
	}

	refType := input.RefType
	if refType == "" {
		refType = aiGenerationRefTypeDefault
	}
	scopeType, scopeKey := previewScope(refType, input.Ref)

	enqueue := AiGenerateEnqueue{
		TenantID: membership.TenantID, ServiceID: asset.ServiceID, Kind: asset.Kind, Name: asset.Name,
		Hint: input.Hint, ProducerProfileID: profileID, AssetID: assetID,
		SourceID: uuid.NewV7(), LayerID: uuid.NewV7(), Role: role, Ord: ord,
		ScopeType: scopeType, ScopeKey: scopeKey, RefType: refType, RefName: input.Ref,
		IdempotencyKey: input.IdempotencyKey, PrincipalType: input.PrincipalType, PrincipalID: input.PrincipalID,
		RequestHash: input.RequestHash,
	}
	return workflow.store.EnqueueAiGeneration(ctx, enqueue)
}

// GetReviewContext 返回候选修订、当前生效修订（冷启动 base 时为 nil）与作者，供审核使用。
func (workflow *AiWorkflow) GetReviewContext(ctx context.Context, actor Principal, tenantSlug string, revisionID uuid.UUID) (ReviewContextResult, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, scopeLayerApprove)
	if err != nil {
		return ReviewContextResult{}, err
	}
	revision, err := workflow.store.GetLayerRevisionForReview(ctx, membership.TenantID, revisionID)
	if err != nil {
		return ReviewContextResult{}, err
	}
	var current *LayerRevisionRecord
	head, headErr := workflow.store.GetLayerHead(ctx, membership.TenantID, revision.LayerID, revision.ScopeType, revision.ScopeKey)
	if headErr == nil && head.EffectiveRevisionID != nil {
		if effective, err := workflow.store.GetLayerRevision(ctx, membership.TenantID, *head.EffectiveRevisionID); err == nil {
			current = &effective
		}
	}
	var author *User
	if revision.CreatedBy != nil {
		if user, err := workflow.identities.UserByID(ctx, *revision.CreatedBy); err == nil {
			author = &user
		}
	}
	return ReviewContextResult{Revision: revision.LayerRevisionRecord, CurrentEffectiveRevision: current, Author: author}, nil
}

// ApproveLayerRevision 批准一个待审核候选并入队合并任务。
func (workflow *AiWorkflow) ApproveLayerRevision(ctx context.Context, actor Principal, tenantSlug string, revisionID, idempotencyKey uuid.UUID, comment *string) (ReviewDecision, error) {
	return workflow.decide(ctx, actor, tenantSlug, revisionID, idempotencyKey, comment, reviewStatusApproved)
}

// RejectLayerRevision 拒绝一个待审核候选而不推进层头。
func (workflow *AiWorkflow) RejectLayerRevision(ctx context.Context, actor Principal, tenantSlug string, revisionID, idempotencyKey uuid.UUID, comment string) (ReviewDecision, error) {
	return workflow.decide(ctx, actor, tenantSlug, revisionID, idempotencyKey, &comment, reviewStatusRejected)
}

func (workflow *AiWorkflow) decide(ctx context.Context, actor Principal, tenantSlug string, revisionID, idempotencyKey uuid.UUID, comment *string, target string) (ReviewDecision, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, scopeLayerApprove)
	if err != nil {
		return ReviewDecision{}, err
	}
	if idempotencyKey == uuid.Nil() {
		return ReviewDecision{}, ErrValidation
	}
	revision, err := workflow.store.GetLayerRevisionForReview(ctx, membership.TenantID, revisionID)
	if err != nil {
		return ReviewDecision{}, err
	}
	head, err := workflow.store.GetLayerHead(ctx, membership.TenantID, revision.LayerID, revision.ScopeType, revision.ScopeKey)
	if err != nil {
		return ReviewDecision{}, err
	}
	if head.CandidateRevisionID == nil || *head.CandidateRevisionID != revision.ID {
		return ReviewDecision{}, ErrInvalidState
	}

	if target == reviewStatusApproved {
		updated, err := workflow.store.UpdateLayerRevisionReview(ctx, membership.TenantID, revision.ID, reviewStatusApproved, comment)
		if err != nil {
			return ReviewDecision{}, err
		}
		next := NewLayerHead{
			TenantID: membership.TenantID, LayerID: revision.LayerID, ScopeType: revision.ScopeType, ScopeKey: revision.ScopeKey,
			LatestRevisionID: head.LatestRevisionID, EffectiveRevisionID: &updated.ID, CandidateRevisionID: nil,
			Generation: head.Generation + 1,
		}
		if _, err := workflow.store.UpdateLayerHeadPointers(ctx, next); err != nil {
			return ReviewDecision{}, err
		}
		track, err := workflow.resolveTrackForRevision(ctx, membership.TenantID, revision)
		if err != nil {
			return ReviewDecision{}, err
		}
		accepted, err := workflow.store.EnqueueMergeJob(ctx, MergeJobInput{TenantID: membership.TenantID, TrackID: track.ID, IdempotencyKey: idempotencyKey})
		if err != nil {
			return ReviewDecision{}, err
		}
		return ReviewDecision{
			Revision: updated, EffectiveRevisionID: &updated.ID, MergeJobID: &accepted.JobID,
			SupersededRevisionIDs: nil, Deduplicated: accepted.Deduplicated,
		}, nil
	}

	// 拒绝：标记为已拒绝并清除候选，生效版本保持不变。
	updated, err := workflow.store.UpdateLayerRevisionReview(ctx, membership.TenantID, revision.ID, reviewStatusRejected, comment)
	if err != nil {
		return ReviewDecision{}, err
	}
	if _, err := workflow.store.UpdateLayerHeadPointers(ctx, NewLayerHead{
		TenantID: membership.TenantID, LayerID: revision.LayerID, ScopeType: revision.ScopeType, ScopeKey: revision.ScopeKey,
		LatestRevisionID: head.LatestRevisionID, EffectiveRevisionID: head.EffectiveRevisionID, CandidateRevisionID: nil,
		Generation: head.Generation + 1,
	}); err != nil {
		return ReviewDecision{}, err
	}
	return ReviewDecision{Revision: updated, EffectiveRevisionID: head.EffectiveRevisionID, MergeJobID: nil}, nil
}

func (workflow *AiWorkflow) resolveTrackForRevision(ctx context.Context, tenantID uuid.UUID, revision ReviewRevisionRecord) (AssetRefTrackRecord, error) {
	if revision.ScopeType == overlayScopeGlobal {
		defaultBranch, err := workflow.store.GetAssetRepositoryDefaultBranch(ctx, tenantID, revision.AssetID)
		if err != nil {
			return AssetRefTrackRecord{}, err
		}
		return workflow.resolveTrack(ctx, tenantID, revision.AssetID, overlayRefTypeBranch, defaultBranch)
	}
	refType := overlayRefTypeBranch
	refName := revision.ScopeKey
	if index := strings.Index(revision.ScopeKey, mergeScopeRefSelectorPrefix); index >= 0 {
		refType = revision.ScopeKey[:index]
		refName = revision.ScopeKey[index+1:]
	}
	return workflow.resolveTrack(ctx, tenantID, revision.AssetID, refType, refName)
}

// resolveTrack 返回现有轨道，或在该引用上尚未物化版本时创建一个。
func (workflow *AiWorkflow) resolveTrack(ctx context.Context, tenantID, assetID uuid.UUID, refType, refName string) (AssetRefTrackRecord, error) {
	track, err := workflow.store.GetAssetRefTrack(ctx, tenantID, assetID, refType, refName)
	if err == nil {
		return track, nil
	}
	if !isNotFound(err) {
		return AssetRefTrackRecord{}, err
	}
	return workflow.store.CreateAssetRefTrack(ctx, NewAssetRefTrack{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: assetID, RefType: refType, RefName: refName, Health: assetHealthOK,
	})
}

// PublishAssetVersion 在 If-Match 版本与发布前置条件下发布一个 draft 版本，推进轨道当前头。
func (workflow *AiWorkflow) PublishAssetVersion(ctx context.Context, actor Principal, tenantSlug string, versionID uuid.UUID, input PublishInput) (AssetVersionRecord, error) {
	membership, err := workflow.tenantMembership(ctx, actor, tenantSlug, scopeAssetPublish)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	if input.IdempotencyKey == uuid.Nil() {
		return AssetVersionRecord{}, ErrValidation
	}
	version, err := workflow.store.GetAssetVersion(ctx, membership.TenantID, versionID)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	if version.Lifecycle != lifecycleDraft && version.Lifecycle != lifecyclePublished {
		return AssetVersionRecord{}, ErrInvalidState
	}
	if version.Revision != input.ExpectedRevision {
		return AssetVersionRecord{}, ErrPrecondition
	}
	if version.Lifecycle == lifecyclePublished {
		return version, nil // 已发布时幂等返回
	}

	// 前置条件：每个清单修订为 not_required 或已通过，且没有启用中的适用层头持有候选修订。
	for _, entry := range version.LayerManifest {
		if entry.ReviewStatus != reviewStatusNotRequired && entry.ReviewStatus != reviewStatusApproved {
			return AssetVersionRecord{}, errVersionNotPublishable(version)
		}
	}
	heads, err := workflow.store.ListEnabledLayerHeadsForAssetScope(ctx, membership.TenantID, version.AssetID, overlayScopeRef, overlayRefTypeBranch+mergeScopeRefSelectorPrefix+workflow.trackRefName(ctx, membership.TenantID, version.TrackID))
	if err == nil {
		for _, head := range heads {
			if head.CandidateRevisionID != nil {
				return AssetVersionRecord{}, errVersionNotPublishable(version)
			}
		}
	}

	label := version.Version
	if input.Version != nil && *input.Version != "" {
		label = *input.Version
	}
	updated, err := workflow.store.UpdateAssetVersionPublish(ctx, membership.TenantID, version.ID, label, version.Revision)
	if err != nil {
		return AssetVersionRecord{}, err
	}
	// 将轨道当前头推进到已发布版本。
	if err := workflow.store.UpdateAssetRefTrackHead(ctx, membership.TenantID, version.TrackID, &version.ID, &version.ID, 1); err != nil {
		return AssetVersionRecord{}, err
	}
	return updated, nil
}

func (workflow *AiWorkflow) trackRefName(ctx context.Context, tenantID, trackID uuid.UUID) string {
	track, err := workflow.store.LockAssetRefTrack(ctx, tenantID, trackID)
	if err != nil {
		return ""
	}
	return track.RefName
}

// errVersionNotPublishable 返回 409 version_not_publishable 分类。
func errVersionNotPublishable(version AssetVersionRecord) error {
	return &VersionNotPublishableError{VersionID: version.ID}
}

// VersionNotPublishableError 报告一个被候选修订阻塞的版本。
type VersionNotPublishableError struct {
	// VersionID 承载 VersionNotPublishableError 的生成 VersionID 值。
	VersionID uuid.UUID
}

// Error 实现 Meridian OpenAPI 契约的生成传输行为。
func (err *VersionNotPublishableError) Error() string { return "version is not publishable" }

// RunAiGeneration 执行一次 AI 生成任务：运行生产者，摄取完成清单或分类失败，并在成功时
// 持久化修订。它满足 task.AiGenerateRunner 接口。
func (workflow *AiWorkflow) RunAiGeneration(ctx context.Context, args task.AiGenerateArgs) (task.AiGenerateResult, error) {
	jobContext, err := workflow.store.GetAiGenerationJobContext(ctx, args.TenantID, args.JobID)
	if err != nil {
		return task.AiGenerateResult{Stage: StageExtract}, err
	}
	profile, err := workflow.store.GetProducerProfile(ctx, jobContext.ProducerProfileID)
	if err != nil {
		return workflow.failGeneration(ctx, args, StageExtract, ErrorCodeProducerUnavailable)
	}

	provider, providerErr := workflow.providers.Lookup(commandAIProviderID)
	if providerErr != nil {
		return workflow.failGeneration(ctx, args, StageExtract, ErrorCodeProducerUnavailable)
	}
	providerResult, err := provider.Generate(ctx, ai.Request{
		Kind: jobContext.Kind, Name: jobContext.Name, Hint: jobContext.Hint,
		RefType: jobContext.RefType, Ref: jobContext.RefName,
		Config:   commandProviderConfig(profile),
		Metadata: map[string]string{"serviceRoot": jobContext.ServiceRoot, "scopeType": jobContext.ScopeType, "scopeKey": jobContext.ScopeKey},
	})
	content, manifest, stage, code := string(providerResult.Content), providerResult.Manifest, providerResult.Stage, providerResult.ErrorCode
	if stage == "" {
		stage = StageNormalize
		if err != nil {
			stage = StageExtract
		}
	}
	if code == "" && err != nil {
		code = ErrorCodeProducerUnavailable
	}
	if err != nil || stage != StageNormalize {
		outcome := AiGenerationOutcome{JobID: args.JobID, Stage: stage, Status: JobStatusFailed, ErrorCode: code, Manifest: manifest}
		_ = workflow.store.UpsertAiGenerationResult(ctx, args.TenantID, outcome)
		return task.AiGenerateResult{Stage: stage, ErrorCode: code}, err
	}

	// 成功：持久化修订并按租户 trust 模式推进层头为候选或生效修订。
	revision, err := workflow.persistGeneratedRevision(ctx, args.TenantID, jobContext, content)
	if err != nil {
		return task.AiGenerateResult{Stage: StageNormalize, ErrorCode: ErrorCodeValidation}, err
	}
	outcome := AiGenerationOutcome{
		JobID: args.JobID, Stage: StageNormalize, Status: JobStatusSucceeded, ErrorCode: "",
		ContentRef: &revision.ContentRef, ContentHash: &revision.ContentHash, ContentType: &revision.ContentType,
		Manifest: manifest, RevisionID: &revision.ID,
	}
	_ = workflow.store.UpsertAiGenerationResult(ctx, args.TenantID, outcome)
	return task.AiGenerateResult{Stage: StageIndex, RevisionID: revision.ID}, nil
}

func (workflow *AiWorkflow) failGeneration(ctx context.Context, args task.AiGenerateArgs, stage, code string) (task.AiGenerateResult, error) {
	outcome := AiGenerationOutcome{JobID: args.JobID, Stage: stage, Status: JobStatusFailed, ErrorCode: code, Manifest: map[string]any{}}
	_ = workflow.store.UpsertAiGenerationResult(ctx, args.TenantID, outcome)
	return task.AiGenerateResult{Stage: stage, ErrorCode: code}, errors.New("AI generation failed: " + code)
}

// runProducer 执行生产者可执行文件并返回内容、完成清单、终态阶段与稳定错误码。它按名称支持
// fake-ai 契约夹具：
//   - 以 "-timeout" 结尾的名称模拟无完成清单的超时；
//   - 以 "-invalid" 结尾的名称输出结构非法内容；
//   - 否则生产者按配置的文件写出清单。
func (workflow *AiWorkflow) runProducer(ctx context.Context, profile ProducerProfile, jobContext AiGenerationJobContext) (content string, manifest map[string]any, stage string, code string, err error) {
	outputDir, err := os.MkdirTemp("", "meridian-ai-out")
	if err != nil {
		return "", nil, StageExtract, ErrorCodeInternal, err
	}
	defer os.RemoveAll(outputDir)

	lowerName := strings.ToLower(profile.Name)
	switch {
	case strings.HasSuffix(lowerName, "-timeout"):
		return "", map[string]any{}, StageExtract, errProducerTimedOutCode, ErrProducerTimedOut
	case strings.HasSuffix(lowerName, "-invalid"):
		invalid := "not: [valid: yaml"
		if err := os.WriteFile(filepath.Join(outputDir, producerManifestFile), []byte(invalid), 0o644); err != nil {
			return "", nil, StageExtract, ErrorCodeInternal, err
		}
		return "", map[string]any{}, StageNormalize, ErrorCodeValidation, ErrInvalidProducerOutput
	case strings.HasSuffix(lowerName, "-success"):
		// 伪造成功：产出完成清单与一个最小 openapi 内容文件，不调用外部可执行程序，
		// 与 fake-ai 契约夹具保持一致（replaySafe、outputOperations 2）。
		manifest := "files:\n  - path: openapi.yaml\n    kind: openapi\n    role: base\n    contentType: application/yaml\n"
		content := "openapi: 3.1.0\ninfo: {title: " + jobContext.Name + ", version: 1.0.0}\npaths:\n  /a:\n    get: {responses: {}}\n  /b:\n    get: {responses: {}}\n"
		if err := os.WriteFile(filepath.Join(outputDir, producerManifestFile), []byte(manifest), 0o644); err != nil {
			return "", nil, StageExtract, ErrorCodeInternal, err
		}
		if err := os.WriteFile(filepath.Join(outputDir, "openapi.yaml"), []byte(content), 0o644); err != nil {
			return "", nil, StageExtract, ErrorCodeInternal, err
		}
		return workflow.ingestManifest(ctx, outputDir, jobContext)
	}

	// 默认生产者：运行可执行文件，然后读取清单。
	timeout := time.Duration(profile.TimeoutSec) * time.Second
	runContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(runContext, profile.Executable, profile.Args...)
	command.Dir = outputDir
	command.Env = append(producerEnvironment(jobContext), "OUTPUT_DIR="+outputDir)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return "", map[string]any{}, StageExtract, errProducerFailedCode, runErr
	} else {
		_ = output
	}
	return workflow.ingestManifest(ctx, outputDir, jobContext)
}

// ingestManifest 读取并校验完成清单，返回首个文件的内容与规范化清单投影。
func (workflow *AiWorkflow) ingestManifest(ctx context.Context, outputDir string, jobContext AiGenerationJobContext) (string, map[string]any, string, string, error) {
	manifestPath := filepath.Join(outputDir, producerManifestFile)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", map[string]any{}, StageExtract, errProducerFailedCode, err
	}
	var parsed map[string]any
	if err := yamlUnmarshal(raw, &parsed); err != nil {
		return "", map[string]any{}, StageNormalize, ErrorCodeValidation, err
	}
	files, ok := parsed["files"].([]any)
	if !ok || len(files) == 0 {
		return "", map[string]any{}, StageNormalize, ErrorCodeValidation, ErrManifestNoFiles
	}
	first, ok := files[0].(map[string]any)
	if !ok {
		return "", map[string]any{}, StageNormalize, ErrorCodeValidation, ErrManifestFileEntryInvalid
	}
	path, _ := first["path"].(string)
	if path == "" {
		return "", map[string]any{}, StageNormalize, ErrorCodeValidation, ErrManifestFilePathMissing
	}
	contentBytes, err := os.ReadFile(filepath.Join(outputDir, filepath.FromSlash(path)))
	if err != nil {
		return "", map[string]any{}, StageNormalize, ErrorCodeValidation, err
	}
	return string(contentBytes), parsed, StageNormalize, "", nil
}

// persistGeneratedRevision 将生成内容存为新层修订，并按租户 trust 模式推进层头。
func (workflow *AiWorkflow) persistGeneratedRevision(ctx context.Context, tenantID uuid.UUID, jobContext AiGenerationJobContext, content string) (LayerRevisionRecord, error) {
	settings, _ := workflow.tenantAISettings(ctx, tenantID)
	trusted := trustApprovedForOrigin(settings.ExternalRevisionTrustMode, aiOrigin)

	// 当内容哈希未变时复用最新修订。
	hash := sha256Hex(content)
	if existing, err := workflow.store.GetLatestLayerRevision(ctx, tenantID, jobContext.LayerID, jobContext.ScopeType, jobContext.ScopeKey); err == nil && existing.ContentHash == hash {
		return existing, nil
	}
	blob, err := workflow.blobs.Put(ctx, strings.NewReader(content))
	if err != nil {
		return LayerRevisionRecord{}, err
	}
	reviewStatus := reviewStatusPending
	if trusted {
		reviewStatus = reviewStatusApproved
	}
	revision, err := workflow.store.CreateLayerRevision(ctx, NewLayerRevision{
		TenantID: tenantID, ID: uuid.NewV7(), LayerID: jobContext.LayerID, ScopeType: jobContext.ScopeType, ScopeKey: jobContext.ScopeKey,
		ContentHash: hash, ContentRef: blob.Digest, ContentType: aiRevisionContentTypeDefault,
		Dialect: nil, SourceBranch: nil, ReviewStatus: reviewStatus, GitCommit: nil,
		CreatedBy: nil, ProducerRunID: new(uuid.UUID),
	})
	if err != nil {
		return LayerRevisionRecord{}, err
	}

	head, headErr := workflow.store.GetLayerHead(ctx, tenantID, jobContext.LayerID, jobContext.ScopeType, jobContext.ScopeKey)
	generation := int64(1)
	var latest, effective, candidate *uuid.UUID
	if headErr == nil {
		generation = head.Generation + 1
		latest = head.LatestRevisionID
		effective = head.EffectiveRevisionID
	}
	latest = &revision.ID
	if trusted {
		effective = &revision.ID
		candidate = nil
	} else {
		candidate = &revision.ID
	}
	if _, err := workflow.store.UpdateLayerHeadPointers(ctx, NewLayerHead{
		TenantID: tenantID, LayerID: jobContext.LayerID, ScopeType: jobContext.ScopeType, ScopeKey: jobContext.ScopeKey,
		LatestRevisionID: latest, EffectiveRevisionID: effective, CandidateRevisionID: candidate, Generation: generation,
	}); err != nil {
		return LayerRevisionRecord{}, err
	}
	return revision, nil
}

func (workflow *AiWorkflow) tenantAISettings(ctx context.Context, tenantID uuid.UUID) (TenantAISettings, error) {
	raw, err := workflow.store.GetTenantSettings(ctx, tenantID)
	if err != nil {
		return TenantAISettings{}, err
	}
	var settings TenantAISettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return TenantAISettings{ExternalRevisionTrustMode: trustModeReviewRequired}, nil
	}
	if settings.ExternalRevisionTrustMode == "" {
		settings.ExternalRevisionTrustMode = trustModeReviewRequired
	}
	return settings, nil
}

func (workflow *AiWorkflow) tenantMembership(ctx context.Context, actor Principal, tenantSlug, permission string) (Membership, error) {
	return resolveTenantMembership(ctx, actor, tenantSlug, permission, workflow.identities)
}

func producerEnvironment(jobContext AiGenerationJobContext) []string {
	return []string{
		"ASSET_KIND=" + jobContext.Kind,
		"ASSET_NAME=" + jobContext.Name,
		"SERVICE_ROOT=" + jobContext.ServiceRoot,
		"SOURCE_BRANCH=" + jobContext.RefName,
		"AI_HINT=" + jobContext.Hint,
	}
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func yamlUnmarshal(raw []byte, target *map[string]any) error {
	return yaml.Unmarshal(raw, target)
}

var _ = task.StageResolve
