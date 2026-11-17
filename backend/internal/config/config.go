package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port, DatabaseURL, MigrationDatabaseURL, SnapshotPath, SourceRegistry string
	CronSecret, AllowedOrigin, ModelURL, Model, ModelKey                  string
	InputRate, OutputRate                                                 *float64
	RefreshTimeout                                                        time.Duration
	MaxTagPosts, TagConcurrency                                           int
}

func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("load environment: %w", err)
	}
	c := Config{Port: os.Getenv("PORT"), DatabaseURL: os.Getenv("DATABASE_URL"), MigrationDatabaseURL: os.Getenv("MIGRATION_DATABASE_URL"), SnapshotPath: os.Getenv("SNAPSHOT_PATH"), SourceRegistry: os.Getenv("SOURCE_REGISTRY"), CronSecret: os.Getenv("CRON_SECRET"), AllowedOrigin: os.Getenv("ALLOWED_ORIGIN"), ModelURL: os.Getenv("MODEL_URL"), Model: os.Getenv("MODEL_NAME"), ModelKey: os.Getenv("MODEL_API_KEY")}
	if c.Port == "" {
		c.Port = "8080"
	}
	if c.ModelURL != "" {
		u, err := url.Parse(c.ModelURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return c, errors.New("MODEL_URL must be an HTTPS endpoint without embedded credentials")
		}
	}
	var err error
	if c.InputRate, err = rate(os.Getenv("MODEL_INPUT_USD_PER_MILLION")); err != nil {
		return c, fmt.Errorf("input rate: %w", err)
	}
	if c.OutputRate, err = rate(os.Getenv("MODEL_OUTPUT_USD_PER_MILLION")); err != nil {
		return c, fmt.Errorf("output rate: %w", err)
	}
	seconds, err := number(os.Getenv("REFRESH_TIMEOUT_SECONDS"), 240, 30, 780)
	if err != nil {
		return c, err
	}
	c.RefreshTimeout = time.Duration(seconds) * time.Second
	if c.MaxTagPosts, err = number(os.Getenv("MAX_TAG_POSTS"), 500, 0, 5000); err != nil {
		return c, err
	}
	if c.TagConcurrency, err = number(os.Getenv("TAG_CONCURRENCY"), 4, 1, 8); err != nil {
		return c, err
	}
	if c.DatabaseURL != "" && c.MigrationDatabaseURL == "" {
		return c, errors.New("MIGRATION_DATABASE_URL is required with DATABASE_URL; use a session connection for migrations")
	}
	if c.MigrationDatabaseURL != "" {
		u, err := url.Parse(c.MigrationDatabaseURL)
		if err != nil || u.Port() == "6543" {
			return c, errors.New("migration connection must use a direct or session endpoint, not transaction port 6543")
		}
	}
	if strings.Contains(c.AllowedOrigin, ",") {
		return c, errors.New("ALLOWED_ORIGIN must be one exact origin")
	}
	return c, nil
}

func rate(s string) (*float64, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errors.New("expected a nonnegative decimal")
	}
	return &v, nil
}
func number(s string, fallback, min, max int) (int, error) {
	if s == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < min || n > max {
		return 0, fmt.Errorf("expected integer between %d and %d", min, max)
	}
	return n, nil
}
