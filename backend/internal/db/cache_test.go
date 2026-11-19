package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"firstrole/internal/models"
)

func TestTagCacheRetention(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if ok, err := m.Acquire(ctx, "owner", time.Minute); err != nil || !ok {
		t.Fatal(ok, err)
	}
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour)
	for _, tc := range []struct {
		key  string
		used time.Time
	}{
		{"expired", cutoff.Add(-time.Second)},
		{"current", cutoff.Add(-time.Hour)},
		{"cohort", cutoff.Add(-time.Hour)},
		{"boundary", cutoff},
		{"recent", cutoff.Add(time.Second)},
	} {
		if err := m.PutTag(ctx, models.TagRecord{Key: tc.key, CreatedAt: cutoff.Add(-time.Hour)}); err != nil {
			t.Fatal(err)
		}
		m.tagUsed[tc.key] = tc.used
	}
	finished := time.Now().UTC()
	m.State.InitialCohort = &models.InitialCohort{CompletedAt: &finished, Versions: map[string]string{"job": "cohort"}}
	m.State.Jobs = []models.Job{{ID: "published"}}
	m.State.Observations["source"] = models.Observation{Eligible: true}
	m.State.Runs = []models.Run{{ID: "previous"}}
	usage := models.Usage{ID: "attempt", ContentKey: "expired"}
	if err := m.PutUsage(ctx, usage); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(m.State)
	n, err := m.PruneTags(ctx, "owner", []string{"current", "current"}, cutoff)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	for _, key := range []string{"current", "cohort", "boundary", "recent"} {
		if _, exists := m.tags[key]; !exists {
			t.Fatalf("protected or recent entry removed: %s", key)
		}
	}
	if _, exists := m.tags["expired"]; exists {
		t.Fatal("unused expired entry retained")
	}
	after, _ := json.Marshal(m.State)
	if string(before) != string(after) || !reflect.DeepEqual(m.usage, []models.Usage{usage}) {
		t.Fatal("pruning changed retained evidence")
	}
}

func TestTagCacheTouchPreservesPayload(t *testing.T) {
	for _, method := range []string{"get", "put"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			m := NewMemory()
			original := models.TagRecord{Key: "key", Model: "original", PromptVersion: "v1", CreatedAt: time.Now().Add(-365 * 24 * time.Hour)}
			if err := m.PutTag(ctx, original); err != nil {
				t.Fatal(err)
			}
			m.tagUsed["key"] = time.Now().Add(-100 * 24 * time.Hour)
			before := time.Now()
			if method == "get" {
				if _, err := m.GetTag(ctx, "key"); err != nil {
					t.Fatal(err)
				}
			} else if err := m.PutTag(ctx, models.TagRecord{Key: "key", Model: "replacement"}); err != nil {
				t.Fatal(err)
			}
			if m.tagUsed["key"].Before(before) || !reflect.DeepEqual(m.tags["key"], original) {
				t.Fatal("cache hit did not touch use time or rewrote provenance")
			}
			if ok, err := m.Acquire(ctx, "owner", time.Minute); err != nil || !ok {
				t.Fatal(ok, err)
			}
			if n, err := m.PruneTags(ctx, "owner", nil, before.Add(-90*24*time.Hour)); err != nil || n != 0 {
				t.Fatal("recently used tag was removed", n, err)
			}
			future := time.Now().Add(time.Hour)
			m.tagUsed["key"] = future
			if _, err := m.GetTag(ctx, "key"); err != nil || !m.tagUsed["key"].Equal(future) {
				t.Fatal("last-used time moved backwards", err)
			}
			if got, err := m.GetTag(ctx, "missing"); err != nil || got != nil || len(m.tags) != 1 {
				t.Fatal("missing lookup created a cache entry", got, err)
			}
		})
	}
}

func TestTagCachePruningGuards(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		readOnly, expired, cancelled bool
		owner                        string
		want                         error
	}{
		{name: "wrong owner", owner: "other", want: ErrLeaseLost},
		{name: "expired lease", owner: "owner", expired: true, want: ErrLeaseLost},
		{name: "snapshot", owner: "owner", readOnly: true, want: ErrReadOnly},
		{name: "cancelled", owner: "owner", cancelled: true, want: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMemory()
			m.owner, m.until = "owner", time.Now().Add(time.Minute)
			if tc.expired {
				m.until = time.Now().Add(-time.Second)
			}
			m.tags["old"] = models.TagRecord{Key: "old"}
			m.tagUsed["old"] = time.Now().Add(-100 * 24 * time.Hour)
			m.readOnly = tc.readOnly
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			n, err := m.PruneTags(ctx, tc.owner, nil, time.Now().Add(-90*24*time.Hour))
			if n != 0 || !errors.Is(err, tc.want) || len(m.tags) != 1 {
				t.Fatal(n, err, len(m.tags))
			}
		})
	}
}

func TestTagCachePruningBounded(t *testing.T) {
	m := NewMemory()
	m.owner, m.until = "owner", time.Now().Add(time.Minute)
	for i := 0; i < TagCachePruneBatchSize+1; i++ {
		key := fmt.Sprint(i)
		m.tags[key] = models.TagRecord{Key: key}
		m.tagUsed[key] = time.Now().Add(-100 * 24 * time.Hour)
	}
	n, err := m.PruneTags(context.Background(), "owner", nil, time.Now().Add(-90*24*time.Hour))
	if err != nil || n != TagCachePruneBatchSize || len(m.tags) != 1 {
		t.Fatal(n, err, len(m.tags))
	}
}
