package web

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidPath is returned when a URL path does not match the expected shape.
var ErrInvalidPath = errors.New("invalid path")

// PathSegments splits urlPath into non-empty segments after stripping trimPrefix
// and any surrounding slashes.
// Example: PathSegments("/samples/123/upload", "/samples/") → ["123", "upload"]
func PathSegments(urlPath, trimPrefix string) []string {
	path := strings.TrimPrefix(urlPath, trimPrefix)
	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// ExtractIDFromPath returns the first path segment after trimPrefix.
// Returns ErrInvalidPath if no segment is present.
// Example: ExtractIDFromPath("/samples/42", "/samples/") → "42"
func ExtractIDFromPath(urlPath, trimPrefix string) (string, error) {
	segs := PathSegments(urlPath, trimPrefix)
	if len(segs) == 0 {
		return "", ErrInvalidPath
	}
	return segs[0], nil
}

// ExtractIDAndAction returns the first two path segments after trimPrefix.
// The second segment (action) is empty when the path contains only one segment.
// Example: ExtractIDAndAction("/steps/123/edit", "/steps/") → ("123", "edit", nil)
func ExtractIDAndAction(urlPath, trimPrefix string) (id, action string, err error) {
	segs := PathSegments(urlPath, trimPrefix)
	if len(segs) == 0 {
		return "", "", ErrInvalidPath
	}
	id = segs[0]
	if len(segs) > 1 {
		action = segs[1]
	}
	return id, action, nil
}

// ExtractArticleTitle extracts and URL-unescapes the first path segment after
// trimPrefix.  Useful for wiki routes where the title is embedded in the URL.
// Example: ExtractArticleTitle("/wiki/delete/My%20Article", "/wiki/delete/") → "My Article"
func ExtractArticleTitle(urlPath, trimPrefix string) (string, error) {
	segs := PathSegments(urlPath, trimPrefix)
	if len(segs) == 0 {
		return "", ErrInvalidPath
	}
	return url.PathUnescape(segs[0])
}
