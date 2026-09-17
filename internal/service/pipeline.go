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
	"strings"
	"time"
	"uuid"

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

// BlobStore persists immutable content-addressed bytes for layer revisions.
type BlobStore interface {
	Put(context.Context, io.Reader) (storage.Blob, error)
	Open(string) (*os.File, storage.Blob, error)
}

// PipelineRunner materializes repository sources into assets, layers,
// revisions, versions, items, and source bindings. It implements task.SyncRunner.
type PipelineRunner struct {
	store     AssetStore
	blobs     BlobStore
	workspace string
	gitBinary string
	now       func() time.Time
}

// NewPipelineRunner constructs the M1 repository synchronization pipeline.
func NewPipelineRunner(store AssetStore, blobs BlobStore, workspace string) *PipelineRunner {
	return &PipelineRunner{store: store, blobs: blobs, workspace: workspace, gitBinary: "git", now: time.Now}
}

// Run performs one repository synchronization across the six pipeline stages.
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
	refType := "branch"
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

// materializeService synchronizes all builtin source specs of one service.
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

// materializeSource globs one builtin source spec path and materializes one
// asset per matched file, then commits the resolved scope atomically.
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
		// No files matched the path: record the source error on the spec and
		// mark the dependent asset tracks stale.
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
			TenantID: tenantID, ID: uuid.NewV7(), SourceSpecID: spec.ID, ScopeType: "ref", ScopeKey: scopeKey,
			ExpansionKey: relative, ResolvedPath: new(relative), AssetID: asset.ID, LayerID: layer.ID,
			State: "active", LastSeenCommit: new(commit),
		})
		if err != nil {
			return err
		}
		seen = append(seen, binding.ID)
		_ = head
	}
	if err := runner.store.MarkBindingsStaleInScope(ctx, tenantID, spec.ID, "ref", scopeKey, seen); err != nil {
		return err
	}
	// The source materialized successfully: clear any prior failure and restore
	// the dependent asset tracks to ok.
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
	// A repeat sync with identical content must not mint a new revision: reuse
	// the latest revision in this scope when its content hash is unchanged.
	if existing, getErr := runner.store.GetLatestLayerRevision(ctx, tenantID, layer.ID, "ref", scopeKey); getErr == nil && existing.ContentHash == contentHash {
		return existing, contentHash, nil
	}
	blob, err := runner.blobs.Put(ctx, strings.NewReader(string(content)))
	if err != nil {
		return LayerRevisionRecord{}, "", fmt.Errorf("store layer content blob: %w", err)
	}
	revision, err := runner.store.CreateLayerRevision(ctx, NewLayerRevision{
		TenantID: tenantID, ID: uuid.NewV7(), LayerID: layer.ID, ScopeType: "ref", ScopeKey: scopeKey,
		ContentHash: contentHash, ContentRef: blob.Digest, ContentType: "application/yaml",
		Dialect: nil, SourceBranch: new(strings.TrimPrefix(scopeKey, "branch:")), ReviewStatus: openapiReviewNotRequired,
		GitCommit: new(commit), CreatedBy: nil, ProducerRunID: nil,
	})
	if err != nil {
		return LayerRevisionRecord{}, "", err
	}
	return revision, contentHash, nil
}

func (runner *PipelineRunner) upsertLayerHead(ctx context.Context, tenantID uuid.UUID, layer LayerRecord, scopeKey string, revisionID uuid.UUID) (LayerHeadRecord, error) {
	return runner.store.UpsertLayerHead(ctx, NewLayerHead{
		TenantID: tenantID, LayerID: layer.ID, ScopeType: "ref", ScopeKey: scopeKey,
		LatestRevisionID: new(revisionID), EffectiveRevisionID: new(revisionID), CandidateRevisionID: nil, Generation: 1,
	})
}

func (runner *PipelineRunner) materializeVersion(ctx context.Context, tenantID uuid.UUID, asset AssetRecord, service ServiceRecord, spec SourceSpecRecord, layer LayerRecord, revision LayerRevisionRecord, contentHash, commit, defaultBranch, refType, refName, checkout, relative string) error {
	track, err := runner.store.CreateAssetRefTrack(ctx, NewAssetRefTrack{
		TenantID: tenantID, ID: uuid.NewV7(), AssetID: asset.ID, RefType: refType, RefName: refName, Health: "ok",
	})
	if err != nil {
		return err
	}

	manifest := []LayerManifestEntry{{
		LayerID: layer.ID, RevisionID: revision.ID, Role: layer.Role, Origin: layer.Origin, Ord: layer.Ord,
		ScopeType: "ref", ScopeKey: "branch:" + refName, ReviewStatus: openapiReviewNotRequired, ContentHash: contentHash,
	}}
	fingerprint := runner.buildFingerprint(manifest)

	latest, err := runner.store.GetLatestVersionInTrack(ctx, tenantID, track.ID)
	if err == nil && latest.InputFingerprint == fingerprint {
		// No new version: content is identical to the current latest.
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
		Version: nextVersionLabel(latest, sequence), Lifecycle: "draft", Revision: 1,
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
	// Items for the openapi kind are fully materialized; mark the version
	// indexed so the operations view resolves to its item set.
	return runner.store.MarkAssetVersionIndexed(ctx, tenantID, version.ID)
}

func (runner *PipelineRunner) buildFingerprint(manifest []LayerManifestEntry) string {
	payload, _ := json.Marshal(struct {
		Manifest          []LayerManifestEntry `json:"manifest"`
		MergeEngineVersion string              `json:"mergeEngineVersion"`
	}{Manifest: manifest, MergeEngineVersion: mergeEngineVersion})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func nextVersionLabel(latest AssetVersionRecord, sequence int64) string {
	if latest.Version == "" {
		return initialVersionLabel
	}
	return latest.Version
}

func (runner *PipelineRunner) recordSourceError(ctx context.Context, tenantID uuid.UUID, spec SourceSpecRecord, code, scopeKey, commit string) error {
	// M1 records the failure as an error binding state for the scope so that
	// listSourceBindings and source status reflect the asset_path_not_found.
	if err := runner.store.MarkBindingsStaleInScope(ctx, tenantID, spec.ID, "ref", scopeKey, nil); err != nil {
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

// SourceMaterializationError is a secret-free pipeline stage failure.
type SourceMaterializationError struct {
	Code   string
	Commit string
}

func (err *SourceMaterializationError) Error() string {
	return "source materialization failed: " + err.Code
}

// openapiOperationItem is one operation extracted from a parsed OpenAPI document.
type openapiOperationItem struct {
	Method     string
	Path       string
	Summary    string
	Tags       []string
	Deprecated bool
}

// indexOpenAPIItems parses the normalized OpenAPI document and indexes each
// operation as an asset item keyed by 'UPPER(method) normalizedPath'.
func (runner *PipelineRunner) indexOpenAPIItems(ctx context.Context, tenantID uuid.UUID, asset AssetRecord, service ServiceRecord, version AssetVersionRecord, checkout, relative string) error {
	content, err := os.ReadFile(filepath.Join(checkout, filepath.FromSlash(relative)))
	if err != nil {
		return fmt.Errorf("read openapi source %s: %w", relative, err)
	}
	operations, err := parseOpenAPIOperations(content)
	if err != nil {
		return err
	}
	for _, operation := range operations {
		display := map[string]any{
			"method": operation.Method, "path": operation.Path,
			"summary": operation.Summary, "tags": operation.Tags, "deprecated": operation.Deprecated,
		}
		displayBytes, _ := json.Marshal(display)
		rawBytes, _ := json.Marshal(display)
		if _, err := runner.store.CreateAssetItem(ctx, NewAssetItem{
			TenantID: tenantID, ID: uuid.NewV7(), AssetVersionID: version.ID, AssetID: asset.ID, ServiceID: service.ID,
			Kind: asset.Kind, ItemType: "operation", Key: operation.Method + " " + normalizePath(operation.Path), Display: displayBytes,
			SearchText: operation.Method + " " + operation.Path + " " + operation.Summary, SearchRaw: rawBytes, Provenance: []byte("{}"),
		}); err != nil {
			return err
		}
	}
	return nil
}

// parseOpenAPIOperations parses an OpenAPI YAML document and returns its
// operations deterministically ordered by path then method. This is the
// extraction step shared with the perf gate.
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
