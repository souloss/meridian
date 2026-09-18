package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"uuid"

	"github.com/meridian-labs/meridian/internal/task"
)

// discoveryMarkerFiles 是契约定义的服务根标记文件。
var discoveryMarkerFiles = [...]string{
	"go.mod", "pom.xml", "build.gradle", "build.gradle.kts", "package.json", "pyproject.toml", "Cargo.toml",
}

// discoveryMarkerDirectories 是契约定义的服务根标记目录。
var discoveryMarkerDirectories = [...]string{"proto"}

// DiscoveryRunner 遍历克隆的仓库树并 upsert 发现候选。
// 默认实现通过 git CLI 克隆到工作区根，并从解析后的树计算标记。
type DiscoveryRunner struct {
	store     DiscoveryStore
	workspace string
	gitBinary string
	now       func() time.Time
}

// NewDiscoveryRunner 构造生产仓库发现运行器。
// 工作区根必须是持久或可重建卷上的绝对路径。
func NewDiscoveryRunner(store DiscoveryStore, workspace string) *DiscoveryRunner {
	return &DiscoveryRunner{store: store, workspace: workspace, gitBinary: "git", now: time.Now}
}

// RunDiscovery implements task.DiscoverRunner.
func (runner *DiscoveryRunner) RunDiscovery(ctx context.Context, args task.DiscoverArgs) (task.DiscoverResult, error) {
	checkout, cleanup, err := runner.clone(ctx, args)
	if err != nil {
		return task.DiscoverResult{}, err
	}
	defer cleanup()

	commit, err := runner.resolveCommit(ctx, checkout)
	if err != nil {
		return task.DiscoverResult{}, err
	}

	markers, err := walkMarkers(checkout)
	if err != nil {
		return task.DiscoverResult{}, err
	}
	candidates := candidateRoots(markers)
	for _, root := range candidates {
		detected := markerKinds(markers[root])
		if _, err := runner.store.UpsertDiscoveryCandidate(ctx, NewDiscoveryCandidate{
			TenantID: args.TenantID, ID: uuid.NewV7(), RepositoryID: args.RepositoryID,
			CommitSHA: commit, RootDir: root, Detected: detected,
		}); err != nil {
			return task.DiscoverResult{}, err
		}
	}
	return task.DiscoverResult{ResolvedCommit: commit, CandidateCount: len(candidates)}, nil
}

func (runner *DiscoveryRunner) clone(ctx context.Context, args task.DiscoverArgs) (string, func(), error) {
	if runner.workspace == "" || !filepath.IsAbs(runner.workspace) {
		return "", func() {}, errors.New("discovery workspace root is not an absolute path")
	}
	checkout := filepath.Join(runner.workspace, args.TenantID.String(), args.RepositoryID.String(), args.JobID.String())
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		return "", func() {}, fmt.Errorf("create discovery workspace: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(checkout) }
	cleanup()

	record, err := runner.store.GetRepository(ctx, args.TenantID, args.RepositoryID)
	if err != nil {
		return "", func() {}, err
	}
	if _, err := exec.LookPath(runner.gitBinary); err != nil {
		return "", cleanup, errors.New("git is not available for repository discovery")
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
		return "", cleanup, err
	}
	return checkout, cleanup, nil
}

func (runner *DiscoveryRunner) resolveCommit(ctx context.Context, checkout string) (string, error) {
	command := exec.CommandContext(ctx, runner.gitBinary, "-C", checkout, "rev-parse", "HEAD")
	command.Env = append([]string(nil), os.Environ()...)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve discovery commit: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

type markerSet map[string]bool

// walkMarkers 返回目录路径到其中发现的标记名集合的映射。
func walkMarkers(checkout string) (map[string]markerSet, error) {
	markers := make(map[string]markerSet)
	err := filepath.WalkDir(checkout, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || (path != checkout && strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			relative, err := filepath.Rel(checkout, path)
			if err != nil {
				return err
			}
			if contains(discoveryMarkerDirectories[:], name) {
				root := filepath.ToSlash(filepath.Dir(relative))
				if root == "." {
					root = ""
				}
				ensureMarker(markers, root, "dir:"+name)
			}
			return nil
		}
		relative, err := filepath.Rel(checkout, path)
		if err != nil {
			return err
		}
		base := filepath.Base(relative)
		if contains(discoveryMarkerFiles[:], base) {
			root := filepath.ToSlash(filepath.Dir(relative))
			if root == "." {
				root = ""
			}
			ensureMarker(markers, root, base)
		}
		return nil
	})
	return markers, err
}

// candidateRoots 返回携带标记的目录，当根目录存在任何标记文件时也包含仓库根。
func candidateRoots(markers map[string]markerSet) []string {
	roots := make([]string, 0, len(markers))
	for root := range markers {
		if len(markers[root]) > 0 {
			roots = append(roots, root)
		}
	}
	sort.Strings(roots)
	return roots
}

func markerKinds(markers markerSet) map[string]any {
	kinds := make([]string, 0, len(markers))
	for marker := range markers {
		kinds = append(kinds, marker)
	}
	sort.Strings(kinds)
	return map[string]any{"markers": kinds}
}

func ensureMarker(markers map[string]markerSet, root, marker string) {
	if markers[root] == nil {
		markers[root] = markerSet{}
	}
	markers[root][marker] = true
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
