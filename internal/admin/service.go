package admin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/dbiface"
	"sampleDB/internal/web"
)

// Service is the domain interface for admin operations.
// All database access goes through this interface, making handlers testable.
type Service interface {
	// Query helpers
	IsAdmin(ctx context.Context, userID int) (bool, error)
	GetAllEquipment(ctx context.Context) ([]Equipment, error)
	GetAllGroups(ctx context.Context) ([]Group, error)
	GetAllUsers(ctx context.Context, allEquipment []Equipment) ([]UserAccess, error)

	// User management
	UpdateAccess(ctx context.Context, p UpdateAccessParams) error
	SetAdmin(ctx context.Context, userID int, isAdmin bool) error
	SetEmail(ctx context.Context, userID int, email string) error
	DeleteUser(ctx context.Context, userID int) (rowsAffected int64, err error)
	ResetPassword(ctx context.Context, userID int) (newPassword string, err error)

	// Equipment management
	AddEquipment(ctx context.Context, name string) error
	DeleteEquipment(ctx context.Context, equipmentID int) error
	SetEquipmentStatus(ctx context.Context, equipmentID int, status string) error
	AddEquipmentGroup(ctx context.Context, equipmentID, groupID int) error
	RemoveEquipmentGroup(ctx context.Context, equipmentID, groupID int) error

	// Group management
	AddGroup(ctx context.Context, name string) error
	DeleteGroup(ctx context.Context, groupID int) error

	// Reports
	GetEquipmentReport(ctx context.Context, equipmentID int, startDate, endDate string) ([]BookingRecord, error)

	// API
	ListAPIUsers(ctx context.Context) ([]APIUser, error)

	// Notifications
	ListNotifications(ctx context.Context) ([]Notification, error)
	CreateNotification(ctx context.Context, message, level, author string) (Notification, error)
	DeleteNotification(ctx context.Context, id int64) error

	// AFM working hours
	GetWorkingHours(ctx context.Context) (WorkingHours, error)
	SetWorkingHours(ctx context.Context, w WorkingHours) error
	ListWorkingHoursExceptions(ctx context.Context, from string) ([]WorkingHoursException, error)
	SetWorkingHoursException(ctx context.Context, e WorkingHoursException) error
	DeleteWorkingHoursException(ctx context.Context, date string) error
}

// DBService is the concrete Service implementation backed by PostgreSQL.
type DBService struct {
	db dbiface.Pool
}

// NewService constructs a DBService backed by pool.
func NewService(db dbiface.Pool) *DBService {
	return &DBService{db: db}
}

// IsAdmin returns true if userID is an active admin user.
func (s *DBService) IsAdmin(ctx context.Context, userID int) (bool, error) {
	var isAdmin bool
	err := s.db.QueryRow(ctx,
		`SELECT COALESCE(admin, false) FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false`,
		userID).Scan(&isAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return isAdmin, err
}

// GetAllEquipment returns all equipment ordered by name, including status and
// the names of groups with access via equipment_group_access.
func (s *DBService) GetAllEquipment(ctx context.Context) ([]Equipment, error) {
	rows, err := s.db.Query(ctx,
		`SELECT e.equipment_id,
                e.name,
                COALESCE(e.description, 'N/A'),
                COALESCE(e.location, 'N/A'),
                COALESCE(e.status, 'online'),
                COALESCE(
                    ARRAY(
                        SELECT g.name
                          FROM equipment_group_access ega
                          JOIN groups g ON g.group_id = ega.group_id
                         WHERE ega.equipment_id = e.equipment_id
                         ORDER BY g.name
                    ),
                    '{}'::text[]
                ) AS group_names
         FROM equipment e
         ORDER BY e.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var equipment []Equipment
	for rows.Next() {
		var eq Equipment
		if err := rows.Scan(&eq.ID, &eq.Name, &eq.Description, &eq.Location, &eq.Status, &eq.Groups); err != nil {
			return nil, err
		}
		if eq.Groups == nil {
			eq.Groups = []string{}
		}
		equipment = append(equipment, eq)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return equipment, nil
}

// GetAllGroups returns all groups ordered by name.
func (s *DBService) GetAllGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.Query(ctx,
		`SELECT group_id, name FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []Group
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return groups, nil
}

// GetAllUsers returns all non-deleted users with their equipment access resolved
// against allEquipment. The caller is expected to fetch allEquipment first via
// GetAllEquipment so a single DB round-trip retrieves all equipment data.
func (s *DBService) GetAllUsers(ctx context.Context, allEquipment []Equipment) ([]UserAccess, error) {
	rows, err := s.db.Query(ctx, `
        SELECT
            u.user_id,
            u.username,
            COALESCE(u.email, '') AS email,
            COALESCE(u.is_approved, false) AS is_approved,
            COALESCE(u.admin, false) AS admin,
            u.afm_outside_hours_allowed,
            btrim(u."group") AS group_name,
            COALESCE(
                array_agg(uep.equipment_id) FILTER (WHERE uep.equipment_id IS NOT NULL),
                '{}'::int[]
            ) as equipment_ids
        FROM users u
        LEFT JOIN user_equipment_permissions uep ON u.user_id = uep.user_id
        WHERE COALESCE(u.deleted, false) = false
        GROUP BY u.user_id, u.username, u.email, u.is_approved, u.admin, u.afm_outside_hours_allowed, group_name
        ORDER BY u.username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	equipmentByID := make(map[int]Equipment, len(allEquipment))
	for _, eq := range allEquipment {
		equipmentByID[eq.ID] = eq
	}

	var users []UserAccess
	for rows.Next() {
		var (
			u            UserAccess
			groupName    sql.NullString
			equipmentIDs []int32
		)
		if err := rows.Scan(&u.UserID, &u.Username, &u.Email, &u.Approved, &u.Admin, &u.AFMOutsideHoursAllowed, &groupName, &equipmentIDs); err != nil {
			return nil, err
		}
		if groupName.Valid {
			u.GroupName = groupName.String
		}
		u.EquipmentAccess = make([]Equipment, 0, len(equipmentIDs))
		for _, id := range equipmentIDs {
			if eq, ok := equipmentByID[int(id)]; ok {
				u.EquipmentAccess = append(u.EquipmentAccess, eq)
			}
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// UpdateAccess applies a transactional update to a user's approval status,
// group, and equipment permissions. All changes are committed atomically; any
// failure rolls back the entire update.
func (s *DBService) UpdateAccess(ctx context.Context, p UpdateAccessParams) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var groupParam interface{}
	if p.GroupName != nil {
		name := strings.TrimSpace(*p.GroupName)
		if name != "" {
			groupParam = name
		}
	}

	userTag, err := tx.Exec(ctx,
		`UPDATE users
		    SET is_approved = $1, "group" = $2,
		        afm_outside_hours_allowed = COALESCE($3, afm_outside_hours_allowed)
		  WHERE user_id = $4 AND COALESCE(deleted, false) = false`,
		p.Approved, groupParam, p.AFMOutsideHoursAllowed, p.UserID)
	if err != nil {
		return fmt.Errorf("update user approval: %w", err)
	}
	if userTag.RowsAffected() == 0 {
		return web.ErrNotFound
	}

	if _, err = tx.Exec(ctx,
		"DELETE FROM user_equipment_permissions WHERE user_id = $1",
		p.UserID); err != nil {
		return fmt.Errorf("delete permissions: %w", err)
	}

	for _, equipID := range p.Equipment {
		if _, err = tx.Exec(ctx,
			"INSERT INTO user_equipment_permissions (user_id, equipment_id) VALUES ($1, $2)",
			p.UserID, equipID); err != nil {
			return fmt.Errorf("insert permission for equipment %d: %w", equipID, err)
		}
	}

	return tx.Commit(ctx)
}

// SetAdmin sets or clears admin status for the given user.
// Returns web.ErrNotFound if no such active user exists.
func (s *DBService) SetAdmin(ctx context.Context, userID int, isAdmin bool) error {
	cmdTag, err := s.db.Exec(ctx,
		"UPDATE users SET admin = $1 WHERE user_id = $2 AND COALESCE(deleted, false) = false",
		isAdmin, userID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// SetEmail links email to the given user. The caller must validate it with
// web.NormalizeEmail first. Returns web.ErrEmailTaken if another account
// already uses the address and web.ErrNotFound if no such active user exists.
func (s *DBService) SetEmail(ctx context.Context, userID int, email string) error {
	cmdTag, err := s.db.Exec(ctx,
		"UPDATE users SET email = $1 WHERE user_id = $2 AND COALESCE(deleted, false) = false",
		email, userID)
	if err != nil {
		if web.IsUniqueViolation(err) {
			return web.ErrEmailTaken
		}
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// DeleteUser soft-deletes a user by setting deleted=true and revoking approval.
// Returns the number of rows affected so callers can detect not-found cases.
func (s *DBService) DeleteUser(ctx context.Context, userID int) (int64, error) {
	cmdTag, err := s.db.Exec(ctx,
		`UPDATE users SET deleted = true, is_approved = false
         WHERE user_id = $1 AND COALESCE(deleted, false) = false`,
		userID)
	if err != nil {
		return 0, err
	}
	return cmdTag.RowsAffected(), nil
}

// ResetPassword generates a new random 12-character alphanumeric password, hashes
// it, and stores it for the given user. Returns the plaintext password so the
// caller can display it to the admin. Returns web.ErrNotFound if the user does
// not exist.
func (s *DBService) ResetPassword(ctx context.Context, userID int) (string, error) {
	password, err := generatePassword(12)
	if err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	cmdTag, err := s.db.Exec(ctx,
		`UPDATE users SET password_hash = $1 WHERE user_id = $2 AND COALESCE(deleted, false) = false`,
		string(hashed), userID)
	if err != nil {
		return "", err
	}
	if cmdTag.RowsAffected() == 0 {
		return "", web.ErrNotFound
	}
	return password, nil
}

// AddEquipment inserts a new equipment record.
func (s *DBService) AddEquipment(ctx context.Context, name string) error {
	_, err := s.db.Exec(ctx, "INSERT INTO equipment (name) VALUES ($1)", name)
	return err
}

// DeleteEquipment removes an equipment record along with all related bookings
// and user permissions inside a single transaction.
func (s *DBService) DeleteEquipment(ctx context.Context, equipmentID int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, "DELETE FROM bookings WHERE equipment_id = $1", equipmentID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM user_equipment_permissions WHERE equipment_id = $1", equipmentID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "DELETE FROM equipment WHERE equipment_id = $1", equipmentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return tx.Commit(ctx)
}

// validEquipmentStatuses matches the CHECK constraint in DDL.
var validEquipmentStatuses = map[string]struct{}{
	"online":      {},
	"offline":     {},
	"maintenance": {},
}

// SetEquipmentStatus updates an equipment row's status column.
// Returns web.ErrNotFound if the equipment does not exist and an error if
// status is not one of the allowed values.
func (s *DBService) SetEquipmentStatus(ctx context.Context, equipmentID int, status string) error {
	if _, ok := validEquipmentStatuses[status]; !ok {
		return fmt.Errorf("invalid equipment status %q", status)
	}
	tag, err := s.db.Exec(ctx,
		"UPDATE equipment SET status = $1 WHERE equipment_id = $2",
		status, equipmentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// AddEquipmentGroup grants a group access to the given equipment. Duplicate
// rows are silently ignored (ON CONFLICT DO NOTHING).
func (s *DBService) AddEquipmentGroup(ctx context.Context, equipmentID, groupID int) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO equipment_group_access (equipment_id, group_id)
         VALUES ($1, $2)
         ON CONFLICT (equipment_id, group_id) DO NOTHING`,
		equipmentID, groupID)
	return err
}

// RemoveEquipmentGroup revokes a group's access to the given equipment.
// Returns nil even if no row existed (idempotent).
func (s *DBService) RemoveEquipmentGroup(ctx context.Context, equipmentID, groupID int) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM equipment_group_access WHERE equipment_id = $1 AND group_id = $2`,
		equipmentID, groupID)
	return err
}

// AddGroup inserts a new group. If the name already exists the insert is silently
// ignored (ON CONFLICT DO NOTHING).
func (s *DBService) AddGroup(ctx context.Context, name string) error {
	_, err := s.db.Exec(ctx,
		"INSERT INTO groups (name) VALUES ($1) ON CONFLICT (name) DO NOTHING",
		name)
	return err
}

// DeleteGroup looks up a group by ID, unlinks all users from it, then deletes it
// inside a single transaction. Returns web.ErrNotFound if the group does not exist.
func (s *DBService) DeleteGroup(ctx context.Context, groupID int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var groupName string
	if err := tx.QueryRow(ctx,
		"SELECT name FROM groups WHERE group_id = $1", groupID).Scan(&groupName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return web.ErrNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE users SET "group" = NULL WHERE btrim("group") = $1`, groupName); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM groups WHERE group_id = $1", groupID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetEquipmentReport returns the booking records matching the given equipment ID
// and date range (both inclusive). The dates should be in YYYY-MM-DD format.
func (s *DBService) GetEquipmentReport(ctx context.Context, equipmentID int, startDate, endDate string) ([]BookingRecord, error) {
	rows, err := s.db.Query(ctx, `
        SELECT
            b.start_time,
            b.end_time,
            b.purpose,
            u.username,
            COALESCE(NULLIF(btrim(u."group"), ''), 'Unassigned') AS user_group,
            e.name as equipment_name
        FROM bookings b
        JOIN users u ON b.user_id = u.user_id
        JOIN equipment e ON b.equipment_id = e.equipment_id
        WHERE b.equipment_id = $1
          AND b.start_time >= $2::date
          AND b.start_time < ($3::date + interval '1 day')
        ORDER BY b.start_time ASC`,
		equipmentID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []BookingRecord
	for rows.Next() {
		var rec BookingRecord
		if err := rows.Scan(&rec.StartTime, &rec.EndTime, &rec.Purpose,
			&rec.Username, &rec.GroupName, &rec.EquipmentName); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// ListAPIUsers returns all approved, non-deleted users for the /api/v1/users endpoint.
func (s *DBService) ListAPIUsers(ctx context.Context) ([]APIUser, error) {
	rows, err := s.db.Query(ctx,
		`SELECT user_id, username, COALESCE(btrim("group"), '')
         FROM users
         WHERE is_approved = true AND COALESCE(deleted, false) = false
         ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]APIUser, 0)
	for rows.Next() {
		var u APIUser
		if err := rows.Scan(&u.ID, &u.Username, &u.GroupName); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// ListNotifications returns all notifications ordered by newest first.
func (s *DBService) ListNotifications(ctx context.Context) ([]Notification, error) {
	rows, err := s.db.Query(ctx,
		`SELECT id, message, level, author, created_at
         FROM notifications
         ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notifs := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Message, &n.Level, &n.Author, &n.CreatedAt); err != nil {
			return nil, err
		}
		notifs = append(notifs, n)
	}
	return notifs, rows.Err()
}

// CreateNotification inserts a new notification and returns it with its assigned ID and timestamp.
func (s *DBService) CreateNotification(ctx context.Context, message, level, author string) (Notification, error) {
	var n Notification
	err := s.db.QueryRow(ctx,
		`INSERT INTO notifications (message, level, author)
         VALUES ($1, $2, $3)
         RETURNING id, message, level, author, created_at`,
		message, level, author,
	).Scan(&n.ID, &n.Message, &n.Level, &n.Author, &n.CreatedAt)
	return n, err
}

// DeleteNotification removes the notification with the given ID.
func (s *DBService) DeleteNotification(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM notifications WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// GetWorkingHours returns the regular schedule. The row is seeded on startup;
// the Mon-Fri 08:00-18:00 fallback only guards against it being removed by hand.
func (s *DBService) GetWorkingHours(ctx context.Context) (WorkingHours, error) {
	w := WorkingHours{StartMinute: 8 * 60, EndMinute: 18 * 60, Weekdays: []int32{1, 2, 3, 4, 5}}
	err := s.db.QueryRow(ctx,
		`SELECT start_minute, end_minute, weekdays FROM working_hours WHERE id = 1`,
	).Scan(&w.StartMinute, &w.EndMinute, &w.Weekdays)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, nil
	}
	return w, err
}

// SetWorkingHours replaces the regular schedule.
func (s *DBService) SetWorkingHours(ctx context.Context, w WorkingHours) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO working_hours (id, start_minute, end_minute, weekdays)
         VALUES (1, $1, $2, $3)
         ON CONFLICT (id) DO UPDATE
         SET start_minute = EXCLUDED.start_minute,
             end_minute = EXCLUDED.end_minute,
             weekdays = EXCLUDED.weekdays`,
		w.StartMinute, w.EndMinute, w.Weekdays)
	return err
}

// ListWorkingHoursExceptions returns exceptions on or after from (YYYY-MM-DD),
// soonest first.
func (s *DBService) ListWorkingHoursExceptions(ctx context.Context, from string) ([]WorkingHoursException, error) {
	rows, err := s.db.Query(ctx,
		`SELECT to_char(exception_date, 'YYYY-MM-DD'), start_minute, end_minute, note
         FROM working_hours_exceptions
         WHERE exception_date >= $1::date
         ORDER BY exception_date`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exceptions []WorkingHoursException
	for rows.Next() {
		var e WorkingHoursException
		if err := rows.Scan(&e.Date, &e.StartMinute, &e.EndMinute, &e.Note); err != nil {
			return nil, err
		}
		exceptions = append(exceptions, e)
	}
	return exceptions, rows.Err()
}

// SetWorkingHoursException adds an exception, replacing any existing one for the date.
func (s *DBService) SetWorkingHoursException(ctx context.Context, e WorkingHoursException) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO working_hours_exceptions (exception_date, start_minute, end_minute, note)
         VALUES ($1::date, $2, $3, $4)
         ON CONFLICT (exception_date) DO UPDATE
         SET start_minute = EXCLUDED.start_minute,
             end_minute = EXCLUDED.end_minute,
             note = EXCLUDED.note`,
		e.Date, e.StartMinute, e.EndMinute, e.Note)
	return err
}

// DeleteWorkingHoursException removes the exception for date. Returns
// web.ErrNotFound if there is none.
func (s *DBService) DeleteWorkingHoursException(ctx context.Context, date string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM working_hours_exceptions WHERE exception_date = $1::date`, date)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}

// generatePassword returns a cryptographically random alphanumeric string of
// the given length using crypto/rand.
func generatePassword(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid password length: %d", length)
	}
	const charset = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}
