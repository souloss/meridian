package handler

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
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
