package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"firstrole/internal/models"
)

func testPostgresTagCacheRetention(t *testing.T, ctx context.Context, p *Postgres) {
	t.Helper()
	for _, name := range []string{"000002_tag_cache_retention.down.sql", "000002_tag_cache_retention.up.sql"} {
		if name == "000002_tag_cache_retention.up.sql" {
			legacy := models.TagRecord{Key: "legacy", Model: "legacy", CreatedAt: time.Now().Add(-365 * 24 * time.Hour)}
			b, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = p.Pool.Exec(ctx, "INSERT INTO firstrole.tag_cache(key,payload) VALUES($1,$2)", legacy.Key, string(b)); err != nil {
				t.Fatal(err)
			}
		}
		migration, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.Pool.Exec(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	var legacyUsed time.Time
	if err := p.Pool.QueryRow(ctx, "SELECT last_used_at FROM firstrole.tag_cache WHERE key='legacy'").Scan(&legacyUsed); err != nil || legacyUsed.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("migration did not grant grace period", legacyUsed, err)
	}
	cutoff := time.Now().UTC().Add(-90 * 24 * time.Hour).Truncate(time.Microsecond)
	for _, key := range []string{"expired", "current", "cohort", "boundary", "touch", "conflict", "locked"} {
		record := models.TagRecord{Key: key, Model: "original", CreatedAt: cutoff.Add(-time.Hour)}
		if err := p.PutTag(ctx, record); err != nil {
			t.Fatal(err)
		}
		used := cutoff.Add(-time.Hour)
		if key == "boundary" {
			used = cutoff
		}
		if _, err := p.Pool.Exec(ctx, "UPDATE firstrole.tag_cache SET last_used_at=$2 WHERE key=$1", key, used); err != nil {
			t.Fatal(err)
		}
	}
	beforeTouch := time.Now()
	if _, err := p.GetTag(ctx, "touch"); err != nil {
		t.Fatal(err)
	}
	if err := p.PutTag(ctx, models.TagRecord{Key: "conflict", Model: "replacement"}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"touch", "conflict"} {
		var payload []byte
		var used time.Time
		if err := p.Pool.QueryRow(ctx, "SELECT payload,last_used_at FROM firstrole.tag_cache WHERE key=$1", key).Scan(&payload, &used); err != nil {
			t.Fatal(err)
		}
		var record models.TagRecord
		if err := json.Unmarshal(payload, &record); err != nil || record.Model != "original" || !record.CreatedAt.Equal(cutoff.Add(-time.Hour)) || used.Before(beforeTouch) {
			t.Fatal("touch changed provenance or failed to update last use", record, used, err)
		}
	}
	s, err := p.Load(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	finished := time.Now()
	s.InitialCohort = &models.InitialCohort{CompletedAt: &finished, Versions: map[string]string{"historical": "cohort"}}
	if err = p.Save(ctx, s, "owner"); err != nil {
		t.Fatal(err)
	}
	if err = p.PutUsage(ctx, models.Usage{ID: "expired-attempt", RunID: "r1", ContentKey: "expired", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	stateBefore, _ := json.Marshal(s)
	usageBefore, err := p.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	usageJSON, _ := json.Marshal(usageBefore)
	if n, err := p.PruneTags(ctx, "other", nil, cutoff); !errors.Is(err, ErrLeaseLost) || n != 0 {
		t.Fatal("unowned pruning allowed", n, err)
	}
	if _, err = p.Pool.Exec(ctx, "UPDATE firstrole.leases SET expires_at=now()-interval '1 second' WHERE name='refresh'"); err != nil {
		t.Fatal(err)
	}
	if n, err := p.PruneTags(ctx, "owner", nil, cutoff); !errors.Is(err, ErrLeaseLost) || n != 0 {
		t.Fatal("expired lease allowed pruning", n, err)
	}
	if ok, err := p.Acquire(ctx, "owner", time.Minute); err != nil || !ok {
		t.Fatal(ok, err)
	}
	locked, err := p.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Rollback(context.Background())
	if _, err = locked.Exec(ctx, "UPDATE firstrole.tag_cache SET last_used_at=statement_timestamp() WHERE key='locked'"); err != nil {
		t.Fatal(err)
	}
	if n, err := p.PruneTags(ctx, "owner", []string{"current"}, cutoff); err != nil || n != 1 {
		t.Fatal("expected only expired entry removed", n, err)
	}
	if err = locked.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"current", "cohort", "boundary", "touch", "conflict", "locked", "legacy"} {
		var exists bool
		if err = p.Pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM firstrole.tag_cache WHERE key=$1)", key).Scan(&exists); err != nil || !exists {
			t.Fatal("protected or fresh entry removed", key, err)
		}
	}
	if got, err := p.GetTag(ctx, "expired"); err != nil || got != nil {
		t.Fatal("expired cache remains", got, err)
	}
	after, err := p.Load(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	stateAfter, _ := json.Marshal(after)
	usageAfter, err := p.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, _ := json.Marshal(usageAfter)
	if string(stateBefore) != string(stateAfter) || string(usageJSON) != string(afterJSON) {
		t.Fatal("cleanup changed historical evidence or published state")
	}
	if _, err = p.Pool.Exec(ctx, "INSERT INTO firstrole.tag_cache(key,payload,last_used_at) SELECT 'bulk-'||n, jsonb_build_object('key','bulk-'||n), $1 FROM generate_series(1,$2::integer) AS n", cutoff.Add(-time.Hour), TagCachePruneBatchSize+1); err != nil {
		t.Fatal(err)
	}
	if n, err := p.PruneTags(ctx, "owner", []string{"current"}, cutoff); err != nil || n != TagCachePruneBatchSize {
		t.Fatal("pruning batch not bounded", n, err)
	}
	if n, err := p.PruneTags(ctx, "owner", []string{"current"}, cutoff); err != nil || n != 1 {
		t.Fatal("remaining batch not removed", n, err)
	}
}
