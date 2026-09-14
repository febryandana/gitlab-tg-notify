package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned by Get when no root message is recorded yet for
// the given (project, issue) pair — see PRD §13.3.
var ErrNotFound = errors.New("store: issue thread not found")

// IssueThreadStore reads and writes the issue_threads table.
type IssueThreadStore struct {
	db *sql.DB
}

func NewIssueThreadStore(db *sql.DB) *IssueThreadStore {
	return &IssueThreadStore{db: db}
}

// Insert records the Telegram message that first announced an issue.
// Called once, right after the "open" message is sent and its message ID is
// known (PRD §9's read/write pattern table).
func (s *IssueThreadStore) Insert(ctx context.Context, projectID, issueIID int, rootMessageID int64, isConfidential bool) error {
	now := time.Now().UTC().Format(time.RFC3339)

	confidential := 0
	if isConfidential {
		confidential = 1
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO issue_threads (
			gitlab_project_id, gitlab_issue_iid, telegram_root_message_id,
			is_confidential, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (gitlab_project_id, gitlab_issue_iid) DO UPDATE SET
			telegram_root_message_id = excluded.telegram_root_message_id,
			is_confidential          = excluded.is_confidential,
			updated_at               = excluded.updated_at
	`, projectID, issueIID, rootMessageID, confidential, now, now)
	if err != nil {
		return fmt.Errorf("insert issue thread (project=%d, iid=%d): %w", projectID, issueIID, err)
	}

	return nil
}

// GetRootMessageID looks up the Telegram message ID to reply to for a given
// GitLab issue. Returns ErrNotFound if the bot never recorded an "open"
// event for it (PRD §13.3: the caller should then send standalone and log a
// warning, not fail the request).
func (s *IssueThreadStore) GetRootMessageID(ctx context.Context, projectID, issueIID int) (int64, error) {
	var rootMessageID int64

	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_root_message_id
		FROM issue_threads
		WHERE gitlab_project_id = ? AND gitlab_issue_iid = ?
	`, projectID, issueIID).Scan(&rootMessageID)

	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get issue thread (project=%d, iid=%d): %w", projectID, issueIID, err)
	}

	return rootMessageID, nil
}
