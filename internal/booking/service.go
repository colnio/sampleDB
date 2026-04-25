package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"sampleDB/internal/dbiface"
	"sampleDB/internal/web"
)

// Service is the domain interface for booking operations.
// All database access goes through this interface, making handlers testable.
type Service interface {
	// Equipment
	GetAllEquipment(ctx context.Context) ([]Equipment, error)
	CheckUserPermission(ctx context.Context, userID, equipmentID int) (bool, error)
	IsAdmin(ctx context.Context, userID int) (bool, error)

	// Bookings
	GetBookingsForWeek(ctx context.Context, start, end time.Time) ([]Booking, error)
	GetBooking(ctx context.Context, bookingID int) (Booking, error)
	CheckConflict(ctx context.Context, equipmentID int, start, end time.Time) (bool, error)
	Create(ctx context.Context, p CreateParams) (int, error)
	Delete(ctx context.Context, bookingID, userID int, isAdmin bool) error

	// Auth helpers
	VerifyUserPassword(ctx context.Context, userID int, password string) (bool, error)
}

// DBService is the concrete Service implementation backed by PostgreSQL.
type DBService struct {
	db  dbiface.Pool
	loc *time.Location
}

// NewService constructs a DBService backed by pool.
// loc is the local timezone used for parsing and displaying booking times.
func NewService(db dbiface.Pool, loc *time.Location) *DBService {
	if loc == nil {
		loc = time.UTC
	}
	return &DBService{db: db, loc: loc}
}

// GetAllEquipment returns all equipment ordered by name.
func (s *DBService) GetAllEquipment(ctx context.Context) ([]Equipment, error) {
	rows, err := s.db.Query(ctx,
		`SELECT equipment_id, name, coalesce(description, 'N/A'), coalesce(location, 'N/A'),
                coalesce(status, 'online')
         FROM equipment
         ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var equipment []Equipment
	for rows.Next() {
		var eq Equipment
		if err := rows.Scan(&eq.ID, &eq.Name, &eq.Description, &eq.Location, &eq.Status); err != nil {
			return nil, err
		}
		equipment = append(equipment, eq)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return equipment, nil
}

// CheckUserPermission returns true if userID has permission to book equipmentID.
func (s *DBService) CheckUserPermission(ctx context.Context, userID, equipmentID int) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(
            SELECT 1 FROM user_equipment_permissions
            WHERE user_id = $1 AND equipment_id = $2
        )`, userID, equipmentID).Scan(&exists)
	return exists, err
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

// GetBookingsForWeek returns all bookings that overlap the given [start, end) window.
func (s *DBService) GetBookingsForWeek(ctx context.Context, start, end time.Time) ([]Booking, error) {
	start = start.In(s.loc)
	end = end.In(s.loc)

	rows, err := s.db.Query(ctx,
		`SELECT b.booking_id, b.equipment_id, b.user_id, u.username,
                b.start_time,
                b.end_time,
                b.purpose, b.created_at
         FROM bookings b
         JOIN users u ON b.user_id = u.user_id
         WHERE b.start_time < $2 AND b.end_time > $1
         ORDER BY b.start_time`,
		start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.EquipmentID, &b.UserID, &b.Username,
			&b.StartTime, &b.EndTime, &b.Purpose, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.StartTime = b.StartTime.In(s.loc)
		b.EndTime = b.EndTime.In(s.loc)
		bookings = append(bookings, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(bookings) == 0 {
		return []Booking{}, nil
	}
	return bookings, nil
}

// CheckConflict returns true if a booking for equipmentID already overlaps [start, end).
// Uses half-open interval semantics (start inclusive, end exclusive) matching the
// bookings_no_overlap exclusion constraint, so back-to-back bookings are allowed.
func (s *DBService) CheckConflict(ctx context.Context, equipmentID int, start, end time.Time) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx,
		`SELECT EXISTS(
            SELECT 1 FROM bookings
            WHERE equipment_id = $1
            AND start_time < $3
            AND end_time > $2
        )`, equipmentID, start, end).Scan(&exists)
	return exists, err
}

// Create inserts a new booking and returns the new booking_id.
// Returns web.ErrConflict if an exclusion constraint violation occurs (concurrent insert).
// Retries once on deadlock (40P01), which can occur when concurrent inserts race on GiST locks.
func (s *DBService) Create(ctx context.Context, p CreateParams) (int, error) {
	const maxAttempts = 2
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var bookingID int
		err := s.db.QueryRow(ctx,
			`INSERT INTO bookings (equipment_id, user_id, start_time, end_time, purpose)
         VALUES ($1, $2, $3, $4, $5) RETURNING booking_id`,
			p.EquipmentID, p.UserID, p.StartTime, p.EndTime, p.Purpose).Scan(&bookingID)
		if err == nil {
			return bookingID, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23P01": // exclusion_violation
				return 0, web.ErrConflict
			case "40P01": // deadlock_detected — retry once
				if attempt < maxAttempts-1 {
					continue
				}
				return 0, err
			}
		}
		return 0, err
	}
	return 0, nil // unreachable: loop always returns
}

// Delete removes a booking. Admins may delete any booking; non-admins must own it.
// Returns web.ErrNotFound if the booking doesn't exist, web.ErrForbidden if
// the user is not the owner and is not an admin.
// GetBooking returns a single booking by ID.
func (s *DBService) GetBooking(ctx context.Context, bookingID int) (Booking, error) {
	var b Booking
	err := s.db.QueryRow(ctx,
		`SELECT b.booking_id, b.equipment_id, b.user_id, COALESCE(u.username,''),
		        b.start_time, b.end_time, COALESCE(b.purpose,''), b.created_at
		 FROM bookings b
		 LEFT JOIN users u ON b.user_id = u.user_id
		 WHERE b.booking_id = $1`, bookingID,
	).Scan(&b.ID, &b.EquipmentID, &b.UserID, &b.Username,
		&b.StartTime, &b.EndTime, &b.Purpose, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, web.ErrNotFound
	}
	return b, err
}

// VerifyUserPassword checks the given password against the stored bcrypt hash for the user.
func (s *DBService) VerifyUserPassword(ctx context.Context, userID int, password string) (bool, error) {
	var hash string
	err := s.db.QueryRow(ctx,
		"SELECT password_hash FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false",
		userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil, nil
}

func (s *DBService) Delete(ctx context.Context, bookingID, userID int, isAdmin bool) error {
	var ownerID int
	err := s.db.QueryRow(ctx,
		"SELECT user_id FROM bookings WHERE booking_id = $1", bookingID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return web.ErrNotFound
		}
		return err
	}
	if ownerID != userID && !isAdmin {
		return web.ErrForbidden
	}
	cmdTag, err := s.db.Exec(ctx, "DELETE FROM bookings WHERE booking_id = $1", bookingID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return web.ErrNotFound
	}
	return nil
}
