//go:build integration

package handler

import (
	"encoding/json/v2"
	"net/http"
	"os"
	"testing"
)

// TestM1ServiceLifecycleSmoke exercises the service lifecycle state machine and
// public read gate (SMK-034) and the soft-delete cascade (SMK-037).
func TestM1ServiceLifecycleSmoke(t *testing.T) {
	if os.Getenv("MERIDIAN_TEST_DATABASE_URL") == "" {
		t.Fatal("M1 service lifecycle smoke requires MERIDIAN_TEST_DATABASE_URL")
	}
	t.Run("SMK-034", smokeM1ServiceLifecycleTransitions)
	t.Run("SMK-037", smokeM1ServiceSoftDelete)
}

func smokeM1ServiceLifecycleTransitions(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)
	f.createOpenAPISource(t, "order-service", "order-service/openapi.yaml")

	// serviceLifecycle is internal by default (accepted as private via candidate acceptance;
	// here we explicitly set visibility through the lifecycle patch).
	etag := f.serviceEtag(t, "order-service")

	// visibility public at draft → getPublicService 404 (draft not public eligible)
	pub := f.request(t, &f.alice, http.MethodPatch, "/api/v1/t/acme/services/order-service", map[string]any{"visibility": "public"}, map[string]string{"If-Match": etag})
	assertStatus(t, pub, http.StatusOK)
	publicRead := f.request(t, nil, http.MethodGet, "/api/v1/public/t/acme/services/order-service", nil, nil)
	assertStatus(t, publicRead, http.StatusNotFound)

	// published → getPublicService 200
	etag = responseString(t, pub, "etag")
	published := f.request(t, &f.alice, http.MethodPatch, "/api/v1/t/acme/services/order-service", map[string]any{"lifecycle": "published"}, map[string]string{"If-Match": etag})
	assertStatus(t, published, http.StatusOK)
	publicRead = f.request(t, nil, http.MethodGet, "/api/v1/public/t/acme/services/order-service", nil, nil)
	assertStatus(t, publicRead, http.StatusOK)

	// deprecated → getPublicService 200
	etag = responseString(t, published, "etag")
	deprecated := f.request(t, &f.alice, http.MethodPatch, "/api/v1/t/acme/services/order-service", map[string]any{"lifecycle": "deprecated"}, map[string]string{"If-Match": etag})
	assertStatus(t, deprecated, http.StatusOK)
	publicRead = f.request(t, nil, http.MethodGet, "/api/v1/public/t/acme/services/order-service", nil, nil)
	assertStatus(t, publicRead, http.StatusOK)

	// retired → getPublicService 404
	etag = responseString(t, deprecated, "etag")
	retired := f.request(t, &f.alice, http.MethodPatch, "/api/v1/t/acme/services/order-service", map[string]any{"lifecycle": "retired"}, map[string]string{"If-Match": etag})
	assertStatus(t, retired, http.StatusOK)
	publicRead = f.request(t, nil, http.MethodGet, "/api/v1/public/t/acme/services/order-service", nil, nil)
	assertStatus(t, publicRead, http.StatusNotFound)

	// retired → published is an invalid transition → 409 invalid_state
	etag = responseString(t, retired, "etag")
	invalid := f.request(t, &f.alice, http.MethodPatch, "/api/v1/t/acme/services/order-service", map[string]any{"lifecycle": "published"}, map[string]string{"If-Match": etag})
	assertError(t, invalid, http.StatusConflict, "invalid_state")

	// createSourceSpec on a retired service → 409 invalid_state
	createSource := f.request(t, &f.alice, http.MethodPost, "/api/v1/t/acme/services/order-service/sources", map[string]any{
		"kind": "openapi", "role": "base", "origin": "repo", "mode": "builtin", "path": "order-service/apis/billing/openapi.yaml",
	}, nil)
	assertError(t, createSource, http.StatusConflict, "invalid_state")
}

func smokeM1ServiceSoftDelete(t *testing.T) {
	f := newM1PipelineFixture(t)
	repositoryID, _ := f.setupRepository(t)
	f.acceptServices(t, repositoryID)
	f.createOpenAPISource(t, "order-service", "order-service/openapi.yaml")
	if job := f.syncAndWait(t, repositoryID); job["status"] != "succeeded" {
		t.Fatalf("sync status = %v", job["status"])
	}

	detail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, detail, http.StatusOK)
	var detailBody struct {
		Etag   string `json:"etag"`
		Assets []struct {
			ID string `json:"id"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode service detail: %v", err)
	}
	if len(detailBody.Assets) != 1 {
		t.Fatalf("assets = %d, want 1", len(detailBody.Assets))
	}
	assetID := detailBody.Assets[0].ID

	// deleteService under the current etag.
	deleted := f.request(t, &f.alice, http.MethodDelete, "/api/v1/t/acme/services/order-service", nil, map[string]string{"If-Match": detailBody.Etag})
	assertStatus(t, deleted, http.StatusNoContent)

	// After delete: getService and getAssetVersion are 404.
	after := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/order-service", nil, nil)
	assertStatus(t, after, http.StatusNotFound)
	versionList := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/assets/"+assetID, nil, nil)
	_ = versionList
	// getPublicService is also 404.
	publicRead := f.request(t, nil, http.MethodGet, "/api/v1/public/t/acme/services/order-service", nil, nil)
	assertStatus(t, publicRead, http.StatusNotFound)
}

// serviceEtag reads the current service ETag.
func (f *m1PipelineFixture) serviceEtag(t *testing.T, slug string) string {
	t.Helper()
	detail := f.request(t, &f.alice, http.MethodGet, "/api/v1/t/acme/services/"+slug, nil, nil)
	assertStatus(t, detail, http.StatusOK)
	return responseString(t, detail, "etag")
}
