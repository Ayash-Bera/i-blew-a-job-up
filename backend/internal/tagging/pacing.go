package tagging

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (c *Client) waitForTurn(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.paceMu.Lock()
		now := time.Now()
		delay := c.nextRequest.Sub(now)
		if delay <= 0 {
			c.nextRequest = now.Add(c.MinInterval)
			c.paceMu.Unlock()
			return nil
		}
		c.paceMu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) postpone(delay time.Duration) {
	c.paceMu.Lock()
	defer c.paceMu.Unlock()
	if until := time.Now().Add(delay); until.After(c.nextRequest) {
		c.nextRequest = until
	}
}

func responseRetryDelay(header string, body []byte, provider string, now time.Time) time.Duration {
	var delay time.Duration
	header = strings.TrimSpace(header)
	if seconds, err := strconv.ParseInt(header, 10, 64); err == nil && seconds > 0 {
		if seconds > int64(time.Duration(1<<63-1)/time.Second) {
			delay = time.Duration(1<<63 - 1)
		} else {
			delay = time.Duration(seconds) * time.Second
		}
	} else if until, err := http.ParseTime(header); err == nil {
		delay = max(0, until.Sub(now))
	}
	if provider == "gemini" {
		var envelope struct {
			Error struct {
				Details []struct {
					Type       string `json:"@type"`
					RetryDelay string `json:"retryDelay"`
				} `json:"details"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			for _, detail := range envelope.Error.Details {
				if detail.Type == "type.googleapis.com/google.rpc.RetryInfo" {
					if d, err := time.ParseDuration(detail.RetryDelay); err == nil {
						delay = max(delay, d)
					}
				}
			}
		}
	}
	return delay
}
