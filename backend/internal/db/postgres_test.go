package db

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"firstrole/internal/models"
	"github.com/jackc/pgx/v5"
)

func TestPostgresIntegration(t *testing.T) {
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL is required for the isolated database integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	name := fmt.Sprintf("firstrole_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+quoted+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	}()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	p, err := Open(ctx, u.String(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Pool.Close()
	if err = Migrate(ctx, u.String()); err != nil {
		t.Fatal("second migration", err)
	}
	if ok, err := p.Acquire(ctx, "owner", time.Minute); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := p.Acquire(ctx, "second", time.Minute); err != nil || ok {
		t.Fatal("parallel lease", ok, err)
	}
	if err = p.Renew(ctx, "second", time.Minute); err != ErrLeaseLost {
		t.Fatal(err)
	}
	state := Empty()
	state.Jobs = []models.Job{{ID: "j1", Title: "Real escaping test ' ;", Cities: []string{}}}
	state.Observations["raw1"] = models.Observation{Raw: models.RawPosting{Description: "Unicode example: Bengaluru", PostingID: "raw1"}, Eligible: true}
	state.Sources = []models.SourceView{{ID: "source", Name: "Employer"}}
	if err = p.Save(ctx, state, "owner"); err != nil {
		t.Fatal(err)
	}
	loaded, err := p.Load(ctx, true)
	if err != nil || len(loaded.Jobs) != 1 || loaded.Jobs[0].Title != state.Jobs[0].Title || len(loaded.Observations) != 1 {
		t.Fatal(loaded, err)
	}
	if err = p.Save(ctx, state, "second"); err != ErrLeaseLost {
		t.Fatal("unowned save accepted", err)
	}
	now := time.Now().UTC()
	r := models.Run{ID: "r1", StartedAt: now, Status: "running"}
	if err = p.PutRun(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Status = "success"
	r.FinishedAt = &now
	if err = p.PutRun(ctx, r); err != nil {
		t.Fatal(err)
	}
	tag := models.TagRecord{Key: "cache", Model: "test", CreatedAt: now}
	if err = p.PutTag(ctx, tag); err != nil {
		t.Fatal(err)
	}
	if got, err := p.GetTag(ctx, "cache"); err != nil || got.Model != "test" {
		t.Fatal(got, err)
	}
	cost := 0.02
	usage := models.Usage{ID: "u1", RunID: "r1", CreatedAt: now, CostUSD: &cost}
	if err = p.PutUsage(ctx, usage); err != nil {
		t.Fatal(err)
	}
	if err = p.PutUsage(ctx, usage); err != nil {
		t.Fatal(err)
	}
	rows, err := p.Usage(ctx)
	if err != nil || len(rows) != 1 || *rows[0].CostUSD != cost {
		t.Fatal(rows, err)
	}
	var allRLS bool
	if err = p.Pool.QueryRow(ctx, "SELECT bool_and(relrowsecurity) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='firstrole' AND c.relkind='r'").Scan(&allRLS); err != nil || !allRLS {
		t.Fatal("RLS disabled", err)
	}
	var publicPrivileges int
	if err = p.Pool.QueryRow(ctx, "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace CROSS JOIN LATERAL aclexplode(c.relacl) a WHERE n.nspname='firstrole' AND a.grantee=0").Scan(&publicPrivileges); err != nil || publicPrivileges != 0 {
		t.Fatal("public access", publicPrivileges, err)
	}
	if err = p.Release(ctx, "owner"); err != nil {
		t.Fatal(err)
	}
	if ok, err := p.Acquire(ctx, "second", time.Minute); err != nil || !ok {
		t.Fatal(ok, err)
	}
	b, err := json.Marshal(loaded)
	if err != nil || len(b) == 0 {
		t.Fatal(err)
	}
}
