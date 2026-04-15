package main

import (
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// isImage — unit tests
// ---------------------------------------------------------------------------

func TestIsImage_ImageTypes(t *testing.T) {
	imageMIMEs := []string{
		"image/png",
		"image/jpeg",
		"image/gif",
		"image/webp",
		"image/svg+xml",
		"image/bmp",
		"image/tiff",
	}
	for _, ct := range imageMIMEs {
		if !isImage(ct) {
			t.Errorf("isImage(%q) = false, want true", ct)
		}
	}
}

func TestIsImage_NonImageTypes(t *testing.T) {
	nonImageMIMEs := []string{
		"text/plain",
		"text/csv",
		"application/pdf",
		"application/zip",
		"application/octet-stream",
		"video/mp4",
		"audio/mpeg",
		"",
	}
	for _, ct := range nonImageMIMEs {
		if isImage(ct) {
			t.Errorf("isImage(%q) = true, want false", ct)
		}
	}
}

// ---------------------------------------------------------------------------
// Content-Type for attachment extensions — verifies mime detection used by
// getAttachments() when setting att.IsImage, and by http.ServeFile for the
// download handler.
// ---------------------------------------------------------------------------

func TestAttachmentMimeByExtension(t *testing.T) {
	cases := []struct {
		ext     string
		wantImg bool
	}{
		{".png", true},
		{".jpg", true},
		{".jpeg", true},
		{".gif", true},
		{".webp", true},
		{".svg", true},
		{".txt", false},
		{".pdf", false},
		{".csv", false},
		{".zip", false},
	}

	for _, tc := range cases {
		ct := mime.TypeByExtension(tc.ext)
		got := isImage(ct)
		if got != tc.wantImg {
			t.Errorf("ext %q: mime=%q isImage=%v, want %v", tc.ext, ct, got, tc.wantImg)
		}
	}
}

// TestServeFileContentType verifies that http.ServeFile (used by
// downloadAttachmentHandler) sets an image/* Content-Type for PNG files,
// which is what the gallery relies on.
func TestServeFileContentType(t *testing.T) {
	// Minimal 1×1 PNG (89 bytes).
	pngBytes := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}

	tmpDir := t.TempDir()
	pngPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(pngPath, pngBytes, 0600); err != nil {
		t.Fatalf("write temp png: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/test.png", nil)
	rr := httptest.NewRecorder()
	http.ServeFile(rr, req, pngPath)

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		t.Errorf("expected image/* Content-Type for .png, got %q", ct)
	}
	if !isImage(ct) {
		t.Errorf("isImage(%q) = false for served PNG, want true", ct)
	}
}

// NOTE: gallery.js behaviour (lightbox open/close/navigation) relies on DOM
// manipulation and is verified manually:
//   - Click a thumbnail in the sample detail or wiki page → modal opens.
//   - Arrow keys / prev/next buttons navigate between images.
//   - ESC key or overlay click closes the modal.
//   - Uploading or deleting an attachment (HTMX swap) re-registers gallery
//     handlers on the new thumbnails.
