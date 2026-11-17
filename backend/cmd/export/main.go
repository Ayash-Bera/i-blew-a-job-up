package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"firstrole/internal/api"
	"firstrole/internal/config"
	"firstrole/internal/db"
	"firstrole/internal/tagging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	format := flag.String("format", "csv", "csv, state, usage, prompt, or schema")
	flag.Parse()
	if *format == "prompt" {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"version": tagging.PromptVersion, "prompt": tagging.Prompt})
	}
	if *format == "schema" {
		return json.NewEncoder(os.Stdout).Encode(tagging.Schema())
	}
	c, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var store db.Store
	if c.DatabaseURL != "" {
		p, err := db.Open(ctx, c.DatabaseURL, c.MigrationDatabaseURL)
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
		return errors.New("configure a database or snapshot")
	}
	if *format == "usage" {
		usage, err := store.Usage(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(usage)
	}
	state, err := store.Load(ctx, *format == "state")
	if err != nil {
		return err
	}
	switch *format {
	case "state":
		return json.NewEncoder(os.Stdout).Encode(state)
	case "csv":
		return api.WriteCSV(csv.NewWriter(os.Stdout), api.PublicState(state, time.Now()).Jobs)
	default:
		return errors.New("format must be csv, state, usage, prompt, or schema")
	}
}
