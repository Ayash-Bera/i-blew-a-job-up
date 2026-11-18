package sources

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"firstrole/internal/models"
)

//go:embed registry.json
var defaultRegistry []byte

const maxResponseBytes = 32 << 20

type collector struct {
	client         *http.Client
	greenhouseBase string
	leverBase      string
	ashbyBase      string
	smartBase      string
	pageSize       int
	maxPages       int
}

func newCollector() *collector {
	return &collector{
		client:         &http.Client{Timeout: 35 * time.Second},
		greenhouseBase: "https://boards-api.greenhouse.io/v1/boards/",
		leverBase:      "https://api.lever.co/v0/postings/",
		ashbyBase:      "https://api.ashbyhq.com/posting-api/job-board/",
		smartBase:      "https://api.smartrecruiters.com/v1/companies/",
		pageSize:       100,
		maxPages:       500,
	}
}

func Registry(path string) ([]models.Source, error) {
	data := defaultRegistry
	if path != "" {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read source registry: %w", err)
		}
	}
	var registry []models.Source
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("decode source registry: %w", err)
	}
	seen := map[string]bool{}
	for _, source := range registry {
		if source.ID == "" || source.Board == "" || source.Company == "" || seen[source.ID] {
			return nil, fmt.Errorf("invalid or repeated source identifier %q", source.ID)
		}
		seen[source.ID] = true
	}
	return registry, nil
}

func Fetch(ctx context.Context, registryPath string) ([]models.SourceResult, error) {
	registry, err := Registry(registryPath)
	if err != nil {
		return nil, err
	}
	return newCollector().fetch(ctx, registry), nil
}

func (c *collector) fetch(ctx context.Context, registry []models.Source) []models.SourceResult {
	results := make([]models.SourceResult, len(registry))
	var group sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, source := range registry {
		if !source.Enabled {
			continue
		}
		group.Add(1)
		go func(i int, source models.Source) {
			defer group.Done()
			result := models.SourceResult{SourceID: source.ID, CheckedAt: time.Now().UTC(), Posts: []models.RawPosting{}}
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				result.Error = ctx.Err().Error()
				results[i] = result
				return
			}
			var err error
			switch source.Kind {
			case "greenhouse":
				result.Posts, err = c.greenhouse(ctx, source)
			case "lever":
				result.Posts, err = c.lever(ctx, source)
			case "ashby":
				result.Posts, err = c.ashby(ctx, source)
			case "smartrecruiters":
				result.Posts, err = c.smart(ctx, source)
			case "workday":
				result.Posts, err = c.workday(ctx, source)
			case "apna":
				result.Posts, result.ClosedPostingIDs, err = c.apna(ctx, source)
			case "marriott":
				result.Posts, result.ClosedPostingIDs, err = c.marriott(ctx, source)
			default:
				err = fmt.Errorf("unsupported source kind %q", source.Kind)
			}
			result.Complete = err == nil
			if err != nil {
				result.Error = err.Error()
			}
			for j := range result.Posts {
				result.Posts[j].FetchedAt = result.CheckedAt
			}
			results[i] = result
		}(i, source)
	}
	group.Wait()
	out := make([]models.SourceResult, 0, len(results))
	for _, result := range results {
		if result.SourceID != "" {
			out = append(out, result)
		}
	}
	return out
}

func (c *collector) request(ctx context.Context, method, url string, body any, output any) error {
	data, err := c.requestBytes(ctx, method, url, body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode source response: %w", err)
	}
	return nil
}

type statusError struct {
	Code       int
	RetryAfter string
}

func (err statusError) Error() string {
	message := fmt.Sprintf("source returned HTTP %d", err.Code)
	if err.RetryAfter != "" {
		message += "; Retry-After: " + err.RetryAfter
	}
	return message
}

func restricted(err error) bool {
	var status statusError
	return errors.As(err, &status) && (status.Code == 403 || status.Code == 429 || status.Code == 481)
}

func (c *collector) requestBytes(ctx context.Context, method, url string, body any) ([]byte, error) {
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		request.Header.Set("User-Agent", "IBlewAJobUp/1.0 (public employer job listings)")
		request.Header.Set("Accept", "application/json")
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		response, err := c.client.Do(request)
		if err != nil {
			last = fmt.Errorf("fetch %s: %w", url, err)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		response.Body.Close()
		if readErr != nil {
			last = fmt.Errorf("read response: %w", readErr)
			continue
		}
		if len(data) > maxResponseBytes {
			return nil, errors.New("source response exceeds size limit")
		}
		if response.StatusCode != http.StatusOK {
			last = statusError{Code: response.StatusCode, RetryAfter: response.Header.Get("Retry-After")}
			if response.StatusCode >= 500 {
				continue
			}
			return nil, last
		}
		return data, nil
	}
	return nil, last
}

var tags = regexp.MustCompile(`<[^>]*>`)
var blocks = regexp.MustCompile(`(?i)</?(?:p|div|li|br|h[1-6]|ul|ol|section|tr|td)[^>]*>`)
var space = regexp.MustCompile(`[\t\r\f\v ]+`)
var blank = regexp.MustCompile(`\n\s*\n+`)

func plain(input string) string {
	for i := 0; i < 2; i++ {
		input = html.UnescapeString(input)
	}
	input = blocks.ReplaceAllString(input, "\n")
	input = tags.ReplaceAllString(input, " ")
	input = strings.ReplaceAll(input, "\u00a0", " ")
	input = space.ReplaceAllString(input, " ")
	input = blank.ReplaceAllString(input, "\n")
	return strings.TrimSpace(input)
}

func id(value json.Number) string { return string(value) }

func epoch(milliseconds int64) string {
	if milliseconds <= 0 {
		return ""
	}
	return time.UnixMilli(milliseconds).UTC().Format(time.RFC3339)
}

func unique(values ...string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	return result
}

func number(value int) string { return strconv.Itoa(value) }

func raw(source models.Source) models.RawPosting {
	return models.RawPosting{SourceID: source.ID, SourceKind: source.Kind, Company: source.Company, Locations: []string{}}
}
