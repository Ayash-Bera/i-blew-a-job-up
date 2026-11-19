ALTER TABLE firstrole.tag_cache
    ADD COLUMN last_used_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX tag_cache_last_used_idx ON firstrole.tag_cache(last_used_at, key);
