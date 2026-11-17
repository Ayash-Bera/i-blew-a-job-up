export type Job = {
  id: string;
  title: string;
  company: string;
  role: string;
  cities: string[];
  location_status: string;
  work_mode: string;
  employment_type: string;
  source: string;
  source_url: string;
  apply_url: string;
  eligibility_evidence: string;
  first_seen_at: string;
  last_checked_at: string;
  published_at: string | null;
  tagging_method: string;
  description_excerpt: string;
};

export type Filters = { q: string; role: string; city: string; type: string; mode: string; page: number };
export type JobsResponse = {
  jobs: Job[];
  total: number;
  page: number;
  limit: number;
  facets: { roles: string[]; cities: string[]; types: string[]; modes: string[] };
};

export type Stats = {
  active_jobs: number;
  companies: number;
  cities: number;
  sources: number;
  last_success_at: string | null;
  last_attempt_at: string | null;
  refresh_status: string;
  tagged_jobs: number;
  pending_review: number;
  evaluation: {
    reviewed: number;
    sample_size: number;
    role_accuracy: number | null;
    location_accuracy: number | null;
    joint_accuracy: number | null;
  };
  costs: { tagging_per_1000_usd: number | null; initial_per_1000_usd: number | null; total_usd: number | null };
};

export type Source = {
  id: string;
  name: string;
  kind: string;
  url: string;
  status: string;
  job_count: number;
  last_success_at: string | null;
  last_error: string | null;
};

export type Run = {
  id: string;
  started_at: string;
  finished_at: string | null;
  status: string;
  fetched: number;
  eligible: number;
  published: number;
  cached: number;
  errors: number;
  cost_usd: number | null;
};
