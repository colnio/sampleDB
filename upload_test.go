package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildMultipartRequest builds a POST request with multiple files in the "files" field.
func buildMultipartRequest(t *testing.T, url string, files []struct{ name, content, mime string }) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="files"; filename="%s"`, f.name)}
		if f.mime != "" {
			h["Content-Type"] = []string{f.mime}
		}
		part, err := mw.CreatePart(h)
		if err != nil {
			t.Fatalf("create part: %v", err)
		}
		if _, err := io.WriteString(part, f.content); err != nil {
			t.Fatalf("write part: %v", err)
		}
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, url, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// ---------------------------------------------------------------------------
// Wiki multi-file upload handler — requires real DB
// ---------------------------------------------------------------------------

func TestUploadWikiAttachment_MultiFile(t *testing.T) {
	pool := testDB(t)

	orig := theApp.DB
	theApp.DB = pool
	t.Cleanup(func() { theApp.DB = orig })

	tmpDir := t.TempDir()
	origUploads := theApp.Config.UploadsDir
	theApp.Config.UploadsDir = tmpDir
	t.Cleanup(func() { theApp.Config.UploadsDir = origUploads })

	// Create a wiki article to attach to.
	articleTitle := fmt.Sprintf("upload-wiki-test-%d", time.Now().UnixNano())
	var articleID int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO articles (title, content, created_by) VALUES ($1, '', 1) RETURNING article_id`,
		articleTitle).Scan(&articleID)
	if err != nil {
		t.Fatalf("create test article: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM articles WHERE article_id = $1", articleID)
	})

	urlPath := fmt.Sprintf("/wiki/upload/%d", articleID)
	req := buildMultipartRequest(t, urlPath, []struct{ name, content, mime string }{
		{"note.txt", "hello wiki", "text/plain"},
		{"diagram.svg", "<svg/>", "image/svg+xml"},
	})
	req = withTestSession(req)

	rr := httptest.NewRecorder()
	uploadArticleAttachmentHandler(rr, req)

	// The handler redirects (302) on success.
	if rr.Code != http.StatusSeeOther && rr.Code != http.StatusFound && rr.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	err = pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM article_attachments WHERE article_id = $1`, articleID).Scan(&count)
	if err != nil {
		t.Fatalf("count wiki attachments: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 wiki attachment rows, got %d", count)
	}

	entries, _ := os.ReadDir(tmpDir)
	if len(entries) < 2 {
		t.Errorf("expected at least 2 files on disk, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// Filename helpers used by upload — pure unit tests, no DB needed
// ---------------------------------------------------------------------------

func TestSaveUploadedFile_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	origUploads := theApp.Config.UploadsDir
	theApp.Config.UploadsDir = tmpDir
	t.Cleanup(func() { theApp.Config.UploadsDir = origUploads })

	content := "test file content"
	fp, err := saveUploadedFile(strings.NewReader(content), "testfile.txt")
	if err != nil {
		t.Fatalf("saveUploadedFile: %v", err)
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	if string(data) != content {
		t.Errorf("saved content = %q, want %q", string(data), content)
	}

	// File must be inside the configured uploads dir.
	rel, err := filepath.Rel(tmpDir, fp)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Errorf("saved file %q is outside uploads dir %q", fp, tmpDir)
	}
}
