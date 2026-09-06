package handler

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/meridian-labs/meridian/internal/generated/api"
	"github.com/meridian-labs/meridian/internal/service"
)

func TestHandlerHealthAndStaticFallback(t *testing.T) {
	t.Parallel()

	server := New()
	tests := []struct {
		name string
		path string
		want int
	}{
		{name: "health", path: "/healthz", want: http.StatusOK},
		{name: "ready", path: "/readyz", want: http.StatusOK},
		{name: "contract", path: "/api/v1/openapi.yaml", want: http.StatusOK},
		{name: "deep link", path: "/t/acme/services/order", want: http.StatusOK},
		{name: "api 404", path: "/api/v1/does-not-exist", want: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
		})
	}
}

func TestOpenAPIOperationsHaveAuthenticationPolicies(t *testing.T) {
	t.Parallel()

	policies := authPolicies()
	if len(policies) == 0 {
		t.Fatal("no authentication policies were derived from OpenAPI")
	}
	for _, operation := range []string{"Healthz", "Login", "GetMe", "CreateToken", "ReceiveGitWebhook"} {
		if _, ok := policies[operation]; !ok {
			t.Errorf("authentication policy for %s is missing", operation)
		}
	}
	if policies["Healthz"].required || policies["Login"].required || policies["ReceiveGitWebhook"].required {
		t.Fatal("a contract-public operation requires authentication")
	}
	if !policies["GetMe"].required || !policies["GetMe"].allowCookie || policies["GetMe"].allowPAT {
		t.Fatalf("GetMe policy = %#v", policies["GetMe"])
	}
}

func TestProtectedOperationRejectsAnonymousRequestBeforeHandler(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	response := httptest.NewRecorder()
	NewWithIdentity(&service.Identity{}, false).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

func TestVersionMatchesContract(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	response := httptest.NewRecorder()
	New().Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload map[string]string
	if err := json.UnmarshalRead(response.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, key := range []string{"serverVersion", "apiVersion", "contractVersion", "buildCommit"} {
		if payload[key] == "" {
			t.Errorf("%s is empty", key)
		}
	}
}

func TestAPINotFoundUsesErrorContract(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	response := httptest.NewRecorder()
	New().Handler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	var payload struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	}
	if err := json.UnmarshalRead(response.Body, &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "not_found" || payload.Message == "" || payload.RequestID == "" {
		t.Fatalf("invalid error response: %#v", payload)
	}
	if got := response.Header().Get("X-Request-Id"); got != payload.RequestID {
		t.Fatalf("X-Request-Id = %q, body requestId = %q", got, payload.RequestID)
	}
}

func TestStaticCachePolicy(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "index", path: "/", want: "no-cache"},
		{name: "deep link", path: "/t/acme/services/order", want: "no-cache"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			New().Handler().ServeHTTP(response, request)
			if got := response.Header().Get("Cache-Control"); got != test.want {
				t.Fatalf("Cache-Control = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStaticDeepLinkHead(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodHead, "/t/acme/services/order", nil)
	response := httptest.NewRecorder()
	New().Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestGlobalCredentialInputAcceptsHTTPUnion(t *testing.T) {
	t.Parallel()
	var body api.GlobalCredentialCreateRequest
	if err := json.Unmarshal([]byte(`{"name":"global","kind":"http_token","httpToken":{"username":"bot","token":"global-first-token"}}`), &body); err != nil {
		t.Fatalf("decode global credential body: %v", err)
	}
	input, err := globalCredentialInput(body)
	if err != nil {
		t.Fatalf("global credential input: %v", err)
	}
	if input.Name != "global" || input.Secret.Kind != "http_token" || input.Secret.HTTPToken != "global-first-token" {
		t.Fatalf("global credential input = %#v", input)
	}
}

func TestKnownHostContractViolationsReturn422(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"host":"git.example.com","publicKey":"AAAA","keyType":"ssh-rsa","fingerprint":"SHA256:forged"}`,
		`{"host":"git.example.com","publicKey":"invalid base64"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/t/acme/known-hosts", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		New().Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422; body = %s", response.Code, response.Body.String())
		}
		var payload struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode validation error: %v", err)
		}
		if payload.Code != "validation_error" {
			t.Fatalf("error code = %q, want validation_error", payload.Code)
		}
	}
}
