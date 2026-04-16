package booking

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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
	GetUserBookings(ctx context.Context, userID int) ([]Booking, error)
	CheckConflict(ctx context.Context, equipmentID int, start, end time.Time) (bool, error)
	Create(ctx context.Context, p CreateParams) error
	Delete(ctx context.Context, bookingID, userID int, isAdmin bool) error
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
		`SELECT equipment_id, name, coalesce(description, 'N/A'), coalesce(location, 'N/A')
         FROM equipment
         ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var equipment []Equipment
	for rows.Next() {
		var eq Equipment
		if err := rows.Scan(&eq.ID, &eq.Name, &eq.Description, &eq.Location); err != nil {
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
		`SELECT admin FROM users WHERE user_id = $1 AND COALESCE(deleted, false) = false`,
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
                b.start_time AT TIME ZONE 'UTC',
                b.end_time AT TIME ZONE 'UTC',
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

// GetUserBookings returns all future bookings for a given user.
func (s *DBService) GetUserBookings(ctx context.Context, userID int) ([]Booking, error) {
	rows, err := s.db.Query(ctx,
		`SELECT booking_id, equipment_id, user_id, start_time, end_time, purpose, created_at
         FROM bookings
         WHERE user_id = $1 AND end_time >= NOW()
         ORDER BY start_time`,
		userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(&b.ID, &b.EquipmentID, &b.UserID, &b.StartTime, &b.EndTime, &b.Purpose, &b.CreatedAt); err != nil {
			return nil, err
		}
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

// Create inserts a new booking.
// Returns web.ErrConflict if an exclusion constraint violation occurs (concurrent insert).
func (s *DBService) Create(ctx context.Context, p CreateParams) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO bookings (equipment_id, user_id, start_time, end_time, purpose)
         VALUES ($1, $2, $3, $4, $5)`,
		p.EquipmentID, p.UserID, p.StartTime, p.EndTime, p.Purpose)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return web.ErrConflict
		}
		return err
	}
	return nil
}

// Delete removes a booking. Admins may delete any booking; non-admins must own it.
// Returns web.ErrNotFound if the booking doesn't exist, web.ErrForbidden if
// the user is not the owner and is not an admin.
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
	_, err = s.db.Exec(ctx, "DELETE FROM bookings WHERE booking_id = $1", bookingID)
	return err
}
