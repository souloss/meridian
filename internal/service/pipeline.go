package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/kinds"
	"github.com/meridian-labs/meridian/internal/plugin"
	"github.com/meridian-labs/meridian/internal/storage"
	"github.com/meridian-labs/meridian/internal/task"
	"go.yaml.in/yaml/v3"
)

const (
	mergeEngineVersion       = "1.0.0"
	openapiPluginVersion     = "1"
	openapiReviewNotRequired = "not_required"
	openapiKind              = "openapi"
	initialVersionLabel      = "1.0.0"
)

// BlobStore 持久化层修订的不可变内容寻址字节。
type BlobStore interface {
	// Put 承载 BlobStore 的生成 Put 值。
	Put(context.Context, io.Reader) (storage.Blob, error)
	// Open 承载 BlobStore 的生成 Open 值。
	Open(string) (*os.File, storage.Blob, error)
}

// PipelineRunner 将仓库源物化为资产、层、修订、版本、条目与源绑定。它实现 task.SyncRunner。
type PipelineRunner struct {
	store     AssetStore
	blobs     BlobStore
	workspace string
	gitBinary string
	now       func() time.Time
	kinds     *kinds.Registry
}

// NewPipelineRunner 构造 M1 仓库同步管线。registry 是插件主机注入的 kind 能力端点注册表；
// 未注入（nil）时，任何 kind 能力调用都返回明确的装配错误，而不是回退到进程内直连实现。
func NewPipelineRunner(store AssetStore, blobs BlobStore, workspace string, registry *kinds.Registry) *PipelineRunner {
	return &PipelineRunner{store: store, blobs: blobs, workspace: workspace, gitBinary: "git", now: time.Now, kinds: registry}
}

// Run 跨六个管线阶段执行一次仓库同步。
func (runner *PipelineRunner) Run(ctx context.Context, args task.CredentialSyncArgs) (task.SyncResult, error) {
	if runner.store == nil {
		return task.SyncResult{}, errors.New("pipeline runner has no asset store")
	}
	checkout, commit, cleanup, err := runner.checkout(ctx, args)
	if err != nil {
		return task.SyncResult{}, err
	}
	defer cleanup()

	record, err := runner.store.GetRepository(ctx, args.TenantID, args.RepositoryID)
	if err != nil {
		return task.SyncResult{}, err
	}
	refType := refTypeBranch
	scopeKey := refType + ":" + args.RefName

	services, err := runner.store.ListServicesByRepository(ctx, args.TenantID, args.RepositoryID)
	if err != nil {
		return task.SyncResult{}, err
	}
	for _, service := range services {
		if err := runner.materializeService(ctx, args.TenantID, service, record.DefaultBranch, refType, args.RefName, scopeKey, commit, checkout); err != nil {
			return task.SyncResult{}, err
		}
	}
	return task.SyncResult{ResolvedCommit: commit}, nil
}

func (runner *PipelineRunner) checkout(ctx context.Context, args task.CredentialSyncArgs) (string, string, func(), error) {
	if runner.workspace == "" || !filepath.IsAbs(runner.workspace) {
		return "", "", func() {}, errors.New("pipeline workspace root is not an absolute path")
	}
	checkout := filepath.Join(runner.workspace, args.TenantID.String(), args.RepositoryID.String(), args.JobID.String())
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		return "", "", func() {}, fmt.Errorf("create pipeline workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(checkout) }
	cleanup()

	record, err := runner.store.GetRepository(ctx, args.TenantID, args.RepositoryID)
	if err != nil {
		return "", "", func() {}, err
	}
	if _, err := exec.LookPath(runner.gitBinary); err != nil {
		return "", "", cleanup, errors.New("git is not available for repository synchronization")
	}
	git := func(arguments ...string) error {
		command := exec.CommandContext(ctx, runner.gitBinary, arguments...)
		command.Env = append([]string(nil), os.Environ()...)
		command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0")
		if output, runErr := command.CombinedOutput(); runErr != nil {
			return fmt.Errorf("git %s: %w", strings.Join(arguments, " "), runErr)
		} else {
			_ = output
		}
		return nil
	}
	if err := git("clone", "--quiet", "--branch", args.RefName, record.URL, checkout); err != nil {
		return "", "", cleanup, err
	}
	command := exec.CommandContext(ctx, runner.gitBinary, "-C", checkout, "rev-parse", "HEAD")
	command.Env = append([]string(nil), os.Environ()...)
	output, err := command.Output()
	if err != nil {
		return "", "", cleanup, fmt.Errorf("resolve sync commit: %w", err)
	}
	return checkout, strings.TrimSpace(string(output)), cleanup, nil
}

// materializeService 同步某服务的全部 builtin 源配置。
func (runner *PipelineRunner) materializeService(ctx context.Context, tenantID uuid.UUID, service ServiceRecord, defaultBranch, refType, refName, scopeKey, commit, checkout string) error {
	specs, err := runner.store.ListSourceSpecsForService(ctx, tenantID, service.ID)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		if spec.Mode != "builtin" || !spec.Enabled {
			continue
		}
		if err := runner.materializeSource(ctx, tenantID, service, spec, defaultBranch, refType, refName, scopeKey, commit, checkout); err != nil {
			return err
		}
	}
	return nil
}

// materializeSource 对某 builtin 源配置路径做 glob，并为每个匹配文件物化一个资产，
// 然后原子提交解析出的作用域。
func (runner *PipelineRunner) materializeSource(ctx context.Context, tenantID uuid.UUID, service ServiceRecord, spec SourceSpecRecord, defaultBranch, refType, refName, scopeKey, commit, checkout string) error {
	pattern := ""
	if spec.Path != nil {
		pattern = *spec.Path
	}
	matches, err := runner.globMatches(checkout, pattern)
	if err != nil {
		return err
	}
	sort.Strings(matches)

	if len(matches) == 0 {
		// 路径无匹配文件：在源配置上记录源错误，并将依赖的资产轨道标记为 stale。
		return runner.recordSourceError(ctx, tenantID, spec, "asset_path_not_found", scopeKey, commit)
	}

	seen := make([]uuid.UUID, 0, len(matches))
	for _, relative := range matches {
		name := runner.renderAssetName(spec.AssetNameTemplate, relative)
		asset, err := runner.store.UpsertAsset(ctx, NewAsset{
			TenantID: tenantID, ID: uuid.NewV7(), ServiceID: service.ID, Kind: spec.Kind, Name: name,
		})
		if err != nil {
			return err
		}
		layer, err := runner.ensureBaseLayer(ctx, tenantID, asset, spec)
		if err != nil {
			return err
		}
		revision, contentHash, err := runner.storeRevision(ctx, tenantID, layer, spec, scopeKey, commit, checkout, relative)
		if err != nil {
			return err
		}
		head, err := runner.upsertLayerHead(ctx, tenantID, layer, scopeKey, revision.ID)
		if err != nil {
			return err
		}
		if err := runner.materializeVersion(ctx, tenantID, asset, service, spec, layer, revision, contentHash, commit, defaultBranch, refType, refName, checkout, relative); err != nil {
			return err
		}
		binding, err := runner.store.UpsertSourceBinding(ctx, NewSourceBinding{
			TenantID: tenantID, ID: uuid.NewV7(), SourceSpecID: spec.ID, ScopeType: overlayScopeRef, ScopeKey: scopeKey,
			ExpansionKey: relative, ResolvedPath: new(relative), AssetID: asset.ID, LayerID: layer.ID,
			State: sourceBindingStateActive, LastSeenCommit: new(commit),
		})
		if err != nil {
			return err
		}
		seen = append(seen, binding.ID)
		_ = head
	}
	if err := runner.store.MarkBindingsStaleInScope(ctx, tenantID, spec.ID, overlayScopeRef, scopeKey, seen); err != nil {
		return err
	}
	// 源物化成功：清除任何先前失败并将依赖资产轨道恢复为 ok。
	if err := runner.store.ClearSourceLastError(ctx, tenantID, spec.ID); err != nil {
		return err
	}
	return runner.store.MarkTracksHealthyForSourceSpec(ctx, tenantID, spec.ID)
}

func (runner *PipelineRunner) globMatches(checkout, pattern string) ([]string, error) {
	root := checkout
	rooted := pattern
	if !filepath.IsAbs(pattern) {
		rooted = filepath.Join(checkout, pattern)
	}
	matches, err := filepath.Glob(rooted)
	if err != nil {
		return nil, fmt.Errorf("glob source path: %w", err)
	}
	relatives := make([]string, 0, len(matches))
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr != nil || info.IsDir() {
			continue
		}
		relative, relErr := filepath.Rel(root, match)
		if relErr != nil {
			return nil, relErr
		}
		relatives = append(relatives, filepath.ToSlash(relative))
	}
	return relatives, nil
}

func (runner *PipelineRunner) renderAssetName(template, relative string) string {
	base := filepath.Base(relative)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	name := template
	if name == "" {
		name = "{file_stem}"
	}
	name = strings.ReplaceAll(name, "{file_stem}", stem)
	parent := filepath.Base(filepath.Dir(relative))
	if parent == "." || parent == "" {
		parent = ""
	}
	name = strings.ReplaceAll(name, "{parent_dir}", parent)
	name = normalizeAssetName(name)
	if name == "" {
		name = stem
	}
	return name
}

func normalizeAssetName(name string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		default:
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.TrimSuffix(builder.String(), "-")
}

func (runner *PipelineRunner) ensureBaseLayer(ctx context.Context, tenantID uuid.UUID, asset AssetRecord, spec SourceSpecRecord) (LayerRecord, error) {
	existing, err := runner.store.GetBaseLayerForAsset(ctx, tenantID, asset.ID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return LayerRecord{}, err
	}
	return runner.store.CreateLayer(ctx, NewLayer{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: asset.ID, SourceSpecID: new(spec.ID),
		Role: "base", Origin: "repo", Ord: 0, Dialect: nil, Enabled: true,
		BranchPatterns: append([]string(nil), spec.BranchPatterns...), DisplayName: asset.Name,
	})
}

func (runner *PipelineRunner) storeRevision(ctx context.Context, tenantID uuid.UUID, layer LayerRecord, spec SourceSpecRecord, scopeKey, commit, checkout, relative string) (LayerRevisionRecord, string, error) {
	content, err := os.ReadFile(filepath.Join(checkout, filepath.FromSlash(relative)))
	if err != nil {
		return LayerRevisionRecord{}, "", fmt.Errorf("read source file %s: %w", relative, err)
	}
	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])
	// 相同内容的重复同步不得铸造新修订：当该作用域最新修订内容哈希未变时复用之。
	if existing, getErr := runner.store.GetLatestLayerRevision(ctx, tenantID, layer.ID, overlayScopeRef, scopeKey); getErr == nil && existing.ContentHash == contentHash {
		return existing, contentHash, nil
	}
	blob, err := runner.blobs.Put(ctx, strings.NewReader(string(content)))
	if err != nil {
		return LayerRevisionRecord{}, "", fmt.Errorf("store layer content blob: %w", err)
	}
	revision, err := runner.store.CreateLayerRevision(ctx, NewLayerRevision{
		TenantID: tenantID, ID: uuid.NewV7(), LayerID: layer.ID, ScopeType: overlayScopeRef, ScopeKey: scopeKey,
		ContentHash: contentHash, ContentRef: blob.Digest, ContentType: contentTypeYAML,
		Dialect: nil, SourceBranch: new(strings.TrimPrefix(scopeKey, overlayRefTypeBranch+mergeScopeRefSelectorPrefix)), ReviewStatus: openapiReviewNotRequired,
		GitCommit: new(commit), CreatedBy: nil, ProducerRunID: nil,
	})
	if err != nil {
		return LayerRevisionRecord{}, "", err
	}
	return revision, contentHash, nil
}

func (runner *PipelineRunner) upsertLayerHead(ctx context.Context, tenantID uuid.UUID, layer LayerRecord, scopeKey string, revisionID uuid.UUID) (LayerHeadRecord, error) {
	return runner.store.UpsertLayerHead(ctx, NewLayerHead{
		TenantID: tenantID, LayerID: layer.ID, ScopeType: overlayScopeRef, ScopeKey: scopeKey,
		LatestRevisionID: new(revisionID), EffectiveRevisionID: new(revisionID), CandidateRevisionID: nil, Generation: 1,
	})
}

func (runner *PipelineRunner) materializeVersion(ctx context.Context, tenantID uuid.UUID, asset AssetRecord, service ServiceRecord, spec SourceSpecRecord, layer LayerRecord, revision LayerRevisionRecord, contentHash, commit, defaultBranch, refType, refName, checkout, relative string) error {
	track, err := runner.store.CreateAssetRefTrack(ctx, NewAssetRefTrack{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: asset.ID, RefType: refType, RefName: refName, Health: assetHealthOK,
	})
	if err != nil {
		return err
	}

	manifest := []LayerManifestEntry{{
		LayerID: layer.ID, RevisionID: revision.ID, Role: layer.Role, Origin: layer.Origin, Ord: layer.Ord,
		ScopeType: overlayScopeRef, ScopeKey: overlayRefTypeBranch + mergeScopeRefSelectorPrefix + refName, ReviewStatus: openapiReviewNotRequired, ContentHash: contentHash,
	}}
	fingerprint := runner.buildFingerprint(manifest)

	latest, err := runner.store.GetLatestVersionInTrack(ctx, tenantID, track.ID)
	if err == nil && latest.InputFingerprint == fingerprint {
		// 无新版本：内容与当前最新版本相同。
		return nil
	}
	_ = latest

	sequence := int64(1)
	var baseline *uuid.UUID
	if err == nil {
		sequence = latest.SequenceNo + 1
		baseline = new(latest.ID)
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	labels, err := json.Marshal(map[string]any{})
	if err != nil {
		return err
	}
	sourceCommit := new(commit)
	version, err := runner.store.CreateAssetVersion(ctx, NewAssetVersion{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: asset.ID, TrackID: track.ID, SequenceNo: sequence,
		Version: nextVersionLabel(latest, sequence), Lifecycle: lifecycleDraft, Revision: 1,
		InputFingerprint: fingerprint, MergeEngineVersion: mergeEngineVersion,
		KindPluginVersion: new(openapiPluginVersion), LayerManifest: manifestBytes,
		MergedHash: new(contentHash), SourceCommit: sourceCommit, BaselineVersionID: baseline,
		Labels: labels, IndexComplete: false,
	})
	if err != nil {
		return err
	}
	if err := runner.store.UpdateAssetRefTrackHead(ctx, tenantID, track.ID, new(version.ID), nil, 1); err != nil {
		return err
	}
	if err := runner.indexOpenAPIItems(ctx, tenantID, asset, service, version, checkout, relative); err != nil {
		return err
	}
	// openapi 类别的条目已完全物化；将版本标记为已索引，使 operations 视图解析到其条目集。
	return runner.store.MarkAssetVersionIndexed(ctx, tenantID, version.ID)
}

func (runner *PipelineRunner) buildFingerprint(manifest []LayerManifestEntry) string {
	payload, _ := json.Marshal(struct {
		Manifest           []LayerManifestEntry `json:"manifest"`
		MergeEngineVersion string               `json:"mergeEngineVersion"`
	}{Manifest: manifest, MergeEngineVersion: mergeEngineVersion})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func nextVersionLabel(latest AssetVersionRecord, sequence int64) string {
	if latest.Version == "" {
		return initialVersionLabel
	}
	// 递增 patch 段；破坏性/主版本决策属于后续里程碑。
	return incrementVersionLabel(latest.Version)
}

// incrementVersionLabel 递增 semver 标签的补丁段。这是「其他一切输入变更」的 M2 自动递增默认。
func incrementVersionLabel(version string) string {
	major, minor, patch := "0", "0", "0"
	parts := strings.SplitN(version, ".", 3)
	if len(parts) > 0 {
		major = parts[0]
	}
	if len(parts) > 1 {
		minor = parts[1]
	}
	if len(parts) > 2 {
		patch = parts[2]
	}
	nextPatch, err := strconv.Atoi(patch)
	if err != nil {
		return version
	}
	return major + "." + minor + "." + strconv.Itoa(nextPatch+1)
}

func (runner *PipelineRunner) recordSourceError(ctx context.Context, tenantID uuid.UUID, spec SourceSpecRecord, code, scopeKey, commit string) error {
	// M1 将失败记录为该作用域的错误绑定状态，使 listSourceBindings 与源状态反映 asset_path_not_found。
	if err := runner.store.MarkBindingsStaleInScope(ctx, tenantID, spec.ID, overlayScopeRef, scopeKey, nil); err != nil {
		return err
	}
	if err := runner.store.SetSourceLastError(ctx, tenantID, spec.ID, code); err != nil {
		return err
	}
	if err := runner.store.MarkTracksStaleForSourceSpec(ctx, tenantID, spec.ID); err != nil {
		return err
	}
	return &SourceMaterializationError{Code: code, Commit: commit}
}

// SourceMaterializationError 是不含秘密的管线阶段失败。
type SourceMaterializationError struct {
	// Code 承载 SourceMaterializationError 的生成 Code 值。
	Code string
	// Commit 承载 SourceMaterializationError 的生成 Commit 值。
	Commit string
}

// Error 实现 Meridian OpenAPI 契约的生成传输行为。
func (err *SourceMaterializationError) Error() string {
	return "source materialization failed: " + err.Code
}

// openapiOperationItem 是从解析后的 OpenAPI 文档提取的一个操作。
type openapiOperationItem struct {
	// Method 承载 openapiOperationItem 的生成 Method 值。
	Method string
	// Path 承载 openapiOperationItem 的生成 Path 值。
	Path string
	// Summary 承载 openapiOperationItem 的生成 Summary 值。
	Summary string
	// Tags 承载 openapiOperationItem 的生成 Tags 值。
	Tags []string
	// Deprecated 承载 openapiOperationItem 的生成 Deprecated 值。
	Deprecated bool
}

// indexOpenAPIItems 解析规范化 OpenAPI 文档，并将每个操作以 'UPPER(method) normalizedPath' 为键
// 索引为资产条目。
func (runner *PipelineRunner) indexOpenAPIItems(ctx context.Context, tenantID uuid.UUID, asset AssetRecord, service ServiceRecord, version AssetVersionRecord, checkout, relative string) error {
	content, err := os.ReadFile(filepath.Join(checkout, filepath.FromSlash(relative)))
	if err != nil {
		return fmt.Errorf("read openapi source %s: %w", relative, err)
	}
	descriptor, endpoint, err := lookupKindEndpoint(runner.kinds, asset.Kind)
	if err != nil {
		return err
	}
	result, err := endpoint.Invoke(ctx, plugin.Call{
		Capability: "kind/" + descriptor.Kind, Method: "extract", ContentType: "application/yaml", Payload: content,
		Metadata: map[string]string{"canonical-version": descriptor.PluginVersion},
	})
	if err != nil {
		return err
	}
	var response struct {
		Items []kinds.Item `json:"items"`
	}
	if err := json.Unmarshal(result.Payload, &response); err != nil {
		return err
	}
	items := response.Items
	for _, item := range items {
		display := item.Display
		displayBytes, _ := json.Marshal(display)
		rawBytes, _ := json.Marshal(display)
		provenance := []byte(`{}`)
		if item.Provenance != nil {
			if encoded, marshalErr := json.Marshal(item.Provenance); marshalErr == nil {
				provenance = encoded
			}
		}
		if _, err := runner.store.CreateAssetItem(ctx, NewAssetItem{
			TenantID: tenantID, ID: uuid.NewV7(), AssetVersionID: version.ID, AssetID: asset.ID, ServiceID: service.ID,
			Kind: asset.Kind, ItemType: item.ItemType, Key: item.Key, Display: displayBytes,
			SearchText: item.SearchText, SearchRaw: rawBytes, Provenance: provenance,
		}); err != nil {
			return err
		}
	}
	return nil
}

// parseOpenAPIOperations 解析 OpenAPI YAML 文档，并按路径后方法确定性排序返回其操作。
// 这是与性能门禁共享的提取步骤。
func parseOpenAPIOperations(content []byte) ([]openapiOperationItem, error) {
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return nil, err
	}
	paths, _ := document["paths"].(map[string]any)
	operations := make([]openapiOperationItem, 0)
	for path, pathItem := range paths {
		item, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for method, operation := range item {
			upper := strings.ToUpper(method)
			if !validHTTPMethod(upper) {
				continue
			}
			op, _ := operation.(map[string]any)
			operations = append(operations, openapiOperationItem{
				Method:     upper,
				Path:       path,
				Summary:    stringValue(op["summary"]),
				Tags:       stringSliceValue(op["tags"]),
				Deprecated: boolValue(op["deprecated"]),
			})
		}
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].Path != operations[j].Path {
			return operations[i].Path < operations[j].Path
		}
		return operations[i].Method < operations[j].Method
	})
	return operations, nil
}

func validHTTPMethod(method string) bool {
	switch method {
	case "GET", "PUT", "POST", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE":
		return true
	}
	return false
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func stringSliceValue(value any) []string {
	if list, ok := value.([]any); ok {
		result := make([]string, 0, len(list))
		for _, entry := range list {
			if text, ok := entry.(string); ok {
				result = append(result, text)
			}
		}
		return result
	}
	return []string{}
}

func boolValue(value any) bool {
	if flag, ok := value.(bool); ok {
		return flag
	}
	return false
}

var _ task.SyncRunner = (*PipelineRunner)(nil)
