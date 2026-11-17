# FirstRole implementation contract

Working code is staged here for the requested repository. Public product name: FirstRole. No mock listings or invented evaluation results.

## Interfaces

Backend module `firstrole`, under `backend/`, uses Go and chi. Frontend under `web/` uses Next.js and the single client `src/lib/api.ts`. All timestamps use RFC3339 UTC over the API, formatted in IST in the interface.

`GET /api/jobs?q=&role=&city=&type=&mode=&page=1&limit=20` returns `{jobs: Job[], total: number, page: number, limit: number, facets: {roles: string[], cities: string[], types: string[], modes: string[]}}`.

Job fields: `id, title, company, role, cities: string[], location_status, work_mode, employment_type, source, source_url, apply_url, eligibility_evidence, first_seen_at, last_checked_at, published_at: string|null, tagging_method, description_excerpt`. Strings except explicitly typed fields. Active jobs only; never substitute fixtures in production.

`GET /api/stats` returns `{active_jobs, companies, cities, sources, last_success_at: string|null, last_attempt_at: string|null, refresh_status, tagged_jobs, pending_review, evaluation: {reviewed, sample_size, role_accuracy: number|null, location_accuracy: number|null, joint_accuracy: number|null}, costs: {tagging_per_1000_usd: number|null, initial_per_1000_usd: number|null, total_usd: number|null}}`.

`GET /api/sources` returns `{sources: [{id, name, kind, url, status, job_count, last_success_at, last_error}]}`. `GET /api/runs` returns `{runs: [{id, started_at, finished_at, status, fetched, eligible, published, cached, errors, cost_usd}]}`. `GET /api/export.csv` downloads the current public dataset. Errors use `{error: string}` and an appropriate HTTP status.

Role taxonomy: Software Engineering; Data and Analytics; Quality Engineering; Infrastructure and Security; Product and Design; Sales and Marketing; Business and Operations; Other; Unknown.
Location status: city, region, remote, unspecified. Work mode: onsite, hybrid, remote, unspecified. Employment type: internship, full-time, part-time, apprenticeship, contract, unspecified.

Source connector entry point: `sources.Fetch(ctx context.Context, registryPath string) ([]models.SourceResult, error)`. Source registry is JSON array of models.Source. Fetch failures appear in each SourceResult and never become complete empty boards. Functions honor cancellation and bounded concurrency.

## Ownership

- source leaf: `backend/internal/sources/**`, `data/**`, `scripts/source-*`. Expands real source coverage, implements adapters and preserves machine-readable evidence. Does not own go.mod or shared models.
- backend leaf: `backend/**` except `backend/internal/sources/**`. Implements normalized eligibility, deduplication, strict model tagging, cost accounting, Postgres persistence, daily refresh route, API, CLI, tests. May extend shared models while preserving connector fields.
- frontend leaf: `web/**`. Implements public search, filters, states, source/methodology/evidence views. No server business logic, mock data, or publication.
- root: integration, project documentation, hosting, evaluation scripts, artifacts, independent verification. Root does not edit leaf-owned files until coordinating ownership handoff.

Production target is Vercel through its CLI. Initially use separate frontend and Go projects unless verified Services support allows a single project. Postgres is durable storage, never the serverless filesystem. Missing credentials stay a visible blocker. No push or PR without the user's exact-action approval. No external submissions. Video is user-owned; human reference labels cannot be invented.

States: source READY; backend READY; frontend READY. Shared interface verified by reading this file before dispatch. Root integration remains OPEN.
