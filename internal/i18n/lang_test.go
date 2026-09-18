package i18n

import "testing"

// TestNegotiate 校验 Accept-Language 解析与白名单回退。
func TestNegotiate(t *testing.T) {
	for _, testCase := range []struct {
		header string
		want   string
	}{
		{"", LangZHCN},
		{"en", LangEN},
		{"zh-CN", LangZHCN},
		{"zh-CN,zh;q=0.9,en;q=0.8", LangZHCN},
		{"en;q=0.9,zh-CN;q=0.5", LangEN},
		{"fr", LangZHCN},
	} {
		if got := Negotiate(testCase.header); got != testCase.want {
			t.Errorf("Negotiate(%q) = %q, want %q", testCase.header, got, testCase.want)
		}
	}
}
