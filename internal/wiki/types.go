// Package wiki provides the domain types, service, and HTTP handlers for the
// wiki feature (article CRUD, attachments, and markdown preview).
package wiki

import (
	"html/template"
	"time"
)

// Article represents a wiki article.
type Article struct {
	ID             int
	Title          string
	Content        ArticleContent
	Tags           []string
	CreatedBy      int
	CreatorName    string
	CreatedAt      time.Time
	LastModifiedAt time.Time
	LastModifiedBy int
	Excerpt        string
	Attachments    []ArticleAttachment
}

// ArticleAttachment is a file attached to a wiki article.
type ArticleAttachment struct {
	IsImage      bool
	ID           int
	ArticleID    int
	Address      string
	OriginalName string
	UploadedAt   time.Time
	UploadedBy   int
}

// ArticleContent holds both the raw Markdown source and the rendered HTML.
type ArticleContent struct {
	Raw  string
	HTML template.HTML
}

// BasePageData holds session-derived fields shared by all page templates.
type BasePageData struct {
	Username string
	UserID   int
	IsAdmin  bool
}

// WikiListPageData is the template data for the article list page.
type WikiListPageData struct {
	BasePageData
	Articles  []Article
	Tags      []string // tag chip vocabulary (stable set: All + known)
	TagFilter string   // currently-applied tag filter ("" = all)
}

// WikiViewPageData is the template data for viewing or inline-editing an article.
type WikiViewPageData struct {
	BasePageData
	Article        *Article
	EditingContent bool
	IsPartial      bool
	Flash          string
	Error          string
}

// WikiEditPageData is the template data for the new/edit article form.
type WikiEditPageData struct {
	BasePageData
	Article       *Article
	ArticleImages []ArticleAttachment
	TagsCSV       string // pre-filled tags input value (comma-separated)
}

// APIArticleItem is the summary response used in list endpoints.
type APIArticleItem struct {
	ID             int       `json:"id"`
	Title          string    `json:"title"`
	CreatedAt      time.Time `json:"created_at"`
	LastModifiedAt time.Time `json:"last_modified_at"`
}

// APIArticleAttachment is the attachment sub-object in API responses.
type APIArticleAttachment struct {
	ID           int       `json:"id"`
	OriginalName string    `json:"original_name"`
	URL          string    `json:"url"`
	IsImage      bool      `json:"is_image"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// APIArticleDetail is the full detail response for a single article.
type APIArticleDetail struct {
	ID             int                    `json:"id"`
	Title          string                 `json:"title"`
	Content        string                 `json:"content"`
	CreatedAt      time.Time              `json:"created_at"`
	LastModifiedAt time.Time              `json:"last_modified_at"`
	Attachments    []APIArticleAttachment `json:"attachments"`
}

// ArticleRequest is the JSON body for create/update API calls.
type ArticleRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}
