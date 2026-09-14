package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestIssueThreadStore_InsertAndGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bot.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	s := NewIssueThreadStore(db)
	ctx := context.Background()

	if _, err := s.GetRootMessageID(ctx, 42, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetRootMessageID before insert: got err %v, want ErrNotFound", err)
	}

	if err := s.Insert(ctx, 42, 1, 1001, true); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := s.GetRootMessageID(ctx, 42, 1)
	if err != nil {
		t.Fatalf("GetRootMessageID after insert: %v", err)
	}
	if got != 1001 {
		t.Errorf("GetRootMessageID = %d, want 1001", got)
	}

	// Insert again for the same (project, iid) should upsert, not error.
	if err := s.Insert(ctx, 42, 1, 2002, true); err != nil {
		t.Fatalf("Insert (update): %v", err)
	}
	got, err = s.GetRootMessageID(ctx, 42, 1)
	if err != nil {
		t.Fatalf("GetRootMessageID after update: %v", err)
	}
	if got != 2002 {
		t.Errorf("GetRootMessageID after update = %d, want 2002", got)
	}
}
