//go:build integration

package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"sampleDB/internal/wiki"
)

func TestWiki_CRUD(t *testing.T) {
	title := "Integration Test Article " + randomHex(4)

	// Create
	body := fmt.Sprintf(`{"title":%q,"content":"## Test\n\nThis article was created by an integration test."}`, title)
	resp := apiDo(t, http.MethodPost, "/api/v1/wiki", jsonBody(body))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/v1/wiki status = %d, want 201; body = %s", resp.StatusCode, mustReadBody(t, resp))
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Fatalf("POST /api/v1/wiki: missing Content-Type header")
	}

	var created wiki.APIArticleDetail
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		resp.Body.Close()
		t.Fatalf("decode create response: %v", err)
	}
	resp.Body.Close()
	if created.ID == 0 {
		t.Fatal("expected non-zero article ID after create")
	}
	if created.Title != title {
		t.Fatalf("title = %q, want %q", created.Title, title)
	}
	t.Cleanup(func() {
		pool.Exec(bgCtx, "DELETE FROM articles WHERE article_id = $1", created.ID)
	})

	// Read
	resp2 := apiDo(t, http.MethodGet, fmt.Sprintf("/api/v1/wiki/%d", created.ID), nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/wiki/%d status = %d, want 200", created.ID, resp2.StatusCode)
	}
	var detail wiki.APIArticleDetail
	if err := json.NewDecoder(resp2.Body).Decode(&detail); err != nil {
		resp2.Body.Close()
		t.Fatalf("decode get response: %v", err)
	}
	resp2.Body.Close()
	if detail.ID != created.ID {
		t.Fatalf("detail.ID = %d, want %d", detail.ID, created.ID)
	}
	if detail.Title != title {
		t.Fatalf("detail.Title = %q, want %q", detail.Title, title)
	}

	// Update
	updatedTitle := "Updated " + title
	updateBody := fmt.Sprintf(`{"title":%q,"content":"Updated content."}`, updatedTitle)
	resp3 := apiDo(t, http.MethodPut, fmt.Sprintf("/api/v1/wiki/%d", created.ID), jsonBody(updateBody))
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/v1/wiki/%d status = %d, want 200; body = %s", created.ID, resp3.StatusCode, mustReadBody(t, resp3))
	}
	var updated wiki.APIArticleDetail
	if err := json.NewDecoder(resp3.Body).Decode(&updated); err != nil {
		resp3.Body.Close()
		t.Fatalf("decode update response: %v", err)
	}
	resp3.Body.Close()
	if updated.Title != updatedTitle {
		t.Fatalf("updated title = %q, want %q", updated.Title, updatedTitle)
	}
	if updated.Content != "Updated content." {
		t.Fatalf("updated content = %q, want Updated content.", updated.Content)
	}
}

func TestWiki_List_FiltersByTag_HTMLPage(t *testing.T) {
	// Seed two articles with different tags directly via the DB so we can
	// exercise the HTML /wiki handler's ?tag= path. (The API doesn't accept
	// tags yet, so direct DB seeding is the cleanest route.)
	title1 := "inttest-safety-" + randomHex(4)
	title2 := "inttest-other-" + randomHex(4)

	var id1, id2 int
	if err := pool.QueryRow(bgCtx,
		`INSERT INTO articles (title, content, tags, created_by, last_modified_by)
		 VALUES ($1, 'Body about safety.', $2, $3, $3) RETURNING article_id`,
		title1, []string{"Safety"}, testUser.id).Scan(&id1); err != nil {
		t.Fatalf("seed safety article: %v", err)
	}
	if err := pool.QueryRow(bgCtx,
		`INSERT INTO articles (title, content, tags, created_by, last_modified_by)
		 VALUES ($1, 'Body about protocols.', $2, $3, $3) RETURNING article_id`,
		title2, []string{"Protocols"}, testUser.id).Scan(&id2); err != nil {
		t.Fatalf("seed protocols article: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(bgCtx, "DELETE FROM articles WHERE article_id IN ($1, $2)", id1, id2)
	})

	// Hit the HTML page with ?tag=Safety — should include title1 but not title2.
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/wiki?tag=Safety", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /wiki?tag=Safety: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, mustReadBody(t, resp))
	}
	body := mustReadBody(t, resp)
	if !strings.Contains(body, title1) {
		t.Errorf("expected safety article %q in response; body did not contain it", title1)
	}
	if strings.Contains(body, title2) {
		t.Errorf("unexpected protocols article %q leaked through with ?tag=Safety", title2)
	}
	if !strings.Contains(body, "filter-chip--tag") {
		t.Errorf("expected tag filter chips in rendered HTML")
	}
}

func TestWiki_List(t *testing.T) {
	resp := apiDo(t, http.MethodGet, "/api/v1/wiki", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/wiki status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Fatalf("missing Content-Type")
	}

	var envelope struct {
		Items  []wiki.APIArticleItem `json:"items"`
		Total  int                   `json:"total"`
		Limit  int                   `json:"limit"`
		Offset int                   `json:"offset"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if envelope.Limit <= 0 {
		t.Fatalf("envelope.Limit = %d, want > 0", envelope.Limit)
	}
}
