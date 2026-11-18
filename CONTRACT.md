# API contract

I Blew a Job Up collects public fresher jobs and internships in India. Evaluation results remain pending until measured.

## Interfaces

Backend module `firstrole`, under `backend/`, uses Go and chi. Frontend under `web/` uses Next.js and the single client `src/lib/api.ts`. All timestamps use RFC3339 UTC over the API, formatted in IST in the interface.

`GET /api/jobs?q=&role=&city=&type=&mode=&page=1&limit=20` returns `{jobs: Job[], total: number, page: number, limit: number, facets: {roles: string[], cities: string[], types: string[], modes: string[]}}`.

Job fields: `id, title, company, role, cities: string[], location_status, work_mode, employment_type, source, source_url, apply_url, eligibility_evidence, first_seen_at, last_checked_at, published_at: string|null, tagging_method, description_excerpt`. Strings except explicitly typed fields. Active jobs only; never substitute fixtures in production.

`GET /api/stats` returns `{active_jobs, companies, cities, sources, last_success_at: string|null, last_attempt_at: string|null, refresh_status, tagged_jobs, pending_review, evaluation: {reviewed, sample_size, role_accuracy: number|null, location_accuracy: number|null, joint_accuracy: number|null}, costs: {tagging_per_1000_usd: number|null, initial_per_1000_usd: number|null, total_usd: number|null}}`.

`GET /api/sources` returns `{sources: [{id, name, kind, url, status, job_count, last_success_at, last_error}]}`. `GET /api/runs` returns `{runs: [{id, started_at, finished_at, status, fetched, eligible, published, cached, errors, cost_usd}]}`. `GET /api/export.csv` downloads the current public dataset. Errors use `{error: string}` and an appropriate HTTP status.

Role taxonomy: Software Engineering; Data and Analytics; Quality Engineering; Infrastructure and Security; Product and Design; Sales and Marketing; Business and Operations; Other; Unknown.
Location status: city, region, remote, unspecified. Work mode: onsite, hybrid, remote, unspecified. Employment type: internship, full-time, part-time, apprenticeship, contract, unspecified.

Source connector entry point: `sources.Fetch(ctx context.Context, registryPath string) ([]models.SourceResult, error)`. Source registry is JSON array of models.Source. Fetch failures appear in each SourceResult and never become complete empty boards. Functions honor cancellation and bounded concurrency.
