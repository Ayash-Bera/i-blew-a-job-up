package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"firstrole/internal/models"
)

func (c *collector) greenhouse(ctx context.Context, source models.Source) ([]models.RawPosting, error) {
	var payload struct {
		Jobs []struct {
			ID            json.Number  `json:"id"`
			InternalID    *json.Number `json:"internal_job_id"`
			RequisitionID string       `json:"requisition_id"`
			Title         string       `json:"title"`
			Location      struct {
				Name string `json:"name"`
			} `json:"location"`
			Content        string `json:"content"`
			URL            string `json:"absolute_url"`
			FirstPublished string `json:"first_published"`
		} `json:"jobs"`
		Meta struct {
			Total *int `json:"total"`
		} `json:"meta"`
	}
	endpoint := c.greenhouseBase + url.PathEscape(source.Board) + "/jobs?content=true"
	if err := c.request(ctx, http.MethodGet, endpoint, nil, &payload); err != nil {
		return nil, err
	}
	if payload.Jobs == nil {
		return nil, fmt.Errorf("missing greenhouse jobs array")
	}
	posts := []models.RawPosting{}
	for _, job := range payload.Jobs {
		if job.InternalID == nil {
			continue
		}
		if job.ID == "" || job.Title == "" || job.URL == "" {
			return posts, fmt.Errorf("incomplete greenhouse posting")
		}
		post := raw(source)
		post.PostingID, post.UnderlyingID, post.RequisitionID = id(job.ID), id(*job.InternalID), job.RequisitionID
		post.Title, post.Locations = job.Title, unique(job.Location.Name)
		post.Description, post.URL, post.ApplyURL = plain(job.Content), job.URL, job.URL
		post.PublishedAt = job.FirstPublished
		posts = append(posts, post)
	}
	if payload.Meta.Total != nil && *payload.Meta.Total != len(payload.Jobs) {
		return posts, fmt.Errorf("greenhouse count mismatch: received %d of %d", len(payload.Jobs), *payload.Meta.Total)
	}
	return posts, nil
}

func (c *collector) lever(ctx context.Context, source models.Source) ([]models.RawPosting, error) {
	type leverJob struct {
		ID              string `json:"id"`
		Text            string `json:"text"`
		Description     string `json:"descriptionPlain"`
		DescriptionHTML string `json:"description"`
		Additional      string `json:"additionalPlain"`
		Opening         string `json:"openingPlain"`
		HostedURL       string `json:"hostedUrl"`
		ApplyURL        string `json:"applyUrl"`
		CreatedAt       int64  `json:"createdAt"`
		Categories      struct {
			Location     string   `json:"location"`
			AllLocations []string `json:"allLocations"`
			Commitment   string   `json:"commitment"`
		} `json:"categories"`
		Lists []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
	}
	posts := []models.RawPosting{}
	seen := map[string]bool{}
	for page := 0; page < c.maxPages; page++ {
		var jobs []leverJob
		endpoint := c.leverBase + url.PathEscape(source.Board) + "?mode=json&limit=" + number(c.pageSize) + "&skip=" + number(page*c.pageSize)
		if err := c.request(ctx, http.MethodGet, endpoint, nil, &jobs); err != nil {
			return posts, err
		}
		if jobs == nil {
			return posts, fmt.Errorf("missing lever jobs array")
		}
		for _, job := range jobs {
			if job.ID == "" || job.Text == "" || job.HostedURL == "" {
				return posts, fmt.Errorf("incomplete lever posting")
			}
			if seen[job.ID] {
				return posts, fmt.Errorf("lever repeated posting %s during pagination", job.ID)
			}
			seen[job.ID] = true
			post := raw(source)
			post.PostingID, post.UnderlyingID = job.ID, job.ID
			post.Title = job.Text
			post.Locations = unique(append([]string{job.Categories.Location}, job.Categories.AllLocations...)...)
			description := job.Description
			if description == "" {
				description = plain(job.DescriptionHTML)
			}
			parts := []string{job.Opening, description}
			for _, section := range job.Lists {
				parts = append(parts, section.Text, plain(section.Content))
			}
			parts = append(parts, job.Additional)
			post.Description = plain(strings.Join(parts, "\n"))
			post.URL, post.ApplyURL, post.EmploymentType = job.HostedURL, job.ApplyURL, job.Categories.Commitment
			if post.ApplyURL == "" {
				post.ApplyURL = post.URL
			}
			post.SourceCreatedAt = epoch(job.CreatedAt)
			posts = append(posts, post)
		}
		if len(jobs) < c.pageSize {
			return posts, nil
		}
	}
	return posts, fmt.Errorf("lever pagination safety limit reached")
}

func (c *collector) ashby(ctx context.Context, source models.Source) ([]models.RawPosting, error) {
	var payload struct {
		Jobs []struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Location  string `json:"location"`
			Secondary []struct {
				Location string `json:"location"`
			} `json:"secondaryLocations"`
			Description     string `json:"descriptionPlain"`
			DescriptionHTML string `json:"descriptionHtml"`
			JobURL          string `json:"jobUrl"`
			ApplyURL        string `json:"applyUrl"`
			Employment      string `json:"employmentType"`
			Published       string `json:"publishedAt"`
			Listed          *bool  `json:"isListed"`
			Address         struct {
				PostalAddress struct {
					Country string `json:"addressCountry"`
				} `json:"postalAddress"`
			} `json:"address"`
		} `json:"jobs"`
	}
	if err := c.request(ctx, http.MethodGet, c.ashbyBase+url.PathEscape(source.Board), nil, &payload); err != nil {
		return nil, err
	}
	if payload.Jobs == nil {
		return nil, fmt.Errorf("missing ashby jobs array")
	}
	posts := []models.RawPosting{}
	for _, job := range payload.Jobs {
		if job.Listed != nil && !*job.Listed {
			continue
		}
		if job.ID == "" || job.Title == "" || job.JobURL == "" {
			return posts, fmt.Errorf("incomplete ashby posting")
		}
		post := raw(source)
		post.PostingID, post.UnderlyingID, post.Title = job.ID, job.ID, job.Title
		locations := []string{job.Location}
		for _, secondary := range job.Secondary {
			locations = append(locations, secondary.Location)
		}
		post.Locations, post.Country = unique(locations...), job.Address.PostalAddress.Country
		post.Description = job.Description
		if post.Description == "" {
			post.Description = plain(job.DescriptionHTML)
		}
		post.URL, post.ApplyURL, post.PublishedAt, post.EmploymentType = job.JobURL, job.ApplyURL, job.Published, job.Employment
		if post.ApplyURL == "" {
			post.ApplyURL = post.URL
		}
		posts = append(posts, post)
	}
	return posts, nil
}
