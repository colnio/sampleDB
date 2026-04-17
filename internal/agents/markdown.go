package agents

import (
	"fmt"
	"regexp"
	"strings"
)

var imagePattern = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)

// rewriteRelativeImages rewrites relative image URLs in Markdown so they are
// prefixed with baseURL. Absolute URLs and paths already starting with '/' are
// left unchanged.
func rewriteRelativeImages(md, baseURL string) string {
	base := strings.TrimRight(baseURL, "/")
	return imagePattern.ReplaceAllStringFunc(md, func(match string) string {
		parts := imagePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		alt := parts[1]
		rawTarget := strings.TrimSpace(parts[2])
		if rawTarget == "" {
			return match
		}

		pathPart := rawTarget
		suffix := ""
		if split := strings.IndexAny(rawTarget, " \t"); split >= 0 {
			pathPart = rawTarget[:split]
			suffix = rawTarget[split:]
		}

		trimmedPath := strings.Trim(pathPart, "<>")
		if strings.HasPrefix(trimmedPath, "http://") ||
			strings.HasPrefix(trimmedPath, "https://") ||
			strings.HasPrefix(trimmedPath, "/") ||
			strings.HasPrefix(trimmedPath, "#") ||
			strings.HasPrefix(trimmedPath, "data:") {
			return match
		}

		trimmedPath = strings.TrimPrefix(trimmedPath, "./")
		trimmedPath = strings.TrimPrefix(trimmedPath, ".\\")
		qualified := fmt.Sprintf("%s/%s", base, trimmedPath)
		if strings.HasPrefix(pathPart, "<") && strings.HasSuffix(pathPart, ">") {
			qualified = "<" + qualified + ">"
		}
		return fmt.Sprintf("![%s](%s%s)", alt, qualified, suffix)
	})
}
