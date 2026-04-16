package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
