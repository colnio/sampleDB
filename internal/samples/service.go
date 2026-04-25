package samples

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"sampleDB/internal/dbiface"
	"sampleDB/internal/web"
)

// Service is the domain interface for sample operations.
// All DB-dependent operations go through this interface, making it easy to
// substitute a mock in tests.
type Service interface {
	// Sample list / search
	ListAll(ctx context.Context, f ListFilters) ([]Sample, error)
	Search(ctx context.Context, query string, f ListFilters) ([]Sample, error)

	// Single sample operations
	GetByID(ctx context.Context, id int) (Sample, error)
	Create(ctx context.Context, p CreateParams) (int, error)
	Update(ctx context.Context, id int, p UpdateParams) error
	Delete(ctx context.Context, id int) error

	// Attachments
	GetAttachments(ctx context.Context, sampleID int) ([]Attachment, error)
	AddAttachment(ctx context.Context, sampleID int, path string) (int, error)
	// DeleteAttachment removes the DB row and returns the file path for cleanup.
	DeleteAttachment(ctx context.Context, attachmentID int) (filePath string, err error)
	// AttachmentInfo returns (sampleID, filePath) for a given attachment ID.
	AttachmentInfo(ctx context.Context, attachmentID int) (sampleID int, filePath string, err error)

	// Authorization
	// CheckOwner returns nil if userID owns sampleID (or is admin).
	// Returns web.ErrNotFound if the sample doesn't exist,
	// web.ErrForbidden if the user is not the owner and not admin.
	CheckOwner(ctx context.Context, sampleID, userID int) error

	// Sharing
	GenerateShareToken(ctx context.Context, sampleID int, visibility string) (string, error)
	GetByShareToken(ctx context.Context, token string) (Sample, error)
	RevokeShareToken(ctx context.Context, sampleID int) error
	// SharedAttachmentInfo returns (sampleID, visibility, filePath) for serving
	// attachments via the public /s/{token}/attachment/{id} route.
	SharedAttachmentInfo(ctx context.Context, token, attachmentID string) (sampleID int, visibility, filePath string, err error)

	// Fabrication steps
	GetSteps(ctx context.Context, sampleID int) ([]FabricationStep, error)
	GetStepByID(ctx context.Context, stepID int) (FabricationStep, error)
	AddStep(ctx context.Context, sampleID int, step FabricationStep) (FabricationStep, error)
	UpdateStep(ctx context.Context, stepID int, step FabricationStep) error
	DeleteStep(ctx context.Context, stepID int) error
	ReorderSteps(ctx context.Context, sampleID int, orderedIDs []int) error

	// Supporting data for forms
	GetApprovedUsers(ctx context.Context) ([]User, error)
	GetAllEquipment(ctx context.Context) ([]Equipment, error)
	IsAdmin(ctx context.Context, userID int) (bool, error)
	// GetUsernameByID returns the username for userID, or "" if not found.
	GetUsernameByID(ctx context.Context, userID int) (string, error)
	// GetUsernameByIDApproved returns the username for an approved userID, or error if not found.
	GetUsernameByIDApproved(ctx context.Context, userID int) (string, error)
	// AttachmentFilePath returns the file path for a sample attachment by ID.
	AttachmentFilePath(ctx context.Context, attachmentID int) (string, error)

	// API-specific operations (paginated/aggregate queries for JSON endpoints)
	APIListSamples(ctx context.Context, p APIListParams) (APIListResult, error)
	APIGetSample(ctx context.Context, id int) (APISampleDetail, error)
	APICreateSample(ctx context.Context, name, description, keywords, owner, samplePrep, status string, ownerUserID *int) (APISampleItem, error)
	APIUpdateSample(ctx context.Context, id int, name, description, keywords, owner, samplePrep, status string, ownerUserID *int) error
	APIDeleteSample(ctx context.Context, id int) ([]string, error)
	SampleExistsByID(ctx context.Context, id int) (bool, error)
}

// ListFilters holds optional query filters for ListAll / Search. Zero-valued
// fields are treated as "no filter" so existing call-sites that don't need
// the new filters can pass ListFilters{}.
type ListFilters struct {
	DateFrom string
	DateTo   string
	Status   string // "", "active", "in_progress", "archived"
	Owner    string // "" (all) or a specific username
}

// CreateParams holds the fields for creating a new sample.
type CreateParams struct {
	Name        string
	Description string
	Keywords    string
	Owner       string
	OwnerUserID *int
	Status      string // "" defers to DB default ('active')
}

// UpdateParams holds the fields for updating an existing sample.
type UpdateParams struct {
	Name        string
	Description string
	Keywords    string
	Owner       string
	OwnerUserID *int
	Status      string // "" leaves status unchanged
}

// DBService is the concrete Service implementation backed by a PostgreSQL pool.
type DBService struct {
	db dbiface.Pool
}

// NewService constructs a DBService using the provided pool.
func NewService(db dbiface.Pool) *DBService {
	return &DBService{db: db}
}

// originalFilenameFromPath extracts the original filename from a stored path
// that was generated by generateUniqueFilename (format: "{hex}_{original}").
func originalFilenameFromPath(path string) string {
	base := filepath.Base(path)
	if idx := strings.Index(base, "_"); idx >= 0 && idx+1 < len(base) {
		return base[idx+1:]
	}
	return base
}

// isImage reports whether the given MIME content-type is an image.
func isImage(contentType string) bool {
	return strings.HasPrefix(contentType, "image/")
}

// hydrationAttachments populates computed fields (OriginalName, ContentType, IsImage)
// for all attachments in the slice.
func hydrateAttachments(atts []Attachment) {
	for i := range atts {
		atts[i].OriginalName = originalFilenameFromPath(atts[i].Address)
		atts[i].ContentType = mime.TypeByExtension(filepath.Ext(atts[i].OriginalName))
		atts[i].IsImage = isImage(atts[i].ContentType)
	}
}

// ------------------------------------------------------------------ ListAll --

func (s *DBService) ListAll(ctx context.Context, f ListFilters) ([]Sample, error) {
	q := `SELECT s.sample_id, s.sample_name, COALESCE(s.sample_description, ''), COALESCE(s.sample_keywords, ''),
		       COALESCE(s.sample_owner, ''), s.owner_user_id, u.username, COALESCE(s.sample_prep, ''), s.created_at,
		       COALESCE(s.status, 'active'),
		       first_img.attachment_id,
		       (SELECT COUNT(*) FROM fabrication_steps WHERE sample_id = s.sample_id) AS step_count
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		  LEFT JOIN LATERAL (
		      SELECT attachment_id FROM attachments
		       WHERE source_type = 'sample' AND source_id = s.sample_id
		         AND attachment_address ~* '\.(jpg|jpeg|png|gif|webp|svg)$'
		       ORDER BY attachment_id ASC LIMIT 1
		  ) first_img ON true`
	var args []interface{}
	var whereClauses []string

	if f.DateFrom != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.created_at >= $%d", len(args)+1))
		args = append(args, f.DateFrom)
	}
	if f.DateTo != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.created_at < $%d::date + interval '1 day'", len(args)+1))
		args = append(args, f.DateTo)
	}
	if f.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("s.status = $%d", len(args)+1))
		args = append(args, f.Status)
	}
	if f.Owner != "" {
		whereClauses = append(whereClauses,
			fmt.Sprintf("(u.username = $%[1]d OR s.sample_owner = $%[1]d)", len(args)+1))
		args = append(args, f.Owner)
	}
	if len(whereClauses) > 0 {
		q += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	q += " ORDER BY s.sample_name ASC"

	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Sample
	for rows.Next() {
		var smpl Sample
		if err := rows.Scan(&smpl.ID, &smpl.Name, &smpl.Description, &smpl.Keywords,
			&smpl.Owner, &smpl.OwnerUserID, &smpl.OwnerUsername, &smpl.SamplePrep,
			&smpl.CreatedAt, &smpl.Status, &smpl.FirstImageID, &smpl.StepCount); err != nil {
			return nil, err
		}
		out = append(out, smpl)
	}
	return out, rows.Err()
}

// escapeLike escapes ILIKE metacharacters so that literal %, _, and \ are not
// treated as wildcards. Use with ESCAPE '\' in the query string.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// ------------------------------------------------------------------ Search --

func (s *DBService) Search(ctx context.Context, query string, f ListFilters) ([]Sample, error) {
	trimmed := strings.TrimSpace(query)
	rawKeywords := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ' ' || r == ',' || r == ';'
	})

	var keywords []string
	for _, kw := range rawKeywords {
		if c := strings.TrimSpace(kw); c != "" {
			keywords = append(keywords, strings.ToLower(c))
		}
	}

	var whereClauses []string
	var args []interface{}
	for i, kw := range keywords {
		whereClauses = append(whereClauses,
			fmt.Sprintf("$%d = ANY(string_to_array(lower(s.sample_keywords), ','))", i+1))
		args = append(args, kw)
	}

	escapedTrimmed := escapeLike(trimmed)

	nameIdx := len(args) + 1
	args = append(args, "%"+escapedTrimmed+"%")
	ownerIdx := len(args) + 1
	args = append(args, "%"+escapedTrimmed+"%")

	searchConds := []string{
		fmt.Sprintf("s.sample_name ILIKE $%d ESCAPE '\\'", nameIdx),
		fmt.Sprintf("s.sample_owner ILIKE $%d ESCAPE '\\'", ownerIdx),
	}
	searchConds = append(searchConds, whereClauses...)

	q := fmt.Sprintf(`SELECT s.sample_id, s.sample_name, COALESCE(s.sample_description, ''), COALESCE(s.sample_keywords, ''),
		       COALESCE(s.sample_owner, ''), s.owner_user_id, u.username, COALESCE(s.sample_prep, ''), s.created_at,
		       COALESCE(s.status, 'active'),
		       first_img.attachment_id,
		       (SELECT COUNT(*) FROM fabrication_steps WHERE sample_id = s.sample_id) AS step_count
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		  LEFT JOIN LATERAL (
		      SELECT attachment_id FROM attachments
		       WHERE source_type = 'sample' AND source_id = s.sample_id
		         AND attachment_address ~* '\.(jpg|jpeg|png|gif|webp|svg)$'
		       ORDER BY attachment_id ASC LIMIT 1
		  ) first_img ON true
		 WHERE (%s)`, strings.Join(searchConds, " OR "))

	if f.DateFrom != "" {
		args = append(args, f.DateFrom)
		q += fmt.Sprintf(" AND s.created_at >= $%d", len(args))
	}
	if f.DateTo != "" {
		args = append(args, f.DateTo)
		q += fmt.Sprintf(" AND s.created_at < $%d::date + interval '1 day'", len(args))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		q += fmt.Sprintf(" AND s.status = $%d", len(args))
	}
	if f.Owner != "" {
		args = append(args, f.Owner)
		q += fmt.Sprintf(" AND (u.username = $%[1]d OR s.sample_owner = $%[1]d)", len(args))
	}
	q += " ORDER BY s.sample_name ASC"

	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Sample
	for rows.Next() {
		var smpl Sample
		if err := rows.Scan(&smpl.ID, &smpl.Name, &smpl.Description, &smpl.Keywords,
			&smpl.Owner, &smpl.OwnerUserID, &smpl.OwnerUsername, &smpl.SamplePrep,
			&smpl.CreatedAt, &smpl.Status, &smpl.FirstImageID, &smpl.StepCount); err != nil {
			return nil, err
		}
		out = append(out, smpl)
	}
	return out, rows.Err()
}

// ----------------------------------------------------------------- GetByID --

func (s *DBService) GetByID(ctx context.Context, id int) (Sample, error) {
	var smpl Sample
	err := s.db.QueryRow(ctx,
		`SELECT s.sample_id, s.sample_name, COALESCE(s.sample_description, ''), COALESCE(s.sample_keywords, ''),
		        COALESCE(s.sample_owner, ''), s.owner_user_id, u.username,
		        COALESCE(s.sample_prep, ''), s.created_at,
		        s.share_token, COALESCE(s.share_visibility, 'private'),
		        COALESCE(s.status, 'active')
		   FROM samples s
		   LEFT JOIN users u ON s.owner_user_id = u.user_id
		  WHERE s.sample_id=$1`, id).Scan(
		&smpl.ID, &smpl.Name, &smpl.Description, &smpl.Keywords, &smpl.Owner,
		&smpl.OwnerUserID, &smpl.OwnerUsername,
		&smpl.SamplePrep, &smpl.CreatedAt,
		&smpl.ShareToken, &smpl.ShareVisibility, &smpl.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return smpl, web.ErrNotFound
		}
		return smpl, err
	}

	atts, err := s.GetAttachments(ctx, id)
	if err != nil {
		return smpl, err
	}
	smpl.Attachments = atts
	return smpl, nil
}

// ------------------------------------------------------------------ Create --

func (s *DBService) Create(ctx context.Context, p CreateParams) (int, error) {
	status := p.Status
	if status == "" {
		status = "active"
	}
	var id int
	err := s.db.QueryRow(ctx,
		`INSERT INTO samples (sample_name, sample_description, sample_keywords, sample_owner, owner_user_id, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING sample_id`,
		p.Name, p.Description, p.Keywords, p.Owner, p.OwnerUserID, status).Scan(&id)
	return id, err
}

// ------------------------------------------------------------------ Update --

func (s *DBService) Update(ctx context.Context, id int, p UpdateParams) error {
	var (
		tag pgconn.CommandTag
		err error
	)
	if p.Status == "" {
		tag, err = s.db.Exec(ctx,
			`UPDATE samples SET sample_name=$1, sample_description=$2, sample_keywords=$3,
			        sample_owner=$4, owner_user_id=$5
			  WHERE sample_id=$6`,
			p.Name, p.Description, p.Keywords, p.Owner, p.OwnerUserID, id)
	} else {
		tag, err = s.db.Exec(ctx,
			`UPDATE samples SET sample_name=$1, sample_description=$2, sample_keywords=$3,
			        sample_owner=$4, owner_user_id=$5, status=$6
			  WHERE sample_id=$7`,
			p.Name, p.Description, p.Keywords, p.Owner, p.OwnerUserID, p.Status, id)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ Delete --

func (s *DBService) Delete(ctx context.Context, id int) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM samples WHERE sample_id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------- Attachments --

func (s *DBService) GetAttachments(ctx context.Context, sampleID int) ([]Attachment, error) {
	rows, err := s.db.Query(ctx,
		`SELECT attachment_id, source_id, attachment_address, uploaded_at
		   FROM attachments
		  WHERE source_type = 'sample' AND source_id = $1
		  ORDER BY uploaded_at DESC, attachment_id DESC`,
		sampleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var atts []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.SampleID, &a.Address, &a.UploadedAt); err != nil {
			return nil, err
		}
		atts = append(atts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	hydrateAttachments(atts)
	return atts, nil
}

func (s *DBService) AddAttachment(ctx context.Context, sampleID int, path string) (int, error) {
	var id int
	// The subquery guards against inserting an attachment for a sample that
	// was concurrently deleted (no FK constraint exists on this table).
	err := s.db.QueryRow(ctx,
		`INSERT INTO attachments (source_id, attachment_address, source_type)
		 SELECT $1, $2, 'sample' FROM samples WHERE sample_id = $1
		 RETURNING attachment_id`,
		sampleID, path).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, web.ErrNotFound
	}
	return id, err
}

func (s *DBService) DeleteAttachment(ctx context.Context, attachmentID int) (string, error) {
	var filePath string
	err := s.db.QueryRow(ctx,
		"DELETE FROM attachments WHERE attachment_id = $1 AND source_type = 'sample' RETURNING attachment_address",
		attachmentID).Scan(&filePath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", web.ErrNotFound
		}
		return "", err
	}
	return filePath, nil
}

func (s *DBService) AttachmentInfo(ctx context.Context, attachmentID int) (int, string, error) {
	var sampleID int
	var filePath string
	err := s.db.QueryRow(ctx,
		"SELECT source_id, attachment_address FROM attachments WHERE attachment_id = $1 AND source_type = 'sample'",
		attachmentID).Scan(&sampleID, &filePath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", web.ErrNotFound
		}
		return 0, "", err
	}
	return sampleID, filePath, nil
}

// ------------------------------------------------------------ CheckOwner ---

func (s *DBService) CheckOwner(ctx context.Context, sampleID, userID int) error {
	// Verify the sample exists; any authenticated user may edit.
	var exists bool
	err := s.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM samples WHERE sample_id = $1)", sampleID).Scan(&exists)
	if err != nil {
		log.Printf("samples.CheckOwner: db error: %v", err)
		return fmt.Errorf("database error: %w", err)
	}
	if !exists {
		return web.ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------- Sharing --

func (s *DBService) GenerateShareToken(ctx context.Context, sampleID int, visibility string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate share token: %w", err)
	}
	token := hex.EncodeToString(b)
	tag, err := s.db.Exec(ctx,
		`UPDATE samples SET share_token=$1, share_visibility=$2 WHERE sample_id=$3`,
		token, visibility, sampleID)
	if err != nil {
		return "", fmt.Errorf("save share token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", web.ErrNotFound
	}
	return token, nil
}

func (s *DBService) GetByShareToken(ctx context.Context, token string) (Sample, error) {
	var smpl Sample
	err := s.db.QueryRow(ctx,
		`SELECT s.sample_id, s.sample_name, COALESCE(s.sample_description,''), COALESCE(s.sample_keywords,''),
		        COALESCE(s.sample_owner,''), s.owner_user_id, u.username,
		        COALESCE(s.sample_prep,''), s.created_at,
		        s.share_token, COALESCE(s.share_visibility,'private'),
		        COALESCE(s.status, 'active')
		   FROM samples s
		   LEFT JOIN users u ON s.owner_user_id = u.user_id
		  WHERE s.share_token = $1`, token).Scan(
		&smpl.ID, &smpl.Name, &smpl.Description, &smpl.Keywords,
		&smpl.Owner, &smpl.OwnerUserID, &smpl.OwnerUsername,
		&smpl.SamplePrep, &smpl.CreatedAt,
		&smpl.ShareToken, &smpl.ShareVisibility, &smpl.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return smpl, web.ErrNotFound
		}
		return smpl, err
	}
	atts, err := s.GetAttachments(ctx, smpl.ID)
	if err != nil {
		return smpl, err
	}
	smpl.Attachments = atts
	return smpl, nil
}

func (s *DBService) RevokeShareToken(ctx context.Context, sampleID int) error {
	_, err := s.db.Exec(ctx,
		`UPDATE samples SET share_token=NULL, share_visibility='private' WHERE sample_id=$1`, sampleID)
	return err
}

func (s *DBService) SharedAttachmentInfo(ctx context.Context, token, attachmentID string) (int, string, string, error) {
	var sampleID int
	var visibility string
	err := s.db.QueryRow(ctx,
		`SELECT s.sample_id, COALESCE(s.share_visibility, 'private')
		   FROM samples s
		  WHERE s.share_token = $1`, token).Scan(&sampleID, &visibility)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", web.ErrNotFound
		}
		return 0, "", "", err
	}

	var filePath string
	err = s.db.QueryRow(ctx,
		`SELECT attachment_address FROM attachments
		  WHERE attachment_id = $1 AND source_id = $2 AND source_type = 'sample'`,
		attachmentID, sampleID).Scan(&filePath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", "", web.ErrNotFound
		}
		return 0, "", "", err
	}
	return sampleID, visibility, filePath, nil
}

// ------------------------------------------------------- Fabrication Steps --

func (s *DBService) GetSteps(ctx context.Context, sampleID int) ([]FabricationStep, error) {
	rows, err := s.db.Query(ctx, `
		SELECT fs.step_id, fs.sample_id, fs.step_order, fs.step_date,
		       fs.person_user_id, u.username,
		       COALESCE(fs.description, ''),
		       fs.equipment_id, e.name,
		       fs.equipment_text,
		       fs.created_at
		  FROM fabrication_steps fs
		  LEFT JOIN users u ON fs.person_user_id = u.user_id
		  LEFT JOIN equipment e ON fs.equipment_id = e.equipment_id
		 WHERE fs.sample_id = $1
		 ORDER BY fs.step_order, fs.step_id`, sampleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []FabricationStep
	for rows.Next() {
		var st FabricationStep
		if err := rows.Scan(
			&st.ID, &st.SampleID, &st.StepOrder, &st.StepDate,
			&st.PersonUserID, &st.PersonUsername,
			&st.Description,
			&st.EquipmentID, &st.EquipmentName,
			&st.EquipmentText,
			&st.CreatedAt,
		); err != nil {
			return nil, err
		}
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (s *DBService) GetStepByID(ctx context.Context, stepID int) (FabricationStep, error) {
	var st FabricationStep
	err := s.db.QueryRow(ctx, `
		SELECT fs.step_id, fs.sample_id, fs.step_order, fs.step_date,
		       fs.person_user_id, u.username,
		       COALESCE(fs.description, ''),
		       fs.equipment_id, e.name,
		       fs.equipment_text,
		       fs.created_at
		  FROM fabrication_steps fs
		  LEFT JOIN users u ON fs.person_user_id = u.user_id
		  LEFT JOIN equipment e ON fs.equipment_id = e.equipment_id
		 WHERE fs.step_id = $1`, stepID).Scan(
		&st.ID, &st.SampleID, &st.StepOrder, &st.StepDate,
		&st.PersonUserID, &st.PersonUsername,
		&st.Description,
		&st.EquipmentID, &st.EquipmentName,
		&st.EquipmentText,
		&st.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return st, web.ErrNotFound
		}
		return st, err
	}
	return st, nil
}

func (s *DBService) AddStep(ctx context.Context, sampleID int, step FabricationStep) (FabricationStep, error) {
	var id int
	err := s.db.QueryRow(ctx, `
		INSERT INTO fabrication_steps
		            (sample_id, step_order, step_date, person_user_id, description, equipment_id, equipment_text)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING step_id`,
		sampleID, step.StepOrder, step.StepDate, step.PersonUserID,
		step.Description, step.EquipmentID, step.EquipmentText,
	).Scan(&id)
	if err != nil {
		return FabricationStep{}, err
	}
	return s.GetStepByID(ctx, id)
}

func (s *DBService) UpdateStep(ctx context.Context, stepID int, step FabricationStep) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE fabrication_steps
		   SET step_order     = $1,
		       step_date      = $2,
		       person_user_id = $3,
		       description    = $4,
		       equipment_id   = $5,
		       equipment_text = $6
		 WHERE step_id = $7`,
		step.StepOrder, step.StepDate, step.PersonUserID,
		step.Description, step.EquipmentID, step.EquipmentText, stepID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

func (s *DBService) DeleteStep(ctx context.Context, stepID int) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM fabrication_steps WHERE step_id = $1`, stepID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

func (s *DBService) ReorderSteps(ctx context.Context, sampleID int, orderedIDs []int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var count int
	if err := tx.QueryRow(ctx,
		"SELECT COUNT(*) FROM fabrication_steps WHERE sample_id = $1",
		sampleID).Scan(&count); err != nil {
		return err
	}
	if count != len(orderedIDs) {
		return fmt.Errorf("reorder: got %d step IDs but sample has %d steps", len(orderedIDs), count)
	}
	var matchCount int
	if err := tx.QueryRow(ctx,
		"SELECT COUNT(*) FROM fabrication_steps WHERE step_id = ANY($1) AND sample_id = $2",
		orderedIDs, sampleID).Scan(&matchCount); err != nil {
		return err
	}
	if matchCount != len(orderedIDs) {
		return fmt.Errorf("reorder: one or more step IDs do not belong to sample %d", sampleID)
	}

	for i, stepID := range orderedIDs {
		if _, err := tx.Exec(ctx,
			`UPDATE fabrication_steps SET step_order = $1 WHERE step_id = $2 AND sample_id = $3`,
			i+1, stepID, sampleID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ------------------------------------------------- Supporting data queries --

func (s *DBService) GetApprovedUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx,
		`SELECT user_id, username FROM users
		  WHERE is_approved = true AND COALESCE(deleted, false) = false
		  ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *DBService) GetAllEquipment(ctx context.Context) ([]Equipment, error) {
	rows, err := s.db.Query(ctx,
		`SELECT equipment_id, name FROM equipment ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var equip []Equipment
	for rows.Next() {
		var e Equipment
		if err := rows.Scan(&e.ID, &e.Name); err != nil {
			return nil, err
		}
		equip = append(equip, e)
	}
	return equip, rows.Err()
}

func (s *DBService) IsAdmin(ctx context.Context, userID int) (bool, error) {
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		"SELECT COALESCE(admin, false) FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
		userID).Scan(&isAdmin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return isAdmin, nil
}

// GetUsernameByID looks up a username for the given user ID.
// Returns "" without error if the user does not exist.
func (s *DBService) GetUsernameByID(ctx context.Context, userID int) (string, error) {
	var username string
	err := s.db.QueryRow(ctx, "SELECT username FROM users WHERE user_id = $1", userID).Scan(&username)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return username, err
}

// GetUsernameByIDApproved looks up an approved user's username.
// Returns an error if not found.
func (s *DBService) GetUsernameByIDApproved(ctx context.Context, userID int) (string, error) {
	var username string
	err := s.db.QueryRow(ctx,
		"SELECT username FROM users WHERE user_id = $1 AND is_approved = true AND COALESCE(deleted, false) = false", userID).Scan(&username)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("user %d not found or not approved", userID)
	}
	return username, err
}

// AttachmentFilePath returns the file path for a sample attachment by ID,
// used for serving files from /attachment/{id}.
func (s *DBService) AttachmentFilePath(ctx context.Context, attachmentID int) (string, error) {
	var filePath string
	err := s.db.QueryRow(ctx,
		"SELECT attachment_address FROM attachments WHERE attachment_id = $1 AND source_type = 'sample'",
		attachmentID).Scan(&filePath)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", web.ErrNotFound
	}
	return filePath, err
}

// APIListParams holds query parameters for the list API.
type APIListParams struct {
	Q      string
	Owner  string
	Status string
	From   string
	To     string
	Limit  int
	Offset int
}

// APIListResult holds the paginated result of a sample list query.
type APIListResult struct {
	Items  []APISampleItem
	Total  int
	Limit  int
	Offset int
}

// APISampleItem is the JSON representation of a sample in list responses.
type APISampleItem struct {
	ID              int       `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Keywords        string    `json:"keywords"`
	Owner           string    `json:"owner"`
	OwnerUserID     *int      `json:"owner_user_id,omitempty"`
	OwnerUsername   *string   `json:"owner_username,omitempty"`
	Status          string    `json:"status"`
	AttachmentCount int       `json:"attachment_count"`
	CreatedAt       time.Time `json:"created_at"`
}

// APISampleDetail is the JSON representation of a full sample detail response.
type APISampleDetail struct {
	ID              int                  `json:"id"`
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	Keywords        string               `json:"keywords"`
	Owner           string               `json:"owner"`
	OwnerUserID     *int                 `json:"owner_user_id,omitempty"`
	OwnerUsername   *string              `json:"owner_username,omitempty"`
	Status          string               `json:"status"`
	SamplePrep      string               `json:"sample_prep"`
	AttachmentCount int                  `json:"attachment_count"`
	CreatedAt       time.Time            `json:"created_at"`
	Steps           []APIFabricationStep `json:"fabrication_steps"`
	Attachments     []APIAttachment      `json:"attachments"`
}

// APIFabricationStep is the JSON representation of a fabrication step.
type APIFabricationStep struct {
	ID             int       `json:"id"`
	SampleID       int       `json:"sample_id"`
	StepOrder      int       `json:"step_order"`
	StepDate       *string   `json:"step_date,omitempty"`
	PersonUserID   *int      `json:"person_user_id,omitempty"`
	PersonUsername *string   `json:"person_username,omitempty"`
	Description    string    `json:"description"`
	EquipmentID    *int      `json:"equipment_id,omitempty"`
	EquipmentName  *string   `json:"equipment_name,omitempty"`
	EquipmentText  *string   `json:"equipment_text,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// APIAttachment is the JSON representation of a sample attachment.
type APIAttachment struct {
	ID           int       `json:"id"`
	OriginalName string    `json:"original_name"`
	URL          string    `json:"url"`
	ContentType  string    `json:"content_type"`
	IsImage      bool      `json:"is_image"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

// FabricationStepToAPI converts a FabricationStep to its API representation.
func FabricationStepToAPI(s FabricationStep) APIFabricationStep {
	item := APIFabricationStep{
		ID:             s.ID,
		SampleID:       s.SampleID,
		StepOrder:      s.StepOrder,
		PersonUserID:   s.PersonUserID,
		PersonUsername: s.PersonUsername,
		Description:    s.Description,
		EquipmentID:    s.EquipmentID,
		EquipmentName:  s.EquipmentName,
		EquipmentText:  s.EquipmentText,
		CreatedAt:      s.CreatedAt,
	}
	if s.StepDate != nil {
		d := s.StepDate.Format("2006-01-02")
		item.StepDate = &d
	}
	return item
}

// AttachmentToAPI converts an Attachment to its API representation.
func AttachmentToAPI(a Attachment) APIAttachment {
	return APIAttachment{
		ID:           a.ID,
		OriginalName: a.OriginalName,
		URL:          fmt.Sprintf("/attachment/%d", a.ID),
		ContentType:  a.ContentType,
		IsImage:      a.IsImage,
		UploadedAt:   a.UploadedAt,
	}
}

// APIListSamples performs a paginated sample list query with optional filters.
func (s *DBService) APIListSamples(ctx context.Context, p APIListParams) (APIListResult, error) {
	var conditions []string
	var args []interface{}

	if p.Q != "" {
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf(
			`(s.sample_name ILIKE $%[1]d ESCAPE '\' OR s.sample_owner ILIKE $%[1]d ESCAPE '\' OR s.sample_keywords ILIKE $%[1]d ESCAPE '\')`, idx))
		args = append(args, "%"+escapeLike(p.Q)+"%")
	}
	if p.Owner != "" {
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf(`(s.sample_owner ILIKE $%[1]d ESCAPE '\' OR u.username ILIKE $%[1]d ESCAPE '\')`, idx))
		args = append(args, "%"+escapeLike(p.Owner)+"%")
	}
	if p.Status != "" {
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf("s.status = $%d", idx))
		args = append(args, p.Status)
	}
	if p.From != "" {
		if _, err := time.Parse("2006-01-02", p.From); err != nil {
			return APIListResult{}, fmt.Errorf("invalid from date: use YYYY-MM-DD: %w", web.ErrValidation)
		}
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf("s.created_at >= $%d", idx))
		args = append(args, p.From)
	}
	if p.To != "" {
		if _, err := time.Parse("2006-01-02", p.To); err != nil {
			return APIListResult{}, fmt.Errorf("invalid to date: use YYYY-MM-DD: %w", web.ErrValidation)
		}
		idx := len(args) + 1
		conditions = append(conditions, fmt.Sprintf("s.created_at < $%d::date + interval '1 day'", idx))
		args = append(args, p.To)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	countJoin := ""
	if p.Owner != "" {
		countJoin = " LEFT JOIN users u ON s.owner_user_id = u.user_id"
	}
	var total int
	if err := s.db.QueryRow(ctx,
		"SELECT COUNT(*) FROM samples s"+countJoin+whereClause, args...).Scan(&total); err != nil {
		return APIListResult{}, err
	}

	pagedArgs := make([]interface{}, len(args), len(args)+2)
	copy(pagedArgs, args)
	pagedArgs = append(pagedArgs, p.Limit, p.Offset)

	mainQ := `SELECT s.sample_id,
		       s.sample_name,
		       COALESCE(s.sample_description, ''),
		       COALESCE(s.sample_keywords, ''),
		       COALESCE(s.sample_owner, ''),
		       s.owner_user_id,
		       u.username,
		       COALESCE(s.status, 'active'),
		       COUNT(a.attachment_id) AS attachment_count,
		       s.created_at
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		  LEFT JOIN attachments a ON a.source_id = s.sample_id AND a.source_type = 'sample'` +
		whereClause +
		fmt.Sprintf(` GROUP BY s.sample_id, u.username
		 ORDER BY s.sample_name
		 LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)

	rows, err := s.db.Query(ctx, mainQ, pagedArgs...)
	if err != nil {
		return APIListResult{}, err
	}
	defer rows.Close()

	items := make([]APISampleItem, 0)
	for rows.Next() {
		var item APISampleItem
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.Keywords, &item.Owner,
			&item.OwnerUserID, &item.OwnerUsername, &item.Status,
			&item.AttachmentCount, &item.CreatedAt,
		); err != nil {
			return APIListResult{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return APIListResult{}, err
	}

	return APIListResult{
		Items:  items,
		Total:  total,
		Limit:  p.Limit,
		Offset: p.Offset,
	}, nil
}

// APIGetSample fetches a full sample detail for API response.
func (s *DBService) APIGetSample(ctx context.Context, id int) (APISampleDetail, error) {
	var detail APISampleDetail
	var samplePrep string

	err := s.db.QueryRow(ctx, `
		SELECT s.sample_id,
		       s.sample_name,
		       COALESCE(s.sample_description, ''),
		       COALESCE(s.sample_keywords, ''),
		       COALESCE(s.sample_owner, ''),
		       s.owner_user_id,
		       u.username,
		       COALESCE(s.status, 'active'),
		       COALESCE(s.sample_prep, ''),
		       s.created_at
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		 WHERE s.sample_id = $1`, id).Scan(
		&detail.ID, &detail.Name, &detail.Description, &detail.Keywords, &detail.Owner,
		&detail.OwnerUserID, &detail.OwnerUsername, &detail.Status,
		&samplePrep, &detail.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return detail, web.ErrNotFound
		}
		return detail, err
	}
	detail.SamplePrep = samplePrep

	steps, err := s.GetSteps(ctx, id)
	if err != nil {
		return detail, err
	}
	detail.Steps = make([]APIFabricationStep, 0, len(steps))
	for _, st := range steps {
		detail.Steps = append(detail.Steps, FabricationStepToAPI(st))
	}

	atts, err := s.GetAttachments(ctx, id)
	if err != nil {
		return detail, err
	}
	detail.AttachmentCount = len(atts)
	detail.Attachments = make([]APIAttachment, 0, len(atts))
	for _, a := range atts {
		detail.Attachments = append(detail.Attachments, AttachmentToAPI(a))
	}

	return detail, nil
}

// APICreateSample inserts a new sample and returns the created item. An empty
// status falls back to the DB default ("active").
func (s *DBService) APICreateSample(ctx context.Context, name, description, keywords, owner, samplePrep, status string, ownerUserID *int) (APISampleItem, error) {
	var id int
	var createdAt time.Time
	var ownerUsername *string
	var scannedStatus string
	err := s.db.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO samples (sample_name, sample_description, sample_keywords, sample_owner, sample_prep, owner_user_id, status)
			VALUES ($1, $2, $3, $4, $5, $6, COALESCE(NULLIF($7, ''), 'active'))
			RETURNING sample_id, owner_user_id, created_at, status
		)
		SELECT ins.sample_id, ins.created_at, COALESCE(ins.status, 'active'), u.username
		  FROM ins
		  LEFT JOIN users u ON u.user_id = ins.owner_user_id`,
		name, description, keywords, owner, samplePrep, ownerUserID, status,
	).Scan(&id, &createdAt, &scannedStatus, &ownerUsername)
	if err != nil {
		return APISampleItem{}, err
	}
	return APISampleItem{
		ID:            id,
		Name:          name,
		Description:   description,
		Keywords:      keywords,
		Owner:         owner,
		OwnerUserID:   ownerUserID,
		OwnerUsername: ownerUsername,
		Status:        scannedStatus,
		CreatedAt:     createdAt,
	}, nil
}

// APIUpdateSample updates a sample's editable fields. An empty status leaves
// the existing status unchanged.
func (s *DBService) APIUpdateSample(ctx context.Context, id int, name, description, keywords, owner, samplePrep, status string, ownerUserID *int) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE samples
		   SET sample_name        = $1,
		       sample_description = $2,
		       sample_keywords    = $3,
		       sample_owner       = $4,
		       sample_prep        = $5,
		       owner_user_id      = $6,
		       status             = COALESCE(NULLIF($7, ''), status)
		 WHERE sample_id = $8`,
		name, description, keywords, owner, samplePrep, ownerUserID, status, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// APIDeleteSample deletes a sample and its attachment rows atomically,
// returning the attachment file paths so the caller can remove the files
// from disk. Both deletes run inside a transaction so that no orphan rows
// are left if either statement fails.
func (s *DBService) APIDeleteSample(ctx context.Context, id int) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT attachment_address FROM attachments WHERE source_id = $1 AND source_type = 'sample'`, id)
	if err != nil {
		return nil, err
	}
	var filePaths []string
	for rows.Next() {
		var fp string
		if scanErr := rows.Scan(&fp); scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		filePaths = append(filePaths, fp)
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, rows.Err()
	}

	tag, err := tx.Exec(ctx, `DELETE FROM samples WHERE sample_id = $1`, id)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, web.ErrNotFound
	}

	// The attachments table has no FK constraint with ON DELETE CASCADE, so
	// attachment rows must be removed explicitly within the same transaction.
	if _, err := tx.Exec(ctx,
		`DELETE FROM attachments WHERE source_id = $1 AND source_type = 'sample'`, id); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return filePaths, nil
}

// SampleExistsByID reports whether a sample exists.
func (s *DBService) SampleExistsByID(ctx context.Context, id int) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM samples WHERE sample_id = $1)", id).Scan(&exists)
	return exists, err
}

// --- helper used by the API handler layer ---

// ParsePagination extracts limit/offset query params with sensible defaults.
// This is defined here so both the HTTP handler and tests can use it.
func ParsePagination(limitStr, offsetStr string) (limit, offset int) {
	limit = 100
	offset = 0
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if offsetStr != "" {
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			offset = n
		}
	}
	return
}
