//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"sampleDB/internal/samples"
)

func TestSamples_CRUD(t *testing.T) {
	// Create
	body := `{"name":"integration-test-sample","description":"created by integration test","keywords":"test silicon","owner":"inttest","sample_prep":"none"}`
	resp := apiDo(t, http.MethodPost, "/api/v1/samples", jsonBody(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/samples status = %d, want 201; body = %s", resp.StatusCode, mustReadBody(t, resp))
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Fatalf("POST /api/v1/samples: missing Content-Type header")
	}

	var created samples.APISampleItem
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected non-zero sample ID after create")
	}
	if created.Name != "integration-test-sample" {
		t.Fatalf("name = %q, want integration-test-sample", created.Name)
	}
	t.Cleanup(func() {
		pool.Exec(bgCtx, "DELETE FROM samples WHERE sample_id = $1", created.ID)
	})

	// Read
	resp2 := apiDo(t, http.MethodGet, fmt.Sprintf("/api/v1/samples/%d", created.ID), nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/samples/%d status = %d, want 200", created.ID, resp2.StatusCode)
	}
	var detail samples.APISampleDetail
	if err := json.NewDecoder(resp2.Body).Decode(&detail); err != nil {
		resp2.Body.Close()
		t.Fatalf("decode get response: %v", err)
	}
	resp2.Body.Close()
	if detail.ID != created.ID {
		t.Fatalf("detail.ID = %d, want %d", detail.ID, created.ID)
	}
	if detail.Name != "integration-test-sample" {
		t.Fatalf("detail.Name = %q, want integration-test-sample", detail.Name)
	}

	// Update
	updateBody := `{"name":"updated-sample","description":"updated description","keywords":"test updated","owner":"inttest","sample_prep":"piranha clean"}`
	resp3 := apiDo(t, http.MethodPut, fmt.Sprintf("/api/v1/samples/%d", created.ID), jsonBody(updateBody))
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/samples/%d status = %d, want 200; body = %s", created.ID, resp3.StatusCode, mustReadBody(t, resp3))
	}
	var updated samples.APISampleDetail
	if err := json.NewDecoder(resp3.Body).Decode(&updated); err != nil {
		resp3.Body.Close()
		t.Fatalf("decode update response: %v", err)
	}
	resp3.Body.Close()
	if updated.Name != "updated-sample" {
		t.Fatalf("updated name = %q, want updated-sample", updated.Name)
	}
	if updated.SamplePrep != "piranha clean" {
		t.Fatalf("updated sample_prep = %q, want piranha clean", updated.SamplePrep)
	}

	// Delete
	resp4 := apiDo(t, http.MethodDelete, fmt.Sprintf("/api/v1/samples/%d", created.ID), nil)
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE /api/v1/samples/%d status = %d, want 204", created.ID, resp4.StatusCode)
	}

	// Confirm gone
	resp5 := apiDo(t, http.MethodGet, fmt.Sprintf("/api/v1/samples/%d", created.ID), nil)
	resp5.Body.Close()
	if resp5.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after delete: status = %d, want 404", resp5.StatusCode)
	}
}

func TestSamples_List(t *testing.T) {
	resp := apiDo(t, http.MethodGet, "/api/v1/samples", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/samples status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var envelope struct {
		Items  []samples.APISampleItem `json:"items"`
		Total  int                     `json:"total"`
		Limit  int                     `json:"limit"`
		Offset int                     `json:"offset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if envelope.Limit <= 0 {
		t.Fatalf("envelope.Limit = %d, want > 0", envelope.Limit)
	}
}

func TestSamples_Unauthenticated(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/samples", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", resp.StatusCode)
	}
}
