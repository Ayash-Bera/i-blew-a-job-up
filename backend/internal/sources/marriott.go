package sources

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"firstrole/internal/models"
)

//go:embed marriott-targets.json
var marriottInventory []byte

type marriottTarget struct {
	URL           string `json:"url"`
	PostingID     string `json:"posting_id"`
	RequisitionID string `json:"requisition_id"`
	ValidThrough  string `json:"valid_through,omitempty"`
}

var employerApply = regexp.MustCompile(`data-apply-url="([^"]+)"`)
var employerRequisition = regexp.MustCompile(`<dd class="summary-value reqid">([^<]+)</dd>`)

func parseMarriott(page []byte, source models.Source, target marriottTarget) (*models.RawPosting, error) {
	var schema struct {
		Type           string   `json:"@type"`
		URL            string   `json:"url"`
		Title          string   `json:"title"`
		Description    string   `json:"description"`
		DatePosted     string   `json:"datePosted"`
		ValidThrough   string   `json:"validThrough"`
		EmploymentType []string `json:"employmentType"`
		Identifier     struct {
			Value string `json:"value"`
		} `json:"identifier"`
		Organization struct {
			Name string `json:"name"`
		} `json:"hiringOrganization"`
		Locations []struct {
			Address struct {
				City    string `json:"addressLocality"`
				Region  string `json:"addressRegion"`
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
			return nil, fmt.Errorf("decode employer schema: %w", err)
		}
		found = true
		break
	}
	if !found {
		return nil, errors.New("employer posting schema is missing")
	}
	if schema.Identifier.Value == "" || schema.Title == "" || schema.Description == "" || schema.URL == "" {
		return nil, errors.New("employer posting fields are incomplete")
	}
	if target.PostingID != "" && schema.Identifier.Value != target.PostingID {
		return nil, errors.New("posting identity changed at inventory URL")
	}
	apply := employerApply.FindSubmatch(page)
	requisition := employerRequisition.FindSubmatch(page)
	if len(apply) != 2 || len(requisition) != 2 {
		return nil, errors.New("employer apply link or requisition is missing")
	}
	actualID := strings.TrimSpace(string(requisition[1]))
	if actualID != target.RequisitionID {
		return nil, errors.New("employer requisition changed at inventory URL")
	}
	post := raw(source)
	post.PostingID, post.UnderlyingID, post.RequisitionID = schema.Identifier.Value, actualID, actualID
	post.Company = "Marriott International"
	post.Title, post.Description = schema.Title, plain(schema.Description)
	post.URL, post.ApplyURL, post.PublishedAt = schema.URL, html.UnescapeString(string(apply[1])), schema.DatePosted
	post.EmploymentType = strings.Join(schema.EmploymentType, ", ")
	post.ValidThrough = schema.ValidThrough
	if post.ValidThrough == "" {
		post.ValidThrough = target.ValidThrough
	}
	if post.ValidThrough != "" {
		expires, err := time.Parse(time.RFC3339, post.ValidThrough)
		if err != nil {
			return nil, fmt.Errorf("parse employer expiry: %w", err)
		}
		if expires.Before(time.Now().UTC()) {
			return nil, nil
		}
	}
	for _, location := range schema.Locations {
		if !strings.EqualFold(location.Address.Country, "IN") {
			continue
		}
		post.Country = "IN"
		post.Locations = append(post.Locations, location.Address.City)
	}
	post.Locations = unique(post.Locations...)
	if post.Country == "" {
		return nil, nil
	}
	return &post, nil
}

func (c *collector) marriott(ctx context.Context, source models.Source) ([]models.RawPosting, []string, error) {
	var inventory struct {
		Targets []marriottTarget `json:"targets"`
	}
	if err := json.Unmarshal(marriottInventory, &inventory); err != nil {
		return nil, nil, fmt.Errorf("read employer inventory: %w", err)
	}
	if len(inventory.Targets) == 0 {
		return nil, nil, errors.New("employer target inventory is empty")
	}
	return c.marriottTargets(ctx, source, inventory.Targets)
}

func (c *collector) marriottTargets(ctx context.Context, source models.Source, targets []marriottTarget) ([]models.RawPosting, []string, error) {
	pages := make([]pageTarget, len(targets))
	for i, target := range targets {
		pages[i] = pageTarget{URL: target.URL, PostingID: target.PostingID}
	}
	return c.inventoryPages(ctx, pages, func(page []byte, index int) (*models.RawPosting, error) {
		return parseMarriott(page, source, targets[index])
	})
}
