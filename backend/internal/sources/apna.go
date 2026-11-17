package sources

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"firstrole/internal/models"
)

//go:embed apna-targets.json
var apnaInventory []byte

var flightBlocks = regexp.MustCompile(`(?s)self\.__next_f\.push\((\[1,.*?\])\)</script>`)
var schemaBlocks = regexp.MustCompile(`(?is)<script[^>]*type=["']application/ld\+json["'][^>]*>(.*?)</script>`)

func flightText(page []byte) (string, error) {
	var output strings.Builder
	for _, match := range flightBlocks.FindAllSubmatch(page, -1) {
		var fields []json.RawMessage
		if err := json.Unmarshal(match[1], &fields); err != nil {
			return "", fmt.Errorf("decode public page data: %w", err)
		}
		if len(fields) != 2 {
			return "", errors.New("unexpected public page data frame")
		}
		var text string
		if err := json.Unmarshal(fields[1], &text); err != nil {
			return "", fmt.Errorf("decode public page text: %w", err)
		}
		output.WriteString(text)
	}
	if output.Len() == 0 {
		return "", errors.New("public page data is missing")
	}
	return output.String(), nil
}

func extractObject(text, key string, value any) error {
	marker := `"` + key + `":`
	index := strings.Index(text, marker)
	if index < 0 {
		return fmt.Errorf("public field %s is missing", key)
	}
	if err := json.NewDecoder(strings.NewReader(text[index+len(marker):])).Decode(value); err != nil {
		return fmt.Errorf("decode public field %s: %w", key, err)
	}
	return nil
}

type apnaDetail struct {
	ID              json.Number `json:"id"`
	Title           string      `json:"title"`
	Description     string      `json:"description"`
	RichDescription string      `json:"rich_description"`
	Expiry          string      `json:"expiry"`
	Experience      string      `json:"experience_in_years"`
	Minimum         *int        `json:"min_experience"`
	Employment      string      `json:"employment_type"`
	PublicURL       string      `json:"public_url"`
	PublicURLV2     string      `json:"public_url_v2"`
	Status          *int        `json:"status"`
	Organization    struct {
		Name string `json:"name"`
	} `json:"organization"`
	Address struct {
		City struct {
			Name string `json:"name"`
		} `json:"city"`
	} `json:"address"`
	International struct {
		IsInternational *bool `json:"is_international"`
	} `json:"international_job_info"`
}

func parseApna(page []byte, source models.Source, expectedID ...string) (*models.RawPosting, error) {
	text, err := flightText(page)
	if err != nil {
		return nil, err
	}
	var detail apnaDetail
	if err := extractObject(text, "jobDetails", &detail); err != nil {
		return nil, err
	}
	if detail.ID == "" || detail.Title == "" || detail.Organization.Name == "" || detail.Minimum == nil || detail.Status == nil || detail.International.IsInternational == nil {
		return nil, errors.New("public posting is missing required validation fields")
	}
	if len(expectedID) > 0 && id(detail.ID) != expectedID[0] {
		return nil, errors.New("posting identity changed at inventory URL")
	}
	if *detail.Status != 0 || *detail.International.IsInternational || *detail.Minimum > 0 {
		return nil, nil
	}
	if detail.Expiry == "" {
		return nil, errors.New("public posting is missing expiry")
	}
	expires, err := time.ParseInLocation("2006-01-02", detail.Expiry, time.FixedZone("IST", 19800))
	if err != nil {
		return nil, fmt.Errorf("decode posting expiry: %w", err)
	}
	expires = expires.Add(24*time.Hour - time.Second)
	if expires.Before(time.Now().UTC()) {
		return nil, nil
	}
	var schema struct {
		Type        string `json:"@type"`
		DatePosted  string `json:"datePosted"`
		JobLocation struct {
			Address struct {
				Country string `json:"addressCountry"`
			} `json:"address"`
		} `json:"jobLocation"`
	}
	found := false
	for _, match := range schemaBlocks.FindAllSubmatch(page, -1) {
		var current struct {
			Type string `json:"@type"`
		}
		if err := json.Unmarshal(match[1], &current); err != nil {
			continue
		}
		if current.Type != "JobPosting" {
			continue
		}
		if err := json.Unmarshal(match[1], &schema); err != nil {
			return nil, fmt.Errorf("decode posting schema: %w", err)
		}
		found = true
		break
	}
	if !found {
		return nil, errors.New("public posting schema is missing")
	}
	if !strings.EqualFold(schema.JobLocation.Address.Country, "IN") && !strings.EqualFold(schema.JobLocation.Address.Country, "India") {
		return nil, nil
	}
	post := raw(source)
	post.Company, post.PostingID = detail.Organization.Name, id(detail.ID)
	post.UnderlyingID = id(detail.ID)
	post.Title, post.Country, post.Locations = detail.Title, "IN", unique(detail.Address.City.Name)
	description := detail.Description
	if description == "" {
		description = plain(detail.RichDescription)
	}
	post.Description = strings.TrimSpace(description) + "\nSource eligibility: " + detail.Experience
	post.URL = detail.PublicURLV2
	if post.URL == "" {
		post.URL = detail.PublicURL
	}
	if post.URL == "" {
		return nil, errors.New("public posting URL is missing")
	}
	post.ApplyURL, post.EmploymentType = post.URL, detail.Employment
	post.ValidThrough, post.PublishedAt = expires.UTC().Format(time.RFC3339), schema.DatePosted
	return &post, nil
}

func (c *collector) apna(ctx context.Context, source models.Source) ([]models.RawPosting, []string, error) {
	var inventory struct {
		Targets []pageTarget `json:"targets"`
	}
	if err := json.Unmarshal(apnaInventory, &inventory); err != nil {
		return nil, nil, fmt.Errorf("read target inventory: %w", err)
	}
	if len(inventory.Targets) == 0 {
		return nil, nil, errors.New("target inventory is empty")
	}
	return c.apnaTargets(ctx, source, inventory.Targets)
}

func (c *collector) apnaTargets(ctx context.Context, source models.Source, targets []pageTarget) ([]models.RawPosting, []string, error) {
	return c.inventoryPages(ctx, targets, func(page []byte, index int) (*models.RawPosting, error) {
		return parseApna(page, source, targets[index].PostingID)
	})
}
