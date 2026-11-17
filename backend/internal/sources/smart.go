package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"firstrole/internal/models"
)

type smartJob struct {
	ID         string `json:"id"`
	JobID      string `json:"jobId"`
	Name       string `json:"name"`
	RefNumber  string `json:"refNumber"`
	PostingURL string `json:"postingUrl"`
	ApplyURL   string `json:"applyUrl"`
	Released   string `json:"releasedDate"`
	Location   struct {
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
		Full    string `json:"fullLocation"`
	} `json:"location"`
	Employment struct {
		Label string `json:"label"`
	} `json:"typeOfEmployment"`
	JobAd struct {
		Sections map[string]struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"sections"`
	} `json:"jobAd"`
}

func (c *collector) smart(ctx context.Context, source models.Source) ([]models.RawPosting, error) {
	base := c.smartBase + url.PathEscape(source.Board) + "/postings"
	list := []smartJob{}
	seen := map[string]bool{}
	expected := -1
	finished := false
	for page := 0; page < c.maxPages; page++ {
		var payload struct {
			Content []smartJob `json:"content"`
			Total   *int       `json:"totalFound"`
		}
		endpoint := base + "?country=in&limit=" + number(c.pageSize) + "&offset=" + number(page*c.pageSize)
		if err := c.request(ctx, http.MethodGet, endpoint, nil, &payload); err != nil {
			return nil, err
		}
		if payload.Content == nil || payload.Total == nil {
			return nil, fmt.Errorf("missing smartrecruiters content or total")
		}
		if expected < 0 {
			expected = *payload.Total
		}
		if expected != *payload.Total {
			return nil, fmt.Errorf("smartrecruiters result count changed during pagination")
		}
		for _, job := range payload.Content {
			if job.ID == "" || seen[job.ID] {
				return nil, fmt.Errorf("missing or repeated smartrecruiters identifier")
			}
			seen[job.ID] = true
			list = append(list, job)
		}
		if len(list) >= expected {
			finished = true
			break
		}
		if len(payload.Content) == 0 {
			return nil, fmt.Errorf("smartrecruiters ended after %d of %d", len(list), expected)
		}
	}
	if !finished {
		return nil, fmt.Errorf("smartrecruiters pagination safety limit reached")
	}
	posts := make([]models.RawPosting, len(list))
	errors := make([]error, len(list))
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var group sync.WaitGroup
	sem := make(chan struct{}, 3)
	for i, summary := range list {
		group.Add(1)
		go func(i int, summary smartJob) {
			defer group.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errors[i] = context.Cause(ctx)
				return
			}
			if ctx.Err() != nil {
				errors[i] = context.Cause(ctx)
				return
			}
			var job smartJob
			if err := c.request(ctx, http.MethodGet, base+"/"+url.PathEscape(summary.ID), nil, &job); err != nil {
				errors[i] = err
				if restricted(err) {
					cancel(err)
				}
				return
			}
			if job.ID == "" || job.Name == "" || job.PostingURL == "" {
				errors[i] = fmt.Errorf("incomplete smartrecruiters detail")
				return
			}
			post := raw(source)
			post.PostingID, post.UnderlyingID, post.RequisitionID = job.ID, job.JobID, job.RefNumber
			post.Title, post.Country = job.Name, job.Location.Country
			location := job.Location.Full
			if location == "" {
				location = strings.Join(unique(job.Location.City, job.Location.Region, job.Location.Country), ", ")
			}
			post.Locations = unique(location)
			parts := []string{}
			for _, name := range []string{"jobDescription", "qualifications", "additionalInformation", "companyDescription"} {
				section := job.JobAd.Sections[name]
				parts = append(parts, section.Title, section.Text)
			}
			post.Description = plain(strings.Join(parts, "\n"))
			post.URL, post.ApplyURL, post.PublishedAt, post.EmploymentType = job.PostingURL, job.ApplyURL, job.Released, job.Employment.Label
			if post.ApplyURL == "" {
				post.ApplyURL = post.URL
			}
			posts[i] = post
		}(i, summary)
	}
	group.Wait()
	result := []models.RawPosting{}
	first := context.Cause(ctx)
	for i, post := range posts {
		if errors[i] != nil && first == nil {
			first = fmt.Errorf("posting detail incomplete: %w", errors[i])
		}
		if post.PostingID != "" {
			result = append(result, post)
		}
	}
	return result, first
}
