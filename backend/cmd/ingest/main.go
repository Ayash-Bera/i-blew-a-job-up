package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"firstrole/internal/config"
	"firstrole/internal/db"
	"firstrole/internal/ingest"
	"firstrole/internal/models"
	"firstrole/internal/sources"
	"firstrole/internal/tagging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	input := flag.String("input", "", "Import collected SourceResult JSON instead of fetching feeds")
	sourceOutput := flag.String("source-results-out", "", "Save exact collected SourceResult JSON before normalization")
	snapshot := flag.String("snapshot-out", "", "Write an explicitly untagged read-only snapshot without a database")
	flag.Parse()
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	definitions, err := sources.Registry(c.SourceRegistry)
	if err != nil {
		return err
	}
	fetch := sources.Fetch
	if *input != "" {
		b, err := os.ReadFile(*input)
		if err != nil {
			return err
		}
		var results []models.SourceResult
		if err = json.Unmarshal(b, &results); err != nil {
			return fmt.Errorf("decode source results: %w", err)
		}
		fetch = func(context.Context, string) ([]models.SourceResult, error) { return results, nil }
	}
	if *sourceOutput != "" {
		collect := fetch
		fetch = func(ctx context.Context, path string) ([]models.SourceResult, error) {
			results, collectErr := collect(ctx, path)
			data, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				return results, errors.Join(collectErr, fmt.Errorf("encode source evidence: %w", err))
			}
			if err = os.WriteFile(*sourceOutput, data, 0600); err != nil {
				return results, errors.Join(collectErr, fmt.Errorf("save source evidence: %w", err))
			}
			return results, collectErr
		}
	}
	if *snapshot != "" {
		fetchCtx, cancel := context.WithTimeout(ctx, c.RefreshTimeout)
		defer cancel()
		results, err := fetch(fetchCtx, c.SourceRegistry)
		if err != nil {
			return err
		}
		state := ingest.Snapshot(results, definitions)
		b, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		if err = os.WriteFile(*snapshot, b, 0644); err != nil {
			return err
		}
		fmt.Printf("Wrote %d real eligible jobs; model tags pending; snapshot is read-only.\n", len(state.Jobs))
		return nil
	}
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL and MIGRATION_DATABASE_URL are required for durable ingestion")
	}
	startup, cancel := context.WithTimeout(ctx, 60*time.Second)
	store, err := db.Open(startup, c.DatabaseURL, c.MigrationDatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer store.Pool.Close()
	engine := &ingest.Engine{Store: store, Fetch: fetch, RegistryPath: c.SourceRegistry, Definitions: definitions, Tagger: &tagging.Client{URL: c.ModelURL, Model: c.Model, Key: c.ModelKey, InputRate: c.InputRate, OutputRate: c.OutputRate}, MaxTags: c.MaxTagPosts, Concurrency: c.TagConcurrency, Timeout: c.RefreshTimeout}
	run, err := engine.Run(ctx)
	b, encodeErr := json.MarshalIndent(run, "", "  ")
	if encodeErr != nil {
		return encodeErr
	}
	fmt.Println(string(b))
	return err
}
