package tagging

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestResponseRetryDelay(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, header, body, provider string
		want                         time.Duration
	}{
		{"seconds", "7", "", "", 7 * time.Second},
		{"date", now.Add(9 * time.Second).Format(http.TimeFormat), "", "", 9 * time.Second},
		{"past", now.Add(-time.Second).Format(http.TimeFormat), "", "", 0},
		{"negative", "-2", "", "", 0},
		{"invalid", "oops", "", "", 0},
		{"overflow", "9223372036854775807", "", "", time.Duration(1<<63 - 1)},
		{"native", "", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"2.5s"}]}}`, "gemini", 2500 * time.Millisecond},
		{"longer header", "7", `{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"2.5s"}]}}`, "gemini", 7 * time.Second},
		{"wrong type", "", `{"error":{"details":[{"@type":"other","retryDelay":"2.5s"}]}}`, "gemini", 0},
		{"invalid body", "", "{", "gemini", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := responseRetryDelay(tc.header, []byte(tc.body), tc.provider, now); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
}

func TestSharedPacing(t *testing.T) {
	client := Client{MinInterval: 40 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var mu sync.Mutex
	var starts []time.Time
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			if err := client.waitForTurn(ctx); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			starts = append(starts, time.Now())
			mu.Unlock()
		})
	}
	wg.Wait()
	if len(starts) != 3 {
		t.Fatal("pacing lost workers")
	}
	slices.SortFunc(starts, func(a, b time.Time) int { return a.Compare(b) })
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 35*time.Millisecond {
			t.Fatal("workers did not share pacing", gap)
		}
	}
}

func TestPacingCancellationAndCooldown(t *testing.T) {
	client := Client{}
	client.postpone(time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.waitForTurn(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	client.paceMu.Lock()
	remaining := time.Until(client.nextRequest)
	client.paceMu.Unlock()
	if remaining < 900*time.Millisecond {
		t.Fatal("cancelled worker consumed shared cooldown", remaining)
	}
}
