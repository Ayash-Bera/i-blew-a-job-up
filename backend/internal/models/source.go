package models

import "time"

type Source struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Board   string `json:"board"`
	Company string `json:"company"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Search  string `json:"search,omitempty"`
}

type RawPosting struct {
	SourceID        string    `json:"source_id"`
	SourceKind      string    `json:"source_kind"`
	Company         string    `json:"company"`
	PostingID       string    `json:"posting_id"`
	UnderlyingID    string    `json:"underlying_id"`
	RequisitionID   string    `json:"requisition_id"`
	Title           string    `json:"title"`
	Locations       []string  `json:"locations"`
	Country         string    `json:"country"`
	Description     string    `json:"description"`
	URL             string    `json:"url"`
	ApplyURL        string    `json:"apply_url"`
	EmploymentType  string    `json:"employment_type"`
	PublishedAt     string    `json:"published_at"`
	SourceCreatedAt string    `json:"source_created_at,omitempty"`
	ValidThrough    string    `json:"valid_through,omitempty"`
	FetchedAt       time.Time `json:"fetched_at"`
}

type SourceResult struct {
	SourceID         string       `json:"source_id"`
	Complete         bool         `json:"complete"`
	Posts            []RawPosting `json:"posts"`
	Error            string       `json:"error"`
	CheckedAt        time.Time    `json:"checked_at"`
	ClosedPostingIDs []string     `json:"closed_posting_ids,omitempty"`
}
