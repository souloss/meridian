package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/meridian-labs/meridian/internal/task"
)

// WebhookDeliverer 通过加密通道配置执行外发 Webhook 投递。
// 签名遵循 contracts/events.yaml x-webhook.signature：
// X-Meridian-Signature-256: sha256=<hmac-sha256(raw-body) 的小写十六进制>。
type WebhookDeliverer struct {
	keyring CredentialKeyring
	client  *http.Client
	now     func() time.Time
}

// NewWebhookDeliverer 构造使用活跃主密钥解密通道配置的投递器。
func NewWebhookDeliverer(keyring CredentialKeyring) *WebhookDeliverer {
	return &WebhookDeliverer{
		keyring: keyring,
		client:  &http.Client{Timeout: webhookRequestTimeout},
		now:     time.Now,
	}
}

// Deliver 按通道类别投递一个已租约事件。in_app 通道不产生外部请求，
// 其站内通知已在事件路由阶段落库；email 通道留待部署管理 SMTP。
func (deliverer *WebhookDeliverer) Deliver(ctx context.Context, delivery task.OutboxDelivery) error {
	switch delivery.ChannelType {
	case notificationChannelKindInApp:
		return nil
	case notificationChannelKindWebhook:
		return deliverer.deliverWebhook(ctx, delivery)
	default:
		return ErrNotificationDeliveryUnsupported
	}
}

// deliverWebhook 解密配置、签名并 POST 事件信封；非 2xx 视为可重试失败。
func (deliverer *WebhookDeliverer) deliverWebhook(ctx context.Context, delivery task.OutboxDelivery) error {
	cfg, err := deliverer.keyring.openChannelConfig(delivery.TenantID, delivery.ChannelID, delivery.EncryptedConfig)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationDeliveryFailed, err)
	}
	if cfg.Endpoint == "" || cfg.Secret == "" {
		return ErrNotificationDeliveryFailed
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, bytes.NewReader(delivery.Payload))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationDeliveryFailed, err)
	}
	request.Header.Set(webhookSignatureHeader, webhookSignatureValue(cfg.Secret, delivery.Payload))
	request.Header.Set("Content-Type", "application/json")
	response, err := deliverer.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationDeliveryFailed, err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
	if !webhookSuccessStatus(response.StatusCode) {
		return fmt.Errorf("%w: http %d", ErrNotificationDeliveryFailed, response.StatusCode)
	}
	return nil
}

// webhookSignatureValue 计算并格式化 HMAC-SHA256 签名。
func webhookSignatureValue(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return webhookSignatureScheme + hex.EncodeToString(mac.Sum(nil))
}

func webhookSuccessStatus(status int) bool {
	switch status {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
		return true
	}
	return false
}

// 外发 Webhook 投递常量（值来源 events.yaml x-webhook 与 domain.yaml kindRules）。
const (
	// webhookSignatureHeader 是 Webhook 签名请求头名。
	webhookSignatureHeader = "X-Meridian-Signature-256"
	// webhookSignatureScheme 是签名值的固定前缀。
	webhookSignatureScheme = "sha256="
	// webhookRequestTimeout 是单次 Webhook 投递请求的超时。
	webhookRequestTimeout = 10 * time.Second
)

var _ task.OutboxDeliverer = (*WebhookDeliverer)(nil)
