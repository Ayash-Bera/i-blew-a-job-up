package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"firstrole/internal/models"
)

type pageTarget struct {
	URL       string `json:"url"`
	PostingID string `json:"posting_id"`
}

func (c *collector) inventoryPages(ctx context.Context, targets []pageTarget, parse func([]byte, int) (*models.RawPosting, error)) ([]models.RawPosting, []string, error) {
	seen := map[string]bool{}
	for _, target := range targets {
		if target.URL == "" || target.PostingID == "" || seen[target.PostingID] {
			return nil, nil, errors.New("inventory target is missing URL or has missing or repeated observed posting ID")
		}
		seen[target.PostingID] = true
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	posts := make([]*models.RawPosting, len(targets))
	closed := make([]bool, len(targets))
	errorsByIndex := make([]error, len(targets))
	var group sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, target := range targets {
		group.Add(1)
		go func(i int, target pageTarget) {
			defer group.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errorsByIndex[i] = context.Cause(ctx)
				return
			}
			if ctx.Err() != nil {
				errorsByIndex[i] = context.Cause(ctx)
				return
			}
			page, err := c.requestBytes(ctx, http.MethodGet, target.URL, nil)
			if err != nil {
				var status statusError
				if errors.As(err, &status) && (status.Code == 404 || status.Code == 410) {
					closed[i] = true
					return
				}
				errorsByIndex[i] = fmt.Errorf("refresh inventory posting: %w", err)
				if restricted(err) {
					cancel(errorsByIndex[i])
				}
				return
			}
			posts[i], errorsByIndex[i] = parse(page, i)
			if posts[i] != nil && posts[i].PostingID != target.PostingID {
				posts[i] = nil
				errorsByIndex[i] = errors.New("posting identity changed at inventory URL")
			}
			closed[i] = posts[i] == nil && errorsByIndex[i] == nil
		}(i, target)
	}
	group.Wait()
	result := []models.RawPosting{}
	closedIDs := []string{}
	first := context.Cause(ctx)
	for i, post := range posts {
		if first == nil && errorsByIndex[i] != nil {
			first = errorsByIndex[i]
		}
		if post != nil {
			result = append(result, *post)
		}
		if closed[i] {
			closedIDs = append(closedIDs, targets[i].PostingID)
		}
	}
	return result, closedIDs, first
}
