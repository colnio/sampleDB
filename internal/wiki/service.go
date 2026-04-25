package wiki

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"sampleDB/internal/dbiface"
	"sampleDB/internal/web"
)

// Service is the domain interface for wiki operations.
// All database access goes through this interface, making handlers testable.
type Service interface {
	// Article CRUD. ListArticles filters by tag when tag != "".
	ListArticles(ctx context.Context, tag string) ([]Article, error)
	GetArticle(ctx context.Context, title string) (*Article, error)
	CreateArticle(ctx context.Context, title, content string, tags []string, createdBy int) (int, error)
	UpdateArticleContent(ctx context.Context, title, content string, tags []string, modifiedBy int) error
	DeleteArticle(ctx context.Context, title string) (filePaths []string, err error)

	// Access control
	ArticleEditAccess(ctx context.Context, title string, userID int) (articleID int, createdBy int, isAdmin bool, err error)
	ArticleUploadAccess(ctx context.Context, articleID string, userID int) (createdBy int, isAdmin bool, err error)
	AttachmentDeleteAccess(ctx context.Context, attachmentID string, userID int) (createdBy int, isAdmin bool, err error)
	IsAdmin(ctx context.Context, userID int) (bool, error)

	// Attachments
	AddAttachment(ctx context.Context, articleID, filePath, originalName string, uploadedBy int) (int, error)
	GetAttachmentInfo(ctx context.Context, attachmentID string) (filePath, originalName string, err error)
	// DeleteAttachment removes the DB row and returns (articleTitle, filePath) for
	// caller-side cleanup.  It does NOT touch the filesystem.
	DeleteAttachment(ctx context.Context, attachmentID string) (articleTitle, filePath string, err error)
	GetArticleTitle(ctx context.Context, articleID string) (string, error)

	// API methods
	APIListArticles(ctx context.Context, limit, offset int) (items []APIArticleItem, total int, err error)
	APIGetArticle(ctx context.Context, id int) (APIArticleDetail, error)
	APICreateArticle(ctx context.Context, title, content string, createdBy int) (APIArticleDetail, error)
	// APIUpdateArticle updates title+content and returns the number of affected rows.
	APIUpdateArticle(ctx context.Context, id int, title, content string, modifiedBy int) (rowsAffected int64, err error)
	APIArticleEditAccess(ctx context.Context, id int, userID int) (createdBy int, isAdmin bool, err error)
}

// DBService is the concrete Service implementation backed by PostgreSQL.
type DBService struct {
	db dbiface.Pool
}

// NewService constructs a DBService backed by pool.
func NewService(db dbiface.Pool) *DBService {
	return &DBService{db: db}
}

// isImage returns true if the MIME type represents an image.
func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

// KnownTags is the stable filter-chip vocabulary used on the wiki list page.
// Authors may still set free-form tags on articles; those simply don't get a
// dedicated chip but remain searchable via ?tag=.
var KnownTags = []string{"Safety", "Equipment", "Protocols", "Admin"}

// buildExcerpt strips common Markdown noise and returns a short, single-line
// teaser (max ~160 chars) for use on article cards.
func buildExcerpt(raw string) string {
	if raw == "" {
		return ""
	}
	s := raw
	// Drop leading heading markers on the first meaningful line.
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Strip heading markers and common markdown symbols for a plain teaser.
		line = strings.TrimLeft(line, "#> -*_`")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if runes := []rune(line); len(runes) > 160 {
			line = string(runes[:157]) + "…"
		}
		return line
	}
	return ""
}

// NormalizeTagsCSV parses a comma-separated tag input from a form field into a
// de-duplicated slice, preserving the original ordering and stripping empty
// entries. Returns an empty slice (never nil) so callers can pass it straight
// to the service layer.
func NormalizeTagsCSV(csv string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, part := range strings.Split(csv, ",") {
		t := strings.TrimSpace(part)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	return out
}

// ---------------------------------------------------------------------------
// Article CRUD
// ---------------------------------------------------------------------------

// ListArticles returns articles (reverse-chronological) with tags, creator
// username, and a short content excerpt suitable for card rendering. When
// tag != "" only articles whose tags[] contains that tag are returned.
func (s *DBService) ListArticles(ctx context.Context, tag string) ([]Article, error) {
	baseQuery := `
		SELECT a.article_id, a.title, a.created_at, a.tags,
		       COALESCE(u.username, ''),
		       COALESCE(substring(a.content from 1 for 240), '')
		  FROM articles a
		  LEFT JOIN users u ON u.user_id = a.created_by`
	args := []any{}
	if tag != "" {
		// Case-insensitive match so the hard-coded filter chips (Title-Case) also
		// surface articles saved with arbitrary casing.
		baseQuery += ` WHERE EXISTS (SELECT 1 FROM unnest(a.tags) AS t WHERE lower(t) = lower($1))`
		args = append(args, tag)
	}
	baseQuery += ` ORDER BY a.created_at DESC`

	rows, err := s.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var (
			a       Article
			rawBody string
		)
		if err := rows.Scan(&a.ID, &a.Title, &a.CreatedAt, &a.Tags, &a.CreatorName, &rawBody); err != nil {
			return nil, err
		}
		a.Excerpt = buildExcerpt(rawBody)
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// GetArticle loads a full article (rendered content and attachment list) by
// title.  Returns web.ErrNotFound when the title is not found.
func (s *DBService) GetArticle(ctx context.Context, title string) (*Article, error) {
	var article Article
	var rawContent string
	err := s.db.QueryRow(ctx,
		`SELECT article_id, title, content, tags, created_at, created_by,
		        COALESCE(last_modified_at, created_at),
		        COALESCE(last_modified_by, created_by)
		   FROM articles WHERE title = $1`, title).Scan(
		&article.ID, &article.Title, &rawContent, &article.Tags, &article.CreatedAt, &article.CreatedBy,
		&article.LastModifiedAt, &article.LastModifiedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, web.ErrNotFound
		}
		return nil, err
	}

	article.Content = ArticleContent{
		Raw:  rawContent,
		HTML: web.RenderMarkdown(rawContent),
	}

	rows, err := s.db.Query(ctx,
		`SELECT attachment_id, original_name, attachment_address, uploaded_at
		   FROM article_attachments WHERE article_id = $1`, article.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var att ArticleAttachment
		if scanErr := rows.Scan(&att.ID, &att.OriginalName, &att.Address, &att.UploadedAt); scanErr != nil {
			return nil, fmt.Errorf("wiki: GetArticle attachment scan: %w", scanErr)
		}
		att.IsImage = isImage(mime.TypeByExtension(filepath.Ext(att.Address)))
		article.Attachments = append(article.Attachments, att)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	return &article, nil
}

// CreateArticle inserts a new article. tags may be nil or empty. Returns the new article_id.
func (s *DBService) CreateArticle(ctx context.Context, title, content string, tags []string, createdBy int) (int, error) {
	if tags == nil {
		tags = []string{}
	}
	var articleID int
	err := s.db.QueryRow(ctx,
		`INSERT INTO articles (title, content, tags, created_by, last_modified_by)
		 VALUES ($1, $2, $3, $4, $4) RETURNING article_id`,
		title, content, tags, createdBy).Scan(&articleID)
	return articleID, err
}

// UpdateArticleContent replaces an article's Markdown content and tags.
func (s *DBService) UpdateArticleContent(ctx context.Context, title, content string, tags []string, modifiedBy int) error {
	if tags == nil {
		tags = []string{}
	}
	tag, err := s.db.Exec(ctx,
		`UPDATE articles SET content = $1, tags = $2, last_modified_at = NOW(),
		 last_modified_by = $3 WHERE title = $4`,
		content, tags, modifiedBy, title)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// DeleteArticle removes an article (and, via CASCADE, its attachments from the
// DB).  It returns the file-system paths of the deleted attachments so the
// caller can remove the files.  The function does NOT touch the filesystem.
func (s *DBService) DeleteArticle(ctx context.Context, title string) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wiki: DeleteArticle: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	attRows, err := tx.Query(ctx,
		`SELECT aa.attachment_address
		   FROM article_attachments aa
		   JOIN articles a ON aa.article_id = a.article_id
		  WHERE a.title = $1`, title)
	if err != nil {
		return nil, fmt.Errorf("wiki: DeleteArticle: could not list attachments for %q: %w", title, err)
	}
	var filePaths []string
	for attRows.Next() {
		var fp string
		if scanErr := attRows.Scan(&fp); scanErr != nil {
			attRows.Close()
			return nil, fmt.Errorf("wiki: DeleteArticle: attachment scan for %q: %w", title, scanErr)
		}
		filePaths = append(filePaths, fp)
	}
	if rowsErr := attRows.Err(); rowsErr != nil {
		attRows.Close()
		return nil, fmt.Errorf("wiki: DeleteArticle: error reading attachments for %q: %w", title, rowsErr)
	}
	attRows.Close()

	cmdTag, err := tx.Exec(ctx, "DELETE FROM articles WHERE title = $1", title)
	if err != nil {
		return nil, fmt.Errorf("wiki: DeleteArticle: delete %q: %w", title, err)
	}
	if cmdTag.RowsAffected() == 0 {
		return nil, web.ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wiki: DeleteArticle: commit: %w", err)
	}
	return filePaths, nil
}

// ---------------------------------------------------------------------------
// Access control
// ---------------------------------------------------------------------------

// ArticleEditAccess returns the creator's user ID and whether the given user
// is an admin, for an article identified by title.  Returns web.ErrNotFound if
// the article does not exist.
func (s *DBService) ArticleEditAccess(ctx context.Context, title string, userID int) (int, int, bool, error) {
	var articleID, createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.article_id, a.created_by, COALESCE(u.admin, false)
		   FROM articles a
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE a.title = $2`, userID, title).Scan(&articleID, &createdBy, &isAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, web.ErrNotFound
	}
	return articleID, createdBy, isAdmin, err
}

// ArticleUploadAccess returns the creator's user ID and admin flag for an
// article identified by its numeric ID (as a string).  Returns web.ErrNotFound
// if the article does not exist.
func (s *DBService) ArticleUploadAccess(ctx context.Context, articleID string, userID int) (int, bool, error) {
	var createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM articles a
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE a.article_id = $2`, userID, articleID).Scan(&createdBy, &isAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, web.ErrNotFound
	}
	return createdBy, isAdmin, err
}

// AttachmentDeleteAccess returns the article creator's user ID and admin flag
// for permission checks before deleting an attachment.  Returns web.ErrNotFound
// if the attachment does not exist.
func (s *DBService) AttachmentDeleteAccess(ctx context.Context, attachmentID string, userID int) (int, bool, error) {
	var createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM article_attachments aa
		   JOIN articles a ON aa.article_id = a.article_id
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE aa.attachment_id = $2`, userID, attachmentID).Scan(&createdBy, &isAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, web.ErrNotFound
	}
	return createdBy, isAdmin, err
}

// IsAdmin returns true if userID has admin privileges and has not been deleted.
func (s *DBService) IsAdmin(ctx context.Context, userID int) (bool, error) {
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		"SELECT COALESCE(admin, false) FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
		userID).Scan(&isAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return isAdmin, nil
}

// ---------------------------------------------------------------------------
// Attachments
// ---------------------------------------------------------------------------

// AddAttachment records an uploaded file in the database and returns the new attachment ID.
func (s *DBService) AddAttachment(ctx context.Context, articleID, filePath, originalName string, uploadedBy int) (int, error) {
	var id int
	err := s.db.QueryRow(ctx,
		`INSERT INTO article_attachments
		   (article_id, attachment_address, original_name, uploaded_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING attachment_id`,
		articleID, filePath, originalName, uploadedBy).Scan(&id)
	return id, err
}

// GetAttachmentInfo returns the storage path and original filename for a given
// attachment ID.  Returns web.ErrNotFound if the attachment does not exist.
func (s *DBService) GetAttachmentInfo(ctx context.Context, attachmentID string) (string, string, error) {
	var filePath, originalName string
	err := s.db.QueryRow(ctx,
		"SELECT attachment_address, original_name FROM article_attachments WHERE attachment_id = $1",
		attachmentID).Scan(&filePath, &originalName)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", web.ErrNotFound
	}
	return filePath, originalName, err
}

// DeleteAttachment removes the DB row and returns (articleTitle, filePath) so
// the caller can delete the file from disk.  Returns web.ErrNotFound if the
// attachment does not exist.  The SELECT and DELETE are performed atomically
// via a CTE so concurrent deletes do not produce stale data.
func (s *DBService) DeleteAttachment(ctx context.Context, attachmentID string) (string, string, error) {
	var filePath, articleTitle string
	err := s.db.QueryRow(ctx,
		`WITH deleted AS (
		     DELETE FROM article_attachments
		     WHERE attachment_id = $1
		     RETURNING attachment_address, article_id
		 )
		 SELECT d.attachment_address, a.title
		 FROM deleted d
		 JOIN articles a ON a.article_id = d.article_id`,
		attachmentID).Scan(&filePath, &articleTitle)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", web.ErrNotFound
		}
		return "", "", err
	}
	return articleTitle, filePath, nil
}

// GetArticleTitle returns the title of an article by its numeric ID.
func (s *DBService) GetArticleTitle(ctx context.Context, articleID string) (string, error) {
	var title string
	err := s.db.QueryRow(ctx,
		"SELECT title FROM articles WHERE article_id = $1", articleID).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", web.ErrNotFound
	}
	return title, err
}

// ---------------------------------------------------------------------------
// API methods
// ---------------------------------------------------------------------------

// APIListArticles returns paginated items and the total count.
func (s *DBService) APIListArticles(ctx context.Context, limit, offset int) ([]APIArticleItem, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM articles").Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT article_id, title, created_at,
		       COALESCE(last_modified_at, created_at)
		  FROM articles
		 ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]APIArticleItem, 0)
	for rows.Next() {
		var item APIArticleItem
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.LastModifiedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// APIGetArticle loads a full article for API consumption (attachments included).
// Returns web.ErrNotFound if the article does not exist.
func (s *DBService) APIGetArticle(ctx context.Context, id int) (APIArticleDetail, error) {
	var detail APIArticleDetail
	err := s.db.QueryRow(ctx, `
		SELECT article_id, title, content, created_at,
		       COALESCE(last_modified_at, created_at)
		  FROM articles WHERE article_id = $1`, id).Scan(
		&detail.ID, &detail.Title, &detail.Content,
		&detail.CreatedAt, &detail.LastModifiedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return APIArticleDetail{}, web.ErrNotFound
		}
		return APIArticleDetail{}, err
	}

	detail.Attachments = make([]APIArticleAttachment, 0)
	attRows, attErr := s.db.Query(ctx, `
		SELECT attachment_id, original_name, attachment_address, uploaded_at
		  FROM article_attachments WHERE article_id = $1`, detail.ID)
	if attErr != nil {
		return APIArticleDetail{}, attErr
	}
	defer attRows.Close()

	for attRows.Next() {
		var att APIArticleAttachment
		var addr string
		if scanErr := attRows.Scan(&att.ID, &att.OriginalName, &addr, &att.UploadedAt); scanErr != nil {
			return APIArticleDetail{}, scanErr
		}
		att.URL = fmt.Sprintf("/wiki/attachment/%d", att.ID)
		att.IsImage = isImage(mime.TypeByExtension(filepath.Ext(addr)))
		detail.Attachments = append(detail.Attachments, att)
	}
	if err := attRows.Err(); err != nil {
		return APIArticleDetail{}, err
	}

	return detail, nil
}

// APICreateArticle inserts a new article and returns the full detail.
func (s *DBService) APICreateArticle(ctx context.Context, title, content string, createdBy int) (APIArticleDetail, error) {
	var id int
	var createdAt time.Time
	err := s.db.QueryRow(ctx, `
		INSERT INTO articles (title, content, created_by, last_modified_by)
		VALUES ($1, $2, $3, $3)
		RETURNING article_id, created_at`,
		title, content, createdBy).Scan(&id, &createdAt)
	if err != nil {
		return APIArticleDetail{}, err
	}

	return APIArticleDetail{
		ID:             id,
		Title:          title,
		Content:        content,
		CreatedAt:      createdAt,
		LastModifiedAt: createdAt,
		Attachments:    []APIArticleAttachment{},
	}, nil
}

// APIUpdateArticle updates an article's title and content, returning the
// number of rows affected.
func (s *DBService) APIUpdateArticle(ctx context.Context, id int, title, content string, modifiedBy int) (int64, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE articles
		   SET title            = $1,
		       content          = $2,
		       last_modified_at = NOW(),
		       last_modified_by = $3
		 WHERE article_id = $4`,
		title, content, modifiedBy, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// APIArticleEditAccess returns the creator's user ID and admin flag for an
// article identified by its numeric ID.  Returns web.ErrNotFound if the article
// does not exist.
func (s *DBService) APIArticleEditAccess(ctx context.Context, id int, userID int) (int, bool, error) {
	var createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM articles a
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE a.article_id = $2`, userID, id).Scan(&createdBy, &isAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, web.ErrNotFound
	}
	return createdBy, isAdmin, err
}
