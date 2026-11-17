package db

import (
	"context"
	"testing"
	"time"
)

func TestEmbeddedSnapshotRequiresExplicitPath(t *testing.T) {
	if _, err := OpenSnapshot(""); err == nil {
		t.Fatal("empty path silently opened embedded data")
	}
	store, err := OpenSnapshot("embedded")
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if state.Mode != "snapshot_read_only" {
		t.Fatal(state.Mode)
	}
	if len(state.Observations) != 0 {
		t.Fatal("raw source descriptions bundled into public snapshot")
	}
	seen := map[string]bool{}
	for _, job := range state.Jobs {
		if job.ID == "" || seen[job.ID] {
			t.Fatal("empty or duplicate bundled job identifier", job.ID)
		}
		seen[job.ID] = true
		if job.SourceURL == "" || job.TaggingMethod != "pending" {
			t.Fatal("unexpected snapshot record", job.ID)
		}
	}
	if _, err = store.Acquire(context.Background(), "refresh", time.Minute); err != ErrReadOnly {
		t.Fatal("snapshot allowed refresh", err)
	}
}
