package service

import (
	"testing"

	"github.com/meridian-labs/meridian/internal/task"
)

// TestAiTrustModeResolution verifies the trust-mode → initial review status
// mapping used by AI generation.
func TestAiTrustModeResolution(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		mode   string
		origin string
		want   bool
	}{
		{trustModeReviewRequired, aiOrigin, false},
		{trustModeTrustAI, aiOrigin, true},
		{trustModeTrustAI, "third_party", false},
		{trustModeTrustAIAndThirdParty, aiOrigin, true},
		{trustModeTrustAIAndThirdParty, "third_party", true},
		{trustModeTrustAIAndThirdParty, "repo", false},
	} {
		if got := trustApprovedForOrigin(testCase.mode, testCase.origin); got != testCase.want {
			t.Errorf("trustApprovedForOrigin(%q, %q) = %v, want %v", testCase.mode, testCase.origin, got, testCase.want)
		}
	}
}

// TestAiGenerateArgsKind verifies the River job kind is stable.
func TestAiGenerateArgsKind(t *testing.T) {
	t.Parallel()
	args := task.AiGenerateArgs{}
	if args.Kind() != "meridian_asset_ai_generate" {
		t.Fatalf("unexpected AI generation River kind")
	}
}
