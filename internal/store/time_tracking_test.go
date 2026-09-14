package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestTimeTrackingStore_GetAndSet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "bot.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	s := NewTimeTrackingStore(db)
	ctx := context.Background()

	if _, err := s.GetLastTotalSecs(ctx, 42, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetLastTotalSecs before set: got err %v, want ErrNotFound", err)
	}

	if err := s.SetLastTotalSecs(ctx, 42, 1, 3600); err != nil {
		t.Fatalf("SetLastTotalSecs: %v", err)
	}

	got, err := s.GetLastTotalSecs(ctx, 42, 1)
	if err != nil {
		t.Fatalf("GetLastTotalSecs after set: %v", err)
	}
	if got != 3600 {
		t.Errorf("GetLastTotalSecs = %d, want 3600", got)
	}

	// Setting again for the same (project, iid) should upsert, not error.
	if err := s.SetLastTotalSecs(ctx, 42, 1, 5400); err != nil {
		t.Fatalf("SetLastTotalSecs (update): %v", err)
	}
	got, err = s.GetLastTotalSecs(ctx, 42, 1)
	if err != nil {
		t.Fatalf("GetLastTotalSecs after update: %v", err)
	}
	if got != 5400 {
		t.Errorf("GetLastTotalSecs after update = %d, want 5400", got)
	}
}
