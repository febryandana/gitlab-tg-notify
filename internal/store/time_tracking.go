package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TimeTrackingStore reads and writes the issue_time_tracking table: it
// remembers the last cumulative "total time spent" GitLab reported for an
// issue, so the webhook handler can compute the delta attributable to one
// specific comment instead of showing the running total on every message.
type TimeTrackingStore struct {
	db *sql.DB
}

func NewTimeTrackingStore(db *sql.DB) *TimeTrackingStore {
	return &TimeTrackingStore{db: db}
}

// GetLastTotalSecs returns the last known cumulative total, or ErrNotFound
// if this issue has never been seen before (callers should treat that as a
// baseline of 0).
func (s *TimeTrackingStore) GetLastTotalSecs(ctx context.Context, projectID, issueIID int) (int64, error) {
	var secs int64

	err := s.db.QueryRowContext(ctx, `
		SELECT last_total_secs
		FROM issue_time_tracking
		WHERE gitlab_project_id = ? AND gitlab_issue_iid = ?
	`, projectID, issueIID).Scan(&secs)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get time tracking (project=%d, iid=%d): %w", projectID, issueIID, err)
	}

	return secs, nil
}

// SetLastTotalSecs records the current cumulative total as the new baseline
// for the next delta calculation.
func (s *TimeTrackingStore) SetLastTotalSecs(ctx context.Context, projectID, issueIID int, secs int64) error {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO issue_time_tracking (
			gitlab_project_id, gitlab_issue_iid, last_total_secs, updated_at
		) VALUES (?, ?, ?, ?)
		ON CONFLICT (gitlab_project_id, gitlab_issue_iid) DO UPDATE SET
			last_total_secs = excluded.last_total_secs,
			updated_at      = excluded.updated_at
	`, projectID, issueIID, secs, now)
	if err != nil {
		return fmt.Errorf("set time tracking (project=%d, iid=%d): %w", projectID, issueIID, err)
	}

	return nil
}
