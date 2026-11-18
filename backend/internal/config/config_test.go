package config

import (
	"testing"
	"time"
)

func TestRatesFinite(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf", "-1", "oops"} {
		if _, err := rate(value); err == nil {
			t.Fatalf("accepted %s", value)
		}
	}
	if v, err := rate(""); err != nil || v != nil {
		t.Fatal("missing rate must stay unknown")
	}
	if v, err := rate("0.075"); err != nil || *v != 0.075 {
		t.Fatal(v, err)
	}
}

func resetConfig(t *testing.T) {
	t.Helper()
	for _, key := range []string{"PORT", "DATABASE_URL", "MIGRATION_DATABASE_URL", "SNAPSHOT_PATH", "SOURCE_REGISTRY", "CRON_SECRET", "ALLOWED_ORIGIN", "MODEL_PROVIDER", "MODEL_URL", "MODEL_NAME", "MODEL_API_KEY", "MODEL_INPUT_USD_PER_MILLION", "MODEL_OUTPUT_USD_PER_MILLION", "MODEL_CACHED_INPUT_USD_PER_MILLION", "MODEL_MIN_INTERVAL_MS", "REFRESH_TIMEOUT_SECONDS", "MAX_TAG_POSTS", "TAG_CONCURRENCY"} {
		t.Setenv(key, "")
	}
}

func TestModelConfiguration(t *testing.T) {
	resetConfig(t)
	c, err := Load()
	if err != nil || c.ModelProvider != "compatible" || c.ModelMinInterval != 0 || c.CachedInputRate != nil {
		t.Fatal(c.ModelProvider, c.ModelMinInterval, err)
	}
	t.Setenv("MODEL_PROVIDER", "gemini")
	t.Setenv("MODEL_NAME", "gemini-3.5-flash-lite")
	t.Setenv("MODEL_URL", "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent")
	t.Setenv("MODEL_MIN_INTERVAL_MS", "12000")
	t.Setenv("MODEL_CACHED_INPUT_USD_PER_MILLION", "0.03")
	c, err = Load()
	if err != nil || c.ModelProvider != "gemini" || c.ModelMinInterval != 12*time.Second || c.CachedInputRate == nil || *c.CachedInputRate != 0.03 {
		t.Fatal(c.ModelProvider, c.ModelMinInterval, err)
	}
}

func TestModelConfigurationRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"MODEL_PROVIDER", "unknown"},
		{"MODEL_MIN_INTERVAL_MS", "-1"},
		{"MODEL_MIN_INTERVAL_MS", "3600001"},
		{"MODEL_MIN_INTERVAL_MS", "1.5"},
		{"MODEL_CACHED_INPUT_USD_PER_MILLION", "NaN"},
		{"MODEL_CACHED_INPUT_USD_PER_MILLION", "+Inf"},
		{"MODEL_CACHED_INPUT_USD_PER_MILLION", "-0.01"},
		{"MODEL_URL", "https://generativelanguage.googleapis.com/v1beta/models/other:generateContent"},
		{"MODEL_URL", "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent?key=test"},
		{"MODEL_URL", "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			resetConfig(t)
			t.Setenv("MODEL_PROVIDER", "gemini")
			t.Setenv("MODEL_NAME", "gemini-3.5-flash-lite")
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid model configuration accepted")
			}
		})
	}
}
