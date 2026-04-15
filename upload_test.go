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

// withUploadSession adds an authenticated session AND sets HX-Request header.
func withUploadSession(req *http.Request) *http.Request {
	req.Header.Set("HX-Request", "true")
	return withTestSession(req)
}

// ---------------------------------------------------------------------------
// Multi-file upload handler — requires real DB
// ---------------------------------------------------------------------------

func TestUploadAttachment_MultiFile(t *testing.T) {
	pool := testDB(t)

	// Replace global pool
	orig := dbPool
	dbPool = pool
	t.Cleanup(func() { dbPool = orig })

	// Use a temp dir so the test doesn't litter the repo uploads/ directory.
	tmpDir := t.TempDir()
	origUploads := appConfig.UploadsDir
	appConfig.UploadsDir = tmpDir
	t.Cleanup(func() { appConfig.UploadsDir = origUploads })

	setTestTemplatesDir(t)

	// Create a sample to attach to.
	sampleName := fmt.Sprintf("upload-test-%d", time.Now().UnixNano())
	var sampleID int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner) VALUES ($1, 'tester') RETURNING sample_id`, sampleName).Scan(&sampleID)
	if err != nil {
		t.Fatalf("create test sample: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
	})

	url := fmt.Sprintf("/samples/%d/upload", sampleID)
	req := buildMultipartRequest(t, url, []struct{ name, content, mime string }{
		{"report.txt", "file one contents", "text/plain"},
		{"data.csv", "a,b,c", "text/csv"},
	})
	req = withUploadSession(req)

	rr := httptest.NewRecorder()
	uploadAttachmentHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned unexpected status %d: %s", rr.Code, rr.Body.String())
	}

	// Verify two attachment rows were created.
	var count int
	err = pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM attachments WHERE source_id = $1 AND source_type = 'sample'`,
		fmt.Sprintf("%d", sampleID)).Scan(&count)
	if err != nil {
		t.Fatalf("count attachments: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 attachment rows, got %d", count)
	}

	// Verify the files were saved on disk.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) < 2 {
		t.Errorf("expected at least 2 files on disk, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// Paste upload — image blob, same endpoint
// ---------------------------------------------------------------------------

func TestUploadAttachment_PasteImage(t *testing.T) {
	pool := testDB(t)

	orig := dbPool
	dbPool = pool
	t.Cleanup(func() { dbPool = orig })

	tmpDir := t.TempDir()
	origUploads := appConfig.UploadsDir
	appConfig.UploadsDir = tmpDir
	t.Cleanup(func() { appConfig.UploadsDir = origUploads })

	setTestTemplatesDir(t)

	sampleName := fmt.Sprintf("paste-test-%d", time.Now().UnixNano())
	var sampleID int
	err := pool.QueryRow(context.Background(),
		`INSERT INTO samples (sample_name, sample_owner) VALUES ($1, 'tester') RETURNING sample_id`, sampleName).Scan(&sampleID)
	if err != nil {
		t.Fatalf("create test sample: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM samples WHERE sample_id = $1", sampleID)
	})

	// Simulate a paste upload: one image/png blob named "image.png".
	url := fmt.Sprintf("/samples/%d/upload", sampleID)
	req := buildMultipartRequest(t, url, []struct{ name, content, mime string }{
		{"image.png", "\x89PNG\r\n\x1a\n", "image/png"}, // minimal PNG header bytes
	})
	req = withUploadSession(req)

	rr := httptest.NewRecorder()
	uploadAttachmentHandler(rr, req)

	if rr.Code == http.StatusMethodNotAllowed || rr.Code == http.StatusBadRequest {
		t.Fatalf("handler error status %d: %s", rr.Code, rr.Body.String())
	}

	var count int
	err = pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM attachments WHERE source_id = $1 AND source_type = 'sample'`,
		fmt.Sprintf("%d", sampleID)).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 attachment, got %d", count)
	}

	// Verify file exists on disk with .png extension.
	entries, _ := os.ReadDir(tmpDir)
	var found bool
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no .png file found in upload dir; got: %v", func() []string {
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			return names
		}())
	}
}

// ---------------------------------------------------------------------------
// No-files guard — handler must reject empty "files" field
// ---------------------------------------------------------------------------

func TestUploadAttachment_NoFiles(t *testing.T) {
	// This test does not need a DB — the handler returns before touching it.
	setTestTemplatesDir(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/samples/1/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withTestSession(req)

	rr := httptest.NewRecorder()
	uploadAttachmentHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty upload, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Wiki multi-file upload handler — requires real DB
// ---------------------------------------------------------------------------

func TestUploadWikiAttachment_MultiFile(t *testing.T) {
	pool := testDB(t)

	orig := dbPool
	dbPool = pool
	t.Cleanup(func() { dbPool = orig })

	tmpDir := t.TempDir()
	origUploads := appConfig.UploadsDir
	appConfig.UploadsDir = tmpDir
	t.Cleanup(func() { appConfig.UploadsDir = origUploads })

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
	origUploads := appConfig.UploadsDir
	appConfig.UploadsDir = tmpDir
	t.Cleanup(func() { appConfig.UploadsDir = origUploads })

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
