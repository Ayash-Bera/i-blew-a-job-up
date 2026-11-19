package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"firstrole/internal/api"
	"firstrole/internal/config"
	"firstrole/internal/db"
	"firstrole/internal/ingest"
	"firstrole/internal/sources"
	"firstrole/internal/tagging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var store db.Store
	if c.DatabaseURL != "" {
		startup, cancel := context.WithTimeout(ctx, 60*time.Second)
		p, err := db.Open(startup, c.DatabaseURL, c.MigrationDatabaseURL)
		cancel()
		if err != nil {
			return err
		}
		defer p.Pool.Close()
		store = p
	} else if c.SnapshotPath != "" {
		store, err = db.OpenSnapshot(c.SnapshotPath)
		if err != nil {
			return err
		}
	} else {
		return errors.New("configure DATABASE_URL and MIGRATION_DATABASE_URL, or explicitly set SNAPSHOT_PATH for a read-only dataset")
	}
	definitions, err := sources.Registry(c.SourceRegistry)
	if err != nil {
		return err
	}
	engine := &ingest.Engine{Store: store, Fetch: sources.Fetch, RegistryPath: c.SourceRegistry, Definitions: definitions, Tagger: &tagging.Client{URL: c.ModelURL, Model: c.Model, Key: c.ModelKey, Provider: c.ModelProvider, InputRate: c.InputRate, OutputRate: c.OutputRate, CachedInputRate: c.CachedInputRate, MinInterval: c.ModelMinInterval}, MaxTags: c.MaxTagPosts, Concurrency: c.TagConcurrency, Timeout: c.RefreshTimeout}
	engine.TagCacheRetention = c.TagCacheRetention
	handler := (&api.Server{Store: store, Engine: engine, Origin: c.AllowedOrigin, CronSecret: c.CronSecret}).Router()
	server := &http.Server{Addr: ":" + c.Port, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: c.RefreshTimeout + 15*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	result := make(chan error, 1)
	go func() { slog.Info("server listening", "port", c.Port); result <- server.ListenAndServe() }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}
