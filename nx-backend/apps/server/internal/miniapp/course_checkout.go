package miniapp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/siteconfig"
)

// Direct checkout uses the authenticated profile as-is; an absent phone remains
// absent. Consultation leads are created only by the contact form workflow.
func (s *Store) CreateOrReuseCourseBooking(ctx context.Context, userID int64, course siteconfig.MiniappCourse) (CourseBooking, error) {
	if userID <= 0 || course.ID == "" || !course.Enabled || course.PriceCents <= 0 || course.PriceCents > siteconfig.MaxCoursePriceCents {
		return CourseBooking{}, ErrOrderNotPayable
	}
	c, cancel := s.ctx(ctx)
	defer cancel()
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return CourseBooking{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockCourseTarget(c, tx, userID, course.ID); err != nil {
		return CourseBooking{}, err
	}
	var owned bool
	if err := tx.QueryRowContext(c, `SELECT EXISTS(SELECT 1 FROM bookings WHERE wx_user_id=$1 AND course_id=$2 AND payment_status='paid')`, userID, course.ID).Scan(&owned); err != nil {
		return CourseBooking{}, err
	}
	if owned {
		return CourseBooking{}, ErrOrderAlreadyOwned
	}
	var b CourseBooking
	var id int64
	err = tx.QueryRowContext(c, `SELECT id,wx_user_id,course_id,course_title,price_cents,payment_mode,payment_status FROM bookings
 WHERE wx_user_id=$1 AND course_id=$2 AND price_cents=$3 AND payment_mode='paid' AND payment_status='pending'
 ORDER BY create_time DESC,id DESC LIMIT 1 FOR UPDATE`, userID, course.ID, course.PriceCents).
		Scan(&id, &b.UserID, &b.CourseID, &b.CourseTitle, &b.PriceCents, &b.PaymentMode, &b.PaymentStatus)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(c, `INSERT INTO bookings (wx_user_id,kind,contact_name,phone,intent,course_id,course_title,price_cents,payment_mode,payment_status)
  SELECT id,'course',nickname,phone,$3,$2,$3,$4,'paid','pending' FROM wx_users WHERE id=$1
  RETURNING id,wx_user_id,course_id,course_title,price_cents,payment_mode,payment_status`, userID, course.ID, strings.TrimSpace(course.Title), course.PriceCents).
			Scan(&id, &b.UserID, &b.CourseID, &b.CourseTitle, &b.PriceCents, &b.PaymentMode, &b.PaymentStatus)
	}
	if err != nil {
		return CourseBooking{}, err
	}
	if err := tx.Commit(); err != nil {
		return CourseBooking{}, err
	}
	b.ID = strconv.FormatInt(id, 10)
	return b, nil
}

func lockCourseTarget(ctx context.Context, tx *sql.Tx, userID int64, courseID string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fmt.Sprintf("course:%d:%s", userID, courseID))
	return err
}

// Course-level locking coordinates differently priced booking snapshots with
// callbacks and checkout retries. Always acquire it before the order lock.
func lockCourseBookingTarget(ctx context.Context, tx *sql.Tx, userID, bookingID int64) error {
	var courseID string
	if err := tx.QueryRowContext(ctx, `SELECT course_id FROM bookings WHERE id=$1 AND wx_user_id=$2`, bookingID, userID).Scan(&courseID); err != nil {
		return err
	}
	return lockCourseTarget(ctx, tx, userID, courseID)
}
