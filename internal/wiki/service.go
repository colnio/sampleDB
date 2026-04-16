package wiki

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	// Article CRUD
	ListArticles(ctx context.Context) ([]Article, error)
	GetArticle(ctx context.Context, title string) (*Article, error)
	CreateArticle(ctx context.Context, title, content string, createdBy int) error
	UpdateArticleContent(ctx context.Context, title, content string, modifiedBy int) error
	DeleteArticle(ctx context.Context, title string) (filePaths []string, err error)

	// Access control
	ArticleEditAccess(ctx context.Context, title string, userID int) (createdBy int, isAdmin bool, err error)
	ArticleUploadAccess(ctx context.Context, articleID string, userID int) (createdBy int, isAdmin bool, err error)
	AttachmentDeleteAccess(ctx context.Context, attachmentID string, userID int) (createdBy int, isAdmin bool, err error)
	IsAdmin(ctx context.Context, userID int) (bool, error)

	// Attachments
	AddAttachment(ctx context.Context, articleID, filePath, originalName string, uploadedBy int) error
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

// ---------------------------------------------------------------------------
// Article CRUD
// ---------------------------------------------------------------------------

// ListArticles returns all articles in reverse-chronological order (title +
// creation time only, no content or attachments).
func (s *DBService) ListArticles(ctx context.Context) ([]Article, error) {
	rows, err := s.db.Query(ctx,
		`SELECT article_id, title, created_at FROM articles ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Title, &a.CreatedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// GetArticle loads a full article (rendered content and attachment list) by
// title.  Returns the pgx.ErrNoRows sentinel when the title is not found.
func (s *DBService) GetArticle(ctx context.Context, title string) (*Article, error) {
	var article Article
	var rawContent string
	err := s.db.QueryRow(ctx,
		`SELECT article_id, title, content, created_at, created_by,
		        COALESCE(last_modified_at, created_at),
		        COALESCE(last_modified_by, created_by)
		   FROM articles WHERE title = $1`, title).Scan(
		&article.ID, &article.Title, &rawContent, &article.CreatedAt, &article.CreatedBy,
		&article.LastModifiedAt, &article.LastModifiedBy)
	if err != nil {
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
			log.Printf("wiki: GetArticle attachment scan error: %v", scanErr)
			continue
		}
		att.IsImage = isImage(mime.TypeByExtension(filepath.Ext(att.Address)))
		article.Attachments = append(article.Attachments, att)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, rowsErr
	}

	return &article, nil
}

// CreateArticle inserts a new article.
func (s *DBService) CreateArticle(ctx context.Context, title, content string, createdBy int) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO articles (title, content, created_by, last_modified_by)
		 VALUES ($1, $2, $3, $3)`,
		title, content, createdBy)
	return err
}

// UpdateArticleContent replaces an article's Markdown content.
func (s *DBService) UpdateArticleContent(ctx context.Context, title, content string, modifiedBy int) error {
	_, err := s.db.Exec(ctx,
		`UPDATE articles SET content = $1, last_modified_at = NOW(),
		 last_modified_by = $2 WHERE title = $3`,
		content, modifiedBy, title)
	return err
}

// DeleteArticle removes an article (and, via CASCADE, its attachments from the
// DB).  It returns the file-system paths of the deleted attachments so the
// caller can remove the files.  The function does NOT touch the filesystem.
func (s *DBService) DeleteArticle(ctx context.Context, title string) ([]string, error) {
	attRows, err := s.db.Query(ctx,
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
		if scanErr := attRows.Scan(&fp); scanErr == nil {
			filePaths = append(filePaths, fp)
		}
	}
	attRows.Close()

	_, err = s.db.Exec(ctx, "DELETE FROM articles WHERE title = $1", title)
	return filePaths, err
}

// ---------------------------------------------------------------------------
// Access control
// ---------------------------------------------------------------------------

// ArticleEditAccess returns the creator's user ID and whether the given user
// is an admin, for an article identified by title.  Returns pgx.ErrNoRows if
// the article does not exist.
func (s *DBService) ArticleEditAccess(ctx context.Context, title string, userID int) (int, bool, error) {
	var createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM articles a
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE a.title = $2`, userID, title).Scan(&createdBy, &isAdmin)
	return createdBy, isAdmin, err
}

// ArticleUploadAccess returns the creator's user ID and admin flag for an
// article identified by its numeric ID (as a string).
func (s *DBService) ArticleUploadAccess(ctx context.Context, articleID string, userID int) (int, bool, error) {
	var createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM articles a
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE a.article_id = $2`, userID, articleID).Scan(&createdBy, &isAdmin)
	return createdBy, isAdmin, err
}

// AttachmentDeleteAccess returns the article creator's user ID and admin flag
// for permission checks before deleting an attachment.
func (s *DBService) AttachmentDeleteAccess(ctx context.Context, attachmentID string, userID int) (int, bool, error) {
	var createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM article_attachments aa
		   JOIN articles a ON aa.article_id = a.article_id
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE aa.attachment_id = $2`, userID, attachmentID).Scan(&createdBy, &isAdmin)
	return createdBy, isAdmin, err
}

// IsAdmin returns true if userID has admin privileges and has not been deleted.
func (s *DBService) IsAdmin(ctx context.Context, userID int) (bool, error) {
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		"SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
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

// AddAttachment records an uploaded file in the database.
func (s *DBService) AddAttachment(ctx context.Context, articleID, filePath, originalName string, uploadedBy int) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO article_attachments
		   (article_id, attachment_address, original_name, uploaded_by)
		 VALUES ($1, $2, $3, $4)`,
		articleID, filePath, originalName, uploadedBy)
	return err
}

// GetAttachmentInfo returns the storage path and original filename for a given
// attachment ID.  Returns pgx.ErrNoRows if the attachment does not exist.
func (s *DBService) GetAttachmentInfo(ctx context.Context, attachmentID string) (string, string, error) {
	var filePath, originalName string
	err := s.db.QueryRow(ctx,
		"SELECT attachment_address, original_name FROM article_attachments WHERE attachment_id = $1",
		attachmentID).Scan(&filePath, &originalName)
	return filePath, originalName, err
}

// DeleteAttachment removes the DB row and returns (articleTitle, filePath) so
// the caller can delete the file from disk.
func (s *DBService) DeleteAttachment(ctx context.Context, attachmentID string) (string, string, error) {
	var filePath, articleTitle string
	err := s.db.QueryRow(ctx,
		`SELECT aa.attachment_address, a.title
		   FROM article_attachments aa
		   JOIN articles a ON aa.article_id = a.article_id
		  WHERE aa.attachment_id = $1`, attachmentID).Scan(&filePath, &articleTitle)
	if err != nil {
		return "", "", err
	}

	_, err = s.db.Exec(ctx,
		"DELETE FROM article_attachments WHERE attachment_id = $1", attachmentID)
	return articleTitle, filePath, err
}

// GetArticleTitle returns the title of an article by its numeric ID.
func (s *DBService) GetArticleTitle(ctx context.Context, articleID string) (string, error) {
	var title string
	err := s.db.QueryRow(ctx,
		"SELECT title FROM articles WHERE article_id = $1", articleID).Scan(&title)
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
func (s *DBService) APIGetArticle(ctx context.Context, id int) (APIArticleDetail, error) {
	var detail APIArticleDetail
	err := s.db.QueryRow(ctx, `
		SELECT article_id, title, content, created_at,
		       COALESCE(last_modified_at, created_at)
		  FROM articles WHERE article_id = $1`, id).Scan(
		&detail.ID, &detail.Title, &detail.Content,
		&detail.CreatedAt, &detail.LastModifiedAt)
	if err != nil {
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
// article identified by its numeric ID.
func (s *DBService) APIArticleEditAccess(ctx context.Context, id int, userID int) (int, bool, error) {
	var createdBy int
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT a.created_by, COALESCE(u.admin, false)
		   FROM articles a
		   LEFT JOIN users u ON u.user_id = $1
		  WHERE a.article_id = $2`, userID, id).Scan(&createdBy, &isAdmin)
	return createdBy, isAdmin, err
}
