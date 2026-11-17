CREATE TABLE firstrole.jobs (id text PRIMARY KEY, payload jsonb NOT NULL);
CREATE TABLE firstrole.observations (id text PRIMARY KEY, payload jsonb NOT NULL);
CREATE TABLE firstrole.sources (id text PRIMARY KEY, payload jsonb NOT NULL);
CREATE TABLE firstrole.runs (id text PRIMARY KEY, started_at timestamptz NOT NULL, payload jsonb NOT NULL);
CREATE INDEX runs_started_idx ON firstrole.runs(started_at DESC);
CREATE TABLE firstrole.tag_cache (key text PRIMARY KEY, payload jsonb NOT NULL);
CREATE TABLE firstrole.usage (id text PRIMARY KEY, run_id text NOT NULL, created_at timestamptz NOT NULL, payload jsonb NOT NULL);
CREATE INDEX usage_run_idx ON firstrole.usage(run_id);
CREATE TABLE firstrole.leases (name text PRIMARY KEY, owner text NOT NULL, expires_at timestamptz NOT NULL);
CREATE TABLE firstrole.settings (key text PRIMARY KEY, payload jsonb NOT NULL);
DO $$
DECLARE item text;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'firstrole_app') THEN
    CREATE ROLE firstrole_app NOLOGIN;
  END IF;
  GRANT USAGE ON SCHEMA firstrole TO firstrole_app;
  FOR item IN SELECT tablename FROM pg_tables WHERE schemaname = 'firstrole' LOOP
    EXECUTE format('ALTER TABLE firstrole.%I ENABLE ROW LEVEL SECURITY', item);
    EXECUTE format('REVOKE ALL ON firstrole.%I FROM PUBLIC', item);
    IF item <> 'schema_migrations' THEN
      EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON firstrole.%I TO firstrole_app', item);
      EXECUTE format('CREATE POLICY backend_access ON firstrole.%I TO firstrole_app USING (true) WITH CHECK (true)', item);
    END IF;
  END LOOP;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'anon') THEN
    REVOKE ALL ON SCHEMA firstrole FROM anon;
    REVOKE ALL ON ALL TABLES IN SCHEMA firstrole FROM anon;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
    REVOKE ALL ON SCHEMA firstrole FROM authenticated;
    REVOKE ALL ON ALL TABLES IN SCHEMA firstrole FROM authenticated;
  END IF;
END $$;
REVOKE ALL ON SCHEMA firstrole FROM PUBLIC;
ALTER DEFAULT PRIVILEGES IN SCHEMA firstrole REVOKE ALL ON TABLES FROM PUBLIC;
