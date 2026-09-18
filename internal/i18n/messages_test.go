package i18n

import (
	"testing"

	meridian "github.com/meridian-labs/meridian"
)

// TestBundleLoadAndRender 校验 YAML 消息加载与变量/单复数渲染。
func TestBundleLoadAndRender(t *testing.T) {
	bundle := New(meridian.I18nLocales, "internal/i18n/locales")
	if err := bundle.Load(); err != nil {
		t.Fatalf("load locales: %v", err)
	}
	if got := bundle.T(LangZHCN, "error.not_found", nil, nil); got != "资源不存在或无权访问。" {
		t.Fatalf("zh not_found = %q", got)
	}
	if got := bundle.T(LangEN, "error.not_found", nil, nil); got != "The resource is absent or unauthorized." {
		t.Fatalf("en not_found = %q", got)
	}
	if got := bundle.T(LangZHCN, "error.quota_exceeded", map[string]any{"Current": 3, "Limit": 2}, 1); got != "租户资源配额将超出限制（当前 3，上限 2）。" {
		t.Fatalf("zh quota_exceeded = %q", got)
	}
	if got := bundle.T(LangZHCN, "error.missing", nil, nil); got != "error.missing" {
		t.Fatalf("missing message fallback = %q", got)
	}
}
