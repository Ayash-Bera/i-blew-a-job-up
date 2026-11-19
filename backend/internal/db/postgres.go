package db

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"firstrole/internal/models"
	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Postgres struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL, migrationURL string) (*Postgres, error) {
	if err := Migrate(ctx, migrationURL); err != nil {
		return nil, err
	}
	c, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("parse database connection configuration")
	}
	c.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	c.MaxConns = 4
	c.MinConns = 0
	c.MaxConnIdleTime = time.Minute
	c.ConnConfig.ConnectTimeout = 10 * time.Second
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return &Postgres{Pool: p}, nil
}
func Migrate(ctx context.Context, url string) error {
	d, err := sql.Open("pgx", url)
	if err != nil {
		return errors.New("parse migration connection configuration")
	}
	defer d.Close()
	d.SetMaxOpenConns(2)
	d.SetConnMaxLifetime(time.Minute)
	if _, err = d.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS firstrole; REVOKE ALL ON SCHEMA firstrole FROM PUBLIC"); err != nil {
		return fmt.Errorf("create private schema: %w", err)
	}
	driver, err := migratepgx.WithInstance(d, &migratepgx.Config{SchemaName: "firstrole", MigrationsTable: "schema_migrations", StatementTimeout: 30 * time.Second})
	if err != nil {
		return fmt.Errorf("initialize migration driver: %w", err)
	}
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("initialize migrations: %w", err)
	}
	m.LockTimeout = 30 * time.Second
	err = m.Up()
	sourceErr, dbErr := m.Close()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if sourceErr != nil {
		return sourceErr
	}
	return dbErr
}

func readRows[T any](ctx context.Context, p *pgxpool.Pool, query string) ([]T, error) {
	rows, err := p.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var v T
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (p *Postgres) Load(ctx context.Context, full bool) (models.State, error) {
	s := Empty()
	var err error
	if s.Jobs, err = readRows[models.Job](ctx, p.Pool, "SELECT payload FROM firstrole.jobs ORDER BY id"); err != nil {
		return s, fmt.Errorf("load jobs: %w", err)
	}
	if s.Sources, err = readRows[models.SourceView](ctx, p.Pool, "SELECT payload FROM firstrole.sources ORDER BY id"); err != nil {
		return s, err
	}
	if s.Runs, err = readRows[models.Run](ctx, p.Pool, "SELECT payload FROM firstrole.runs ORDER BY started_at DESC LIMIT 100"); err != nil {
		return s, err
	}
	if full {
		rows, e := p.Pool.Query(ctx, "SELECT id,payload FROM firstrole.observations")
		if e != nil {
			return s, e
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var b []byte
			if e = rows.Scan(&id, &b); e != nil {
				return s, e
			}
			var o models.Observation
			if e = json.Unmarshal(b, &o); e != nil {
				return s, e
			}
			s.Observations[id] = o
		}
		if e = rows.Err(); e != nil {
			return s, e
		}
	}
	var payload []byte
	err = p.Pool.QueryRow(ctx, "SELECT payload FROM firstrole.settings WHERE key='summary'").Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var meta struct {
		Evaluation    models.Evaluation     `json:"evaluation"`
		Costs         models.Costs          `json:"costs"`
		InitialCohort *models.InitialCohort `json:"initial_cohort"`
	}
	if err = json.Unmarshal(payload, &meta); err != nil {
		return s, err
	}
	s.Evaluation = meta.Evaluation
	s.Costs = meta.Costs
	s.InitialCohort = meta.InitialCohort
	return s, nil
}
func (p *Postgres) Acquire(ctx context.Context, owner string, ttl time.Duration) (bool, error) {
	tag, err := p.Pool.Exec(ctx, "INSERT INTO firstrole.leases(name,owner,expires_at) VALUES ('refresh',$1,now()+$2*interval '1 second') ON CONFLICT(name) DO UPDATE SET owner=excluded.owner,expires_at=excluded.expires_at WHERE firstrole.leases.expires_at < now()", owner, ttl.Seconds())
	return tag.RowsAffected() == 1, err
}
func (p *Postgres) Renew(ctx context.Context, owner string, ttl time.Duration) error {
	tag, err := p.Pool.Exec(ctx, "UPDATE firstrole.leases SET expires_at=now()+$2*interval '1 second' WHERE name='refresh' AND owner=$1 AND expires_at>now()", owner, ttl.Seconds())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (p *Postgres) Release(ctx context.Context, owner string) error {
	_, err := p.Pool.Exec(ctx, "DELETE FROM firstrole.leases WHERE name='refresh' AND owner=$1", owner)
	return err
}
func (p *Postgres) Save(ctx context.Context, s models.State, owner string) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var actual string
	if err = tx.QueryRow(ctx, "SELECT owner FROM firstrole.leases WHERE name='refresh' AND expires_at>now() FOR UPDATE").Scan(&actual); err != nil || actual != owner {
		return ErrLeaseLost
	}
	if _, err = tx.Exec(ctx, "DELETE FROM firstrole.jobs; DELETE FROM firstrole.sources"); err != nil {
		return err
	}

	type item struct {
		ID      string `json:"id"`
		Payload any    `json:"payload"`
	}
	put := func(table string, items []item) error {
		if len(items) == 0 {
			return nil
		}
		b, e := json.Marshal(items)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "INSERT INTO firstrole."+table+"(id,payload) SELECT id,payload FROM jsonb_to_recordset($1::jsonb) AS x(id text,payload jsonb) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", string(b))
		return e
	}
	jobs := []item{}
	for _, j := range s.Jobs {
		jobs = append(jobs, item{j.ID, j})
	}
	if err = put("jobs", jobs); err != nil {
		return err
	}
	sources := []item{}
	for _, source := range s.Sources {
		sources = append(sources, item{source.ID, source})
	}
	if err = put("sources", sources); err != nil {
		return err
	}
	observations := []item{}
	for id, o := range s.Observations {
		observations = append(observations, item{id, o})
	}
	if err = put("observations", observations); err != nil {
		return err
	}
	b, err := json.Marshal(map[string]any{"evaluation": s.Evaluation, "costs": s.Costs, "initial_cohort": s.InitialCohort})
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO firstrole.settings(key,payload) VALUES('summary',$1) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload", string(b)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (p *Postgres) PutRun(ctx context.Context, r models.Run) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = p.Pool.Exec(ctx, "INSERT INTO firstrole.runs(id,started_at,payload) VALUES($1,$2,$3) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", r.ID, r.StartedAt, string(b))
	return err
}
func (p *Postgres) GetTag(ctx context.Context, key string) (*models.TagRecord, error) {
	var b []byte
	err := p.Pool.QueryRow(ctx, "UPDATE firstrole.tag_cache SET last_used_at=GREATEST(last_used_at,statement_timestamp()) WHERE key=$1 RETURNING payload", key).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t models.TagRecord
	err = json.Unmarshal(b, &t)
	return &t, err
}
func (p *Postgres) PutTag(ctx context.Context, t models.TagRecord) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	_, err = p.Pool.Exec(ctx, "INSERT INTO firstrole.tag_cache(key,payload) VALUES($1,$2) ON CONFLICT(key) DO UPDATE SET last_used_at=GREATEST(firstrole.tag_cache.last_used_at,statement_timestamp())", t.Key, string(b))
	return err
}

func (p *Postgres) PruneTags(ctx context.Context, owner string, protected []string, cutoff time.Time) (int, error) {
	if cutoff.IsZero() || cutoff.After(time.Now()) {
		return 0, errors.New("cache cutoff must be a past timestamp")
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin cache pruning: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	var actual string
	err = tx.QueryRow(ctx, "SELECT owner FROM firstrole.leases WHERE name='refresh' AND expires_at>now() FOR UPDATE").Scan(&actual)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && actual != owner {
		return 0, ErrLeaseLost
	}
	if err != nil {
		return 0, fmt.Errorf("check cache pruning lease: %w", err)
	}
	var versions []byte
	err = tx.QueryRow(ctx, "SELECT payload #> '{initial_cohort,versions}' FROM firstrole.settings WHERE key='summary'").Scan(&versions)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("load protected cache cohort: %w", err)
	}
	cohort := models.InitialCohort{}
	if len(versions) > 0 {
		if err := json.Unmarshal(versions, &cohort.Versions); err != nil {
			return 0, fmt.Errorf("decode protected cache cohort: %w", err)
		}
	}
	keep := cacheProtection(protected, &cohort)
	keys := make([]string, 0, len(keep))
	for key := range keep {
		keys = append(keys, key)
	}
	deleted, err := tx.Exec(ctx, `WITH expired AS (
		SELECT key FROM firstrole.tag_cache
		WHERE last_used_at < $1 AND key <> ALL($2::text[])
		ORDER BY last_used_at, key LIMIT $3 FOR UPDATE SKIP LOCKED
	)
	DELETE FROM firstrole.tag_cache AS cache USING expired
	WHERE cache.key=expired.key AND cache.last_used_at < $1`, cutoff, keys, TagCachePruneBatchSize)
	if err != nil {
		return 0, fmt.Errorf("delete unused tags: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit cache pruning: %w", err)
	}
	return int(deleted.RowsAffected()), nil
}
func (p *Postgres) PutUsage(ctx context.Context, u models.Usage) error {
	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = p.Pool.Exec(ctx, "INSERT INTO firstrole.usage(id,run_id,created_at,payload) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", u.ID, u.RunID, u.CreatedAt, string(b))
	return err
}
func (p *Postgres) Usage(ctx context.Context) ([]models.Usage, error) {
	return readRows[models.Usage](ctx, p.Pool, "SELECT payload FROM firstrole.usage ORDER BY created_at")
}
