package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"strings"
	"time"
	"uuid"
)

// 本文件承载入站 Git webhook 的签名校验与同步任务入队用例（receiveGitWebhook）。

// WebhookStore 是入站 webhook 的持久化边界。
type WebhookStore interface {
	// GetRepositoryWebhookSecretHash 返回仓库的 webhook 校验秘密摘要（可为空）。
	GetRepositoryWebhookSecretHash(context.Context, uuid.UUID, uuid.UUID) ([]byte, error)
	// GetRepositoryTenantByID 跨租户按 id 定位仓库所属租户。
	GetRepositoryTenantByID(context.Context, uuid.UUID) (RepositoryTenantRef, error)
	// EnqueueWebhookSyncJob 为入站 webhook 记录一条 repo.sync 任务。
	EnqueueWebhookSyncJob(context.Context, uuid.UUID, uuid.UUID, string, string) (JobAccepted, error)
}

// Webhooks 协调入站 Git webhook 校验与同步入队。
type Webhooks struct {
	store WebhookStore
	now   func() time.Time
}

// NewWebhooks 构造入站 webhook 用例。
func NewWebhooks(store WebhookStore) *Webhooks {
	return &Webhooks{store: store, now: time.Now}
}

// GitPushEvent 是一条已解码的 push 事件。
type GitPushEvent struct {
	// RefType 是引用类型（branch/tag）。
	RefType string
	// Ref 是引用名（如 refs/heads/main）。
	Ref string
	// Deleted 表示该引用是否被删除。
	Deleted bool
	// BeforeCommit 是变更前提交。
	BeforeCommit *string
	// AfterCommit 是变更后提交（可为空，表示删除）。
	AfterCommit *string
}

// Receive 校验签名并为其仓库入队一条同步任务。
// 无已配置秘密的仓库拒绝处理（401），避免无法验证的入站变更被信任。
func (webhooks *Webhooks) Receive(ctx context.Context, repositoryID uuid.UUID, signature string, event GitPushEvent) (JobAccepted, error) {
	tenant, err := webhooks.store.GetRepositoryTenantByID(ctx, repositoryID)
	if err != nil {
		return JobAccepted{}, err
	}
	secretHash, err := webhooks.store.GetRepositoryWebhookSecretHash(ctx, tenant.TenantID, repositoryID)
	if err != nil {
		return JobAccepted{}, err
	}
	if len(secretHash) == 0 {
		return JobAccepted{}, ErrUnauthenticated
	}
	canonical, err := json.Marshal(event)
	if err != nil {
		return JobAccepted{}, ErrValidation
	}
	if !webhooks.verifySignature(secretHash, canonical, signature) {
		return JobAccepted{}, ErrUnauthenticated
	}
	if event.Deleted {
		// 删除引用无需同步，返回一个稳定接受的空任务投影。
		return JobAccepted{JobID: uuid.NewV7(), Deduplicated: true}, nil
	}
	refType, refName := webhooks.parseRef(event.Ref)
	if refName == "" {
		return JobAccepted{}, ErrValidation
	}
	return webhooks.store.EnqueueWebhookSyncJob(ctx, tenant.TenantID, repositoryID, refType, refName)
}

// verifySignature 以常量时间比较 HMAC-SHA256 签名（格式 sha256=<hex>）。
func (webhooks *Webhooks) verifySignature(secret, payload []byte, signature string) bool {
	const scheme = "sha256="
	if !strings.HasPrefix(signature, scheme) {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, scheme))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return hmac.Equal(provided, mac.Sum(nil))
}

// parseRef 将 refs/heads/main 规范化为 (branch, main)。
func (webhooks *Webhooks) parseRef(ref string) (string, string) {
	const headsPrefix = "refs/heads/"
	const tagsPrefix = "refs/tags/"
	switch {
	case strings.HasPrefix(ref, headsPrefix):
		return refTypeBranch, strings.TrimPrefix(ref, headsPrefix)
	case strings.HasPrefix(ref, tagsPrefix):
		return refTypeTag, strings.TrimPrefix(ref, tagsPrefix)
	default:
		return "", ""
	}
}
