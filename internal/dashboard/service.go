package dashboard

import (
	"context"
	"database/sql"
	"time"

	"sampleDB/internal/dbiface"
)

// Service aggregates the data shown on the dashboard.
type Service interface {
	KPIs(ctx context.Context) (KPIs, error)
	RecentSamples(ctx context.Context, limit int) ([]RecentSample, error)
	TodayBookings(ctx context.Context, now time.Time, loc *time.Location, userID int, isAdmin bool) ([]TodayBooking, error)
}

// DBService is the concrete Service backed by a pgx pool.
type DBService struct {
	db dbiface.Pool
}

// NewService constructs a DBService.
func NewService(db dbiface.Pool) *DBService {
	return &DBService{db: db}
}

// KPIs runs four independent count queries and returns the aggregate. Errors
// from any single query bubble up; partial success is not returned because the
// dashboard is rendered as a single snapshot.
func (s *DBService) KPIs(ctx context.Context) (KPIs, error) {
	var k KPIs
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM samples`).Scan(&k.TotalSamples); err != nil {
		return k, err
	}
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM bookings
		  WHERE CURRENT_TIMESTAMP >= start_time AND CURRENT_TIMESTAMP < end_time`).Scan(&k.ActiveBookings); err != nil {
		return k, err
	}
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM articles`).Scan(&k.WikiArticles); err != nil {
		return k, err
	}
	if err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM users
		  WHERE COALESCE(is_approved, false) = false
		    AND COALESCE(deleted, false) = false`).Scan(&k.PendingApprovals); err != nil {
		return k, err
	}
	return k, nil
}

// RecentSamples returns the most-recently created samples for the landing-page
// card list. The first-image join lets the template show a thumbnail without
// a second round-trip per card.
func (s *DBService) RecentSamples(ctx context.Context, limit int) ([]RecentSample, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.Query(ctx, `
		SELECT s.sample_id,
		       s.sample_name,
		       COALESCE(s.sample_description, ''),
		       COALESCE(s.sample_owner, ''),
		       u.username,
		       COALESCE(s.status, 'active'),
		       s.created_at,
		       first_img.attachment_id
		  FROM samples s
		  LEFT JOIN users u ON s.owner_user_id = u.user_id
		  LEFT JOIN LATERAL (
		      SELECT attachment_id FROM attachments
		       WHERE source_type = 'sample' AND source_id = s.sample_id
		         AND attachment_address ~* '\.(jpg|jpeg|png|gif|webp|svg)$'
		       ORDER BY attachment_id ASC LIMIT 1
		  ) first_img ON true
		 ORDER BY s.created_at DESC, s.sample_id DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecentSample
	for rows.Next() {
		var r RecentSample
		var ownerName sql.NullString
		var imgID sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Owner,
			&ownerName, &r.Status, &r.CreatedAt, &imgID); err != nil {
			return nil, err
		}
		if ownerName.Valid {
			s := ownerName.String
			r.OwnerUsername = &s
		}
		if imgID.Valid {
			id := int(imgID.Int64)
			r.FirstImageID = &id
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TodayBookings returns bookings for today in loc that the requesting user is
// allowed to see, ordered by start time. Visibility mirrors the calendar:
// offline/maintenance equipment is visible to everyone; online equipment is
// only visible to admins or users with an explicit permission row.
func (s *DBService) TodayBookings(ctx context.Context, now time.Time, loc *time.Location, userID int, isAdmin bool) ([]TodayBooking, error) {
	if loc == nil {
		loc = time.UTC
	}
	nowLocal := now.In(loc)
	dayStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)

	rows, err := s.db.Query(ctx, `
		SELECT b.booking_id,
		       b.equipment_id,
		       e.name,
		       COALESCE(e.status, 'online'),
		       b.user_id,
		       u.username,
		       b.start_time,
		       b.end_time,
		       COALESCE(b.purpose, '')
		  FROM bookings b
		  JOIN equipment e ON b.equipment_id = e.equipment_id
		  JOIN users u ON b.user_id = u.user_id
		 WHERE b.start_time < $2 AND b.end_time > $1
		   AND (
		         COALESCE(e.status, 'online') IN ('offline', 'maintenance')
		         OR $3 = true
		         OR EXISTS (
		             SELECT 1 FROM user_equipment_permissions
		              WHERE user_id = $4 AND equipment_id = e.equipment_id
		         )
		       )
		 ORDER BY b.start_time`, dayStart, dayEnd, isAdmin, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TodayBooking
	for rows.Next() {
		var b TodayBooking
		if err := rows.Scan(&b.ID, &b.EquipmentID, &b.EquipmentName, &b.EquipmentStatus,
			&b.UserID, &b.Username, &b.StartTime, &b.EndTime, &b.Purpose); err != nil {
			return nil, err
		}
		b.StartTime = b.StartTime.In(loc)
		b.EndTime = b.EndTime.In(loc)
		out = append(out, b)
	}
	return out, rows.Err()
}
