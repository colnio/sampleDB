package wiki_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sampleDB/internal/auth"
	"sampleDB/internal/wiki"
)

type wikiAPIServiceStub struct {
	wiki.Service

	apiListArticles      func(ctx context.Context, limit, offset int) ([]wiki.APIArticleItem, int, error)
	apiGetArticle        func(ctx context.Context, id int) (wiki.APIArticleDetail, error)
	apiCreateArticle     func(ctx context.Context, title, content string, createdBy int) (wiki.APIArticleDetail, error)
	apiUpdateArticle     func(ctx context.Context, id int, title, content string, modifiedBy int) (int64, error)
	apiArticleEditAccess func(ctx context.Context, id int, userID int) (int, bool, error)
}

func (s *wikiAPIServiceStub) APIListArticles(ctx context.Context, limit, offset int) ([]wiki.APIArticleItem, int, error) {
	return s.apiListArticles(ctx, limit, offset)
}

func (s *wikiAPIServiceStub) APIGetArticle(ctx context.Context, id int) (wiki.APIArticleDetail, error) {
	return s.apiGetArticle(ctx, id)
}

func (s *wikiAPIServiceStub) APICreateArticle(ctx context.Context, title, content string, createdBy int) (wiki.APIArticleDetail, error) {
	return s.apiCreateArticle(ctx, title, content, createdBy)
}

func (s *wikiAPIServiceStub) APIUpdateArticle(ctx context.Context, id int, title, content string, modifiedBy int) (int64, error) {
	return s.apiUpdateArticle(ctx, id, title, content, modifiedBy)
}

func (s *wikiAPIServiceStub) APIArticleEditAccess(ctx context.Context, id int, userID int) (int, bool, error) {
	return s.apiArticleEditAccess(ctx, id, userID)
}

func authenticatedWikiRequest(method, target string, body io.Reader, contentType string, session auth.Session) *http.Request {
	req := httptest.NewRequest(method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req.WithContext(auth.ContextWithSession(req.Context(), session))
}

func decodeWikiResponse[T any](t *testing.T, rr *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

func TestAPIWiki_ListArticles_ReturnsEnvelope(t *testing.T) {
	h := wiki.NewHandlers(&wikiAPIServiceStub{
		apiListArticles: func(_ context.Context, limit, offset int) ([]wiki.APIArticleItem, int, error) {
			if limit != 25 || offset != 50 {
				t.Fatalf("limit=%d offset=%d, want 25/50", limit, offset)
			}
			return []wiki.APIArticleItem{{
				ID:             1,
				Title:          "RCA Clean Protocol",
				CreatedAt:      time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC),
				LastModifiedAt: time.Date(2026, 3, 1, 14, 30, 0, 0, time.UTC),
			}}, 7, nil
		},
	}, nil, wiki.HandlerConfig{})

	req := authenticatedWikiRequest(http.MethodGet, "/api/v1/wiki?limit=25&offset=50", nil, "", auth.Session{UserID: 1, Username: "tester"})
	rr := httptest.NewRecorder()
	h.APIWiki(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	resp := decodeWikiResponse[struct {
		Items  []wiki.APIArticleItem `json:"items"`
		Total  int                   `json:"total"`
		Limit  int                   `json:"limit"`
		Offset int                   `json:"offset"`
	}](t, rr)
	if len(resp.Items) != 1 || resp.Total != 7 || resp.Limit != 25 || resp.Offset != 50 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestAPIWiki_GetArticle_ReturnsDetail(t *testing.T) {
	h := wiki.NewHandlers(&wikiAPIServiceStub{
		apiGetArticle: func(_ context.Context, id int) (wiki.APIArticleDetail, error) {
			if id != 5 {
				t.Fatalf("APIGetArticle called with id=%d, want 5", id)
			}
			return wiki.APIArticleDetail{
				ID:             5,
				Title:          "RCA Clean Protocol",
				Content:        "## Overview",
				CreatedAt:      time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC),
				LastModifiedAt: time.Date(2026, 3, 1, 14, 30, 0, 0, time.UTC),
				Attachments: []wiki.APIArticleAttachment{{
					ID:           8,
					OriginalName: "diagram.png",
					URL:          "/wiki/attachment/8",
					IsImage:      true,
					UploadedAt:   time.Date(2026, 1, 10, 9, 5, 0, 0, time.UTC),
				}},
			}, nil
		},
	}, nil, wiki.HandlerConfig{})

	req := authenticatedWikiRequest(http.MethodGet, "/api/v1/wiki/5", nil, "", auth.Session{UserID: 1, Username: "tester"})
	rr := httptest.NewRecorder()
	h.APIWiki(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	resp := decodeWikiResponse[wiki.APIArticleDetail](t, rr)
	if resp.Title != "RCA Clean Protocol" || len(resp.Attachments) != 1 {
		t.Fatalf("unexpected detail: %#v", resp)
	}
}

func TestAPIWiki_CreateArticle_UsesAuthenticatedUser(t *testing.T) {
	h := wiki.NewHandlers(&wikiAPIServiceStub{
		apiCreateArticle: func(_ context.Context, title, content string, createdBy int) (wiki.APIArticleDetail, error) {
			if title != "RCA Clean Protocol" || content != "## Overview" || createdBy != 17 {
				t.Fatalf("unexpected create args: title=%q content=%q createdBy=%d", title, content, createdBy)
			}
			return wiki.APIArticleDetail{
				ID:      9,
				Title:   title,
				Content: content,
			}, nil
		},
	}, nil, wiki.HandlerConfig{})

	req := authenticatedWikiRequest(http.MethodPost, "/api/v1/wiki", strings.NewReader(`{"title":"RCA Clean Protocol","content":"## Overview"}`), "application/json", auth.Session{UserID: 17, Username: "alice"})
	rr := httptest.NewRecorder()
	h.APIWiki(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rr.Code, rr.Body.String())
	}
	resp := decodeWikiResponse[wiki.APIArticleDetail](t, rr)
	if resp.ID != 9 || resp.Title != "RCA Clean Protocol" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestAPIWiki_UpdateArticle_ReturnsUpdatedDetail(t *testing.T) {
	h := wiki.NewHandlers(&wikiAPIServiceStub{
		apiArticleEditAccess: func(_ context.Context, id int, userID int) (int, bool, error) {
			if id != 9 || userID != 17 {
				t.Fatalf("unexpected edit access args: id=%d userID=%d", id, userID)
			}
			return 17, false, nil
		},
		apiUpdateArticle: func(_ context.Context, id int, title, content string, modifiedBy int) (int64, error) {
			if id != 9 || title != "Updated Title" || content != "Updated content" || modifiedBy != 17 {
				t.Fatalf("unexpected update args: id=%d title=%q content=%q modifiedBy=%d", id, title, content, modifiedBy)
			}
			return 1, nil
		},
		apiGetArticle: func(_ context.Context, id int) (wiki.APIArticleDetail, error) {
			return wiki.APIArticleDetail{
				ID:      id,
				Title:   "Updated Title",
				Content: "Updated content",
			}, nil
		},
	}, nil, wiki.HandlerConfig{})

	req := authenticatedWikiRequest(http.MethodPut, "/api/v1/wiki/9", strings.NewReader(`{"title":"Updated Title","content":"Updated content"}`), "application/json", auth.Session{UserID: 17, Username: "alice"})
	rr := httptest.NewRecorder()
	h.APIWiki(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	resp := decodeWikiResponse[wiki.APIArticleDetail](t, rr)
	if resp.ID != 9 || resp.Title != "Updated Title" || resp.Content != "Updated content" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
