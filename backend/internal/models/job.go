package models

import "time"

var Roles = []string{"Software Engineering", "Data and Analytics", "Quality Engineering", "Infrastructure and Security", "Product and Design", "Sales and Marketing", "Business and Operations", "Other", "Unknown"}

type Job struct {
	ID                  string     `json:"id"`
	Title               string     `json:"title"`
	Company             string     `json:"company"`
	Role                string     `json:"role"`
	Cities              []string   `json:"cities"`
	LocationStatus      string     `json:"location_status"`
	WorkMode            string     `json:"work_mode"`
	EmploymentType      string     `json:"employment_type"`
	Source              string     `json:"source"`
	SourceIDs           []string   `json:"source_ids,omitempty"`
	SourceURL           string     `json:"source_url"`
	ApplyURL            string     `json:"apply_url"`
	EligibilityEvidence string     `json:"eligibility_evidence"`
	FirstSeenAt         time.Time  `json:"first_seen_at"`
	LastCheckedAt       time.Time  `json:"last_checked_at"`
	PublishedAt         *time.Time `json:"published_at"`
	ValidThrough        *time.Time `json:"valid_through,omitempty"`
	TaggingMethod       string     `json:"tagging_method"`
	DescriptionExcerpt  string     `json:"description_excerpt"`
}

type Tags struct {
	Role             string   `json:"role"`
	Cities           []string `json:"cities"`
	LocationStatus   string   `json:"location_status"`
	WorkMode         string   `json:"work_mode"`
	EmploymentType   string   `json:"employment_type"`
	RoleEvidence     string   `json:"role_evidence"`
	LocationEvidence string   `json:"location_evidence"`
}

type Observation struct {
	Raw          RawPosting `json:"raw"`
	FirstSeenAt  time.Time  `json:"first_seen_at"`
	MissingCount int        `json:"missing_count"`
	Eligible     bool       `json:"eligible"`
	Evidence     string     `json:"evidence"`
	Reason       string     `json:"reason"`
}

type SourceView struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Kind          string     `json:"kind"`
	URL           string     `json:"url"`
	Status        string     `json:"status"`
	JobCount      int        `json:"job_count"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastError     string     `json:"last_error"`
}

type Run struct {
	ID         string     `json:"id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Status     string     `json:"status"`
	Fetched    int        `json:"fetched"`
	Eligible   int        `json:"eligible"`
	Published  int        `json:"published"`
	Cached     int        `json:"cached"`
	Errors     int        `json:"errors"`
	CostUSD    *float64   `json:"cost_usd"`
}

type Evaluation struct {
	Reviewed         int      `json:"reviewed"`
	SampleSize       int      `json:"sample_size"`
	RoleAccuracy     *float64 `json:"role_accuracy"`
	LocationAccuracy *float64 `json:"location_accuracy"`
	JointAccuracy    *float64 `json:"joint_accuracy"`
}

type Costs struct {
	TaggingPer1000USD *float64 `json:"tagging_per_1000_usd"`
	InitialPer1000USD *float64 `json:"initial_per_1000_usd"`
	TotalUSD          *float64 `json:"total_usd"`
}

type Stats struct {
	ActiveJobs    int        `json:"active_jobs"`
	Companies     int        `json:"companies"`
	Cities        int        `json:"cities"`
	Sources       int        `json:"sources"`
	LastSuccessAt *time.Time `json:"last_success_at"`
	LastAttemptAt *time.Time `json:"last_attempt_at"`
	RefreshStatus string     `json:"refresh_status"`
	TaggedJobs    int        `json:"tagged_jobs"`
	PendingReview int        `json:"pending_review"`
	Evaluation    Evaluation `json:"evaluation"`
	Costs         Costs      `json:"costs"`
}

type State struct {
	Observations  map[string]Observation `json:"observations"`
	Jobs          []Job                  `json:"jobs"`
	Sources       []SourceView           `json:"sources"`
	Runs          []Run                  `json:"runs"`
	Evaluation    Evaluation             `json:"evaluation"`
	Costs         Costs                  `json:"costs"`
	Mode          string                 `json:"mode"`
	PendingReview int                    `json:"pending_review"`
	InitialCohort *InitialCohort         `json:"initial_cohort,omitempty"`
}

type InitialCohort struct {
	StartedAt   time.Time         `json:"started_at"`
	Versions    map[string]string `json:"versions"`
	CompletedAt *time.Time        `json:"completed_at"`
	CostUSD     *float64          `json:"cost_usd"`
	Per1000USD  *float64          `json:"per_1000_usd"`
}

type TagRecord struct {
	Key           string    `json:"key"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"prompt_version"`
	Tags          Tags      `json:"tags"`
	CreatedAt     time.Time `json:"created_at"`
}

type Usage struct {
	ID                       string    `json:"id"`
	RunID                    string    `json:"run_id"`
	JobID                    string    `json:"job_id"`
	ContentKey               string    `json:"content_key"`
	Model                    string    `json:"model"`
	PromptVersion            string    `json:"prompt_version"`
	RepairPolicy             string    `json:"repair_policy,omitempty"`
	RepairReason             string    `json:"repair_reason,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
	InputTokens              *int      `json:"input_tokens"`
	OutputTokens             *int      `json:"output_tokens"`
	CachedInputTokens        *int      `json:"cached_input_tokens,omitempty"`
	ThinkingTokens           *int      `json:"thinking_tokens,omitempty"`
	InputUSDPerMillion       *float64  `json:"input_usd_per_million"`
	OutputUSDPerMillion      *float64  `json:"output_usd_per_million"`
	CachedInputUSDPerMillion *float64  `json:"cached_input_usd_per_million,omitempty"`
	CostUSD                  *float64  `json:"cost_usd"`
	Status                   string    `json:"status"`
	HTTPStatus               int       `json:"http_status,omitempty"`
	FailureReason            string    `json:"failure_reason,omitempty"`
}
