package wiki_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v3"

	"sampleDB/internal/web"
	"sampleDB/internal/wiki"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newSvc(t *testing.T) (wiki.Service, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(mock.Close)
	return wiki.NewService(mock), mock
}

// ---------------------------------------------------------------------------
// ListArticles
// ---------------------------------------------------------------------------

func TestListArticles_ReturnsTwoRows(t *testing.T) {
	svc, mock := newSvc(t)

	now := time.Now()
	mock.ExpectQuery(`SELECT article_id, title, created_at FROM articles`).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "title", "created_at"}).
			AddRow(1, "First", now).
			AddRow(2, "Second", now))

	articles, err := svc.ListArticles(context.Background())
	if err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if len(articles) != 2 {
		t.Errorf("expected 2 articles, got %d", len(articles))
	}
	if articles[0].Title != "First" {
		t.Errorf("expected first article title %q, got %q", "First", articles[0].Title)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestListArticles_EmptyResult(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT article_id, title, created_at FROM articles`).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "title", "created_at"}))

	articles, err := svc.ListArticles(context.Background())
	if err != nil {
		t.Fatalf("ListArticles: %v", err)
	}
	if len(articles) != 0 {
		t.Errorf("expected 0 articles, got %d", len(articles))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetArticle
// ---------------------------------------------------------------------------

func TestGetArticle_Found(t *testing.T) {
	svc, mock := newSvc(t)

	now := time.Now()
	mock.ExpectQuery(`SELECT article_id, title, content`).
		WithArgs("My Article").
		WillReturnRows(pgxmock.NewRows([]string{
			"article_id", "title", "content", "created_at", "created_by",
			"last_modified_at", "last_modified_by",
		}).AddRow(42, "My Article", "# Hello", now, 1, now, 1))

	mock.ExpectQuery(`SELECT attachment_id, original_name`).
		WithArgs(42).
		WillReturnRows(pgxmock.NewRows([]string{"attachment_id", "original_name", "attachment_address", "uploaded_at"}))

	article, err := svc.GetArticle(context.Background(), "My Article")
	if err != nil {
		t.Fatalf("GetArticle: %v", err)
	}
	if article.ID != 42 {
		t.Errorf("expected ID 42, got %d", article.ID)
	}
	if article.Content.Raw != "# Hello" {
		t.Errorf("expected raw content %q, got %q", "# Hello", article.Content.Raw)
	}
	if article.Content.HTML == "" {
		t.Error("expected rendered HTML to be non-empty")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestGetArticle_NotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT article_id, title, content`).
		WithArgs("Missing").
		WillReturnError(pgx.ErrNoRows)

	_, err := svc.GetArticle(context.Background(), "Missing")
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateArticle
// ---------------------------------------------------------------------------

func TestCreateArticle_InsertsRow(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`INSERT INTO articles`).
		WithArgs("New Title", "some content", 7).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := svc.CreateArticle(context.Background(), "New Title", "some content", 7); err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateArticleContent
// ---------------------------------------------------------------------------

func TestUpdateArticleContent_UpdatesRow(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE articles SET content`).
		WithArgs("new body", 7, "My Article").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	if err := svc.UpdateArticleContent(context.Background(), "My Article", "new body", 7); err != nil {
		t.Fatalf("UpdateArticleContent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteArticle
// ---------------------------------------------------------------------------

func TestDeleteArticle_ReturnsFilePaths(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT aa.attachment_address`).
		WithArgs("Old Article").
		WillReturnRows(pgxmock.NewRows([]string{"attachment_address"}).
			AddRow("uploads/a.png").
			AddRow("uploads/b.pdf"))

	mock.ExpectExec(`DELETE FROM articles WHERE title`).
		WithArgs("Old Article").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	paths, err := svc.DeleteArticle(context.Background(), "Old Article")
	if err != nil {
		t.Fatalf("DeleteArticle: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 file paths, got %d", len(paths))
	}
	if paths[0] != "uploads/a.png" {
		t.Errorf("expected first path %q, got %q", "uploads/a.png", paths[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ArticleEditAccess
// ---------------------------------------------------------------------------

func TestArticleEditAccess_CreatorNonAdmin(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT a.created_by, COALESCE\(u.admin`).
		WithArgs(5, "My Article").
		WillReturnRows(pgxmock.NewRows([]string{"created_by", "admin"}).AddRow(5, false))

	createdBy, isAdmin, err := svc.ArticleEditAccess(context.Background(), "My Article", 5)
	if err != nil {
		t.Fatalf("ArticleEditAccess: %v", err)
	}
	if createdBy != 5 {
		t.Errorf("expected createdBy 5, got %d", createdBy)
	}
	if isAdmin {
		t.Error("expected isAdmin false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

func TestArticleEditAccess_NonCreatorAdmin(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT a.created_by, COALESCE\(u.admin`).
		WithArgs(99, "My Article").
		WillReturnRows(pgxmock.NewRows([]string{"created_by", "admin"}).AddRow(3, true))

	createdBy, isAdmin, err := svc.ArticleEditAccess(context.Background(), "My Article", 99)
	if err != nil {
		t.Fatalf("ArticleEditAccess: %v", err)
	}
	if createdBy != 3 {
		t.Errorf("expected createdBy 3, got %d", createdBy)
	}
	if !isAdmin {
		t.Error("expected isAdmin true")
	}
}

// ---------------------------------------------------------------------------
// IsAdmin
// ---------------------------------------------------------------------------

func TestIsAdmin_True(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT COALESCE\(admin, false\) FROM users`).
		WithArgs(1).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(true))

	isAdmin, err := svc.IsAdmin(context.Background(), 1)
	if err != nil {
		t.Fatalf("IsAdmin: %v", err)
	}
	if !isAdmin {
		t.Error("expected isAdmin true")
	}
}

func TestIsAdmin_False(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT COALESCE\(admin, false\) FROM users`).
		WithArgs(2).
		WillReturnRows(pgxmock.NewRows([]string{"admin"}).AddRow(false))

	isAdmin, err := svc.IsAdmin(context.Background(), 2)
	if err != nil {
		t.Fatalf("IsAdmin: %v", err)
	}
	if isAdmin {
		t.Error("expected isAdmin false")
	}
}

// ---------------------------------------------------------------------------
// AddAttachment
// ---------------------------------------------------------------------------

func TestAddAttachment_InsertsRow(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`INSERT INTO article_attachments`).
		WithArgs("42", "uploads/x.png", "original.png", 7).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	if err := svc.AddAttachment(context.Background(), "42", "uploads/x.png", "original.png", 7); err != nil {
		t.Fatalf("AddAttachment: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet mock expectations: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetAttachmentInfo
// ---------------------------------------------------------------------------

func TestGetAttachmentInfo_Found(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT attachment_address, original_name`).
		WithArgs("10").
		WillReturnRows(pgxmock.NewRows([]string{"attachment_address", "original_name"}).
			AddRow("uploads/file.pdf", "report.pdf"))

	fp, orig, err := svc.GetAttachmentInfo(context.Background(), "10")
	if err != nil {
		t.Fatalf("GetAttachmentInfo: %v", err)
	}
	if fp != "uploads/file.pdf" {
		t.Errorf("expected path %q, got %q", "uploads/file.pdf", fp)
	}
	if orig != "report.pdf" {
		t.Errorf("expected original %q, got %q", "report.pdf", orig)
	}
}

func TestGetAttachmentInfo_NotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT attachment_address, original_name`).
		WithArgs("99").
		WillReturnError(pgx.ErrNoRows)

	_, _, err := svc.GetAttachmentInfo(context.Background(), "99")
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// DeleteAttachment
// ---------------------------------------------------------------------------

func TestDeleteAttachment_ReturnsPathAndTitle(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT aa.attachment_address, a.title`).
		WithArgs("5").
		WillReturnRows(pgxmock.NewRows([]string{"attachment_address", "title"}).
			AddRow("uploads/img.jpg", "My Article"))

	mock.ExpectExec(`DELETE FROM article_attachments`).
		WithArgs("5").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))

	title, fp, err := svc.DeleteAttachment(context.Background(), "5")
	if err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	if title != "My Article" {
		t.Errorf("expected title %q, got %q", "My Article", title)
	}
	if fp != "uploads/img.jpg" {
		t.Errorf("expected path %q, got %q", "uploads/img.jpg", fp)
	}
}

// ---------------------------------------------------------------------------
// GetArticleTitle
// ---------------------------------------------------------------------------

func TestGetArticleTitle(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT title FROM articles`).
		WithArgs("7").
		WillReturnRows(pgxmock.NewRows([]string{"title"}).AddRow("Wiki Home"))

	title, err := svc.GetArticleTitle(context.Background(), "7")
	if err != nil {
		t.Fatalf("GetArticleTitle: %v", err)
	}
	if title != "Wiki Home" {
		t.Errorf("expected %q, got %q", "Wiki Home", title)
	}
}

// ---------------------------------------------------------------------------
// APIListArticles
// ---------------------------------------------------------------------------

func TestAPIListArticles_Pagination(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM articles`).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(42))

	now := time.Now()
	mock.ExpectQuery(`SELECT article_id, title, created_at`).
		WithArgs(10, 0).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "title", "created_at", "last_modified_at"}).
			AddRow(1, "A", now, now).
			AddRow(2, "B", now, now))

	items, total, err := svc.APIListArticles(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("APIListArticles: %v", err)
	}
	if total != 42 {
		t.Errorf("expected total 42, got %d", total)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// APIGetArticle
// ---------------------------------------------------------------------------

func TestAPIGetArticle_Found(t *testing.T) {
	svc, mock := newSvc(t)

	now := time.Now()
	mock.ExpectQuery(`SELECT article_id, title, content, created_at`).
		WithArgs(3).
		WillReturnRows(pgxmock.NewRows([]string{
			"article_id", "title", "content", "created_at", "last_modified_at",
		}).AddRow(3, "Test", "body", now, now))

	mock.ExpectQuery(`SELECT attachment_id, original_name, attachment_address, uploaded_at`).
		WithArgs(3).
		WillReturnRows(pgxmock.NewRows([]string{"attachment_id", "original_name", "attachment_address", "uploaded_at"}))

	detail, err := svc.APIGetArticle(context.Background(), 3)
	if err != nil {
		t.Fatalf("APIGetArticle: %v", err)
	}
	if detail.ID != 3 {
		t.Errorf("expected ID 3, got %d", detail.ID)
	}
	if detail.Title != "Test" {
		t.Errorf("expected title %q, got %q", "Test", detail.Title)
	}
	if detail.Attachments == nil {
		t.Error("expected non-nil Attachments slice")
	}
}

func TestAPIGetArticle_NotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT article_id, title, content, created_at`).
		WithArgs(999).
		WillReturnError(pgx.ErrNoRows)

	_, err := svc.APIGetArticle(context.Background(), 999)
	if !errors.Is(err, web.ErrNotFound) {
		t.Errorf("expected web.ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// APICreateArticle
// ---------------------------------------------------------------------------

func TestAPICreateArticle_ReturnsDetail(t *testing.T) {
	svc, mock := newSvc(t)

	now := time.Now()
	mock.ExpectQuery(`INSERT INTO articles`).
		WithArgs("New", "content", 1).
		WillReturnRows(pgxmock.NewRows([]string{"article_id", "created_at"}).AddRow(10, now))

	detail, err := svc.APICreateArticle(context.Background(), "New", "content", 1)
	if err != nil {
		t.Fatalf("APICreateArticle: %v", err)
	}
	if detail.ID != 10 {
		t.Errorf("expected ID 10, got %d", detail.ID)
	}
	if detail.Title != "New" {
		t.Errorf("expected title %q, got %q", "New", detail.Title)
	}
	if detail.Attachments == nil {
		t.Error("expected non-nil Attachments slice")
	}
}

// ---------------------------------------------------------------------------
// APIUpdateArticle
// ---------------------------------------------------------------------------

func TestAPIUpdateArticle_UpdatesRow(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE articles`).
		WithArgs("Updated Title", "new body", 5, 7).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	n, err := svc.APIUpdateArticle(context.Background(), 7, "Updated Title", "new body", 5)
	if err != nil {
		t.Fatalf("APIUpdateArticle: %v", err)
	}
	if n != 1 {
		t.Errorf("expected rowsAffected 1, got %d", n)
	}
}

func TestAPIUpdateArticle_NotFound(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectExec(`UPDATE articles`).
		WithArgs("Title", "body", 5, 999).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	n, err := svc.APIUpdateArticle(context.Background(), 999, "Title", "body", 5)
	if err != nil {
		t.Fatalf("APIUpdateArticle: %v", err)
	}
	if n != 0 {
		t.Errorf("expected rowsAffected 0, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// APIArticleEditAccess
// ---------------------------------------------------------------------------

func TestAPIArticleEditAccess_OwnerNonAdmin(t *testing.T) {
	svc, mock := newSvc(t)

	mock.ExpectQuery(`SELECT a.created_by, COALESCE\(u.admin`).
		WithArgs(5, 10).
		WillReturnRows(pgxmock.NewRows([]string{"created_by", "admin"}).AddRow(5, false))

	createdBy, isAdmin, err := svc.APIArticleEditAccess(context.Background(), 10, 5)
	if err != nil {
		t.Fatalf("APIArticleEditAccess: %v", err)
	}
	if createdBy != 5 {
		t.Errorf("expected createdBy 5, got %d", createdBy)
	}
	if isAdmin {
		t.Error("expected isAdmin false")
	}
}

// ---------------------------------------------------------------------------
// Preview handler
// ---------------------------------------------------------------------------

func TestPreview_JSONInput(t *testing.T) {
	h := wiki.NewHandlers(nil, nil, wiki.HandlerConfig{})

	body := `{"content": "# Hello"}`
	req := httptest.NewRequest(http.MethodPost, "/wiki/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.Preview(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	got := rr.Body.String()
	if !strings.Contains(got, "<h1") {
		t.Errorf("expected rendered HTML with <h1>, got: %s", got)
	}
}

func TestPreview_FormInput(t *testing.T) {
	h := wiki.NewHandlers(nil, nil, wiki.HandlerConfig{})

	body := "content=%23+Hello"
	req := httptest.NewRequest(http.MethodPost, "/wiki/preview", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	h.Preview(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPreview_MethodNotAllowed(t *testing.T) {
	h := wiki.NewHandlers(nil, nil, wiki.HandlerConfig{})

	req := httptest.NewRequest(http.MethodGet, "/wiki/preview", nil)
	rr := httptest.NewRecorder()

	h.Preview(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rr.Code)
	}
}
