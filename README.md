# FirstRole

Fresher jobs and internships in India, with original source links, duplicate handling, role and city tags, and a daily refresh pipeline.

Repository: https://github.com/Ayash-Bera/firstrole-india

Live preview: https://firstrole-india.vercel.app

## Status

The implemented tool is deployed as a read-only preview with real collected listings. Durable database configuration and model credentials are still required to enable tagging and unattended refresh. Human evaluation and measured model costs are pending. A saved snapshot is explicitly marked read-only. Pending tags and unknown costs are never presented as measured results.

The qualifying dataset target is at least 200 unique current posts. The final accuracy result requires 50 human-reviewed records. See [SETUP.md](SETUP.md) for runtime configuration and [EVALUATION.md](EVALUATION.md) for the evaluation procedure.

The latest complete live-source check retained 226 eligible posts from 11 employers across 12 enabled source boards in 27.82 seconds. That is a collection result, with model tags still pending. The data is concentrated in hospitality: 191 records came from Marriott. Known expiry dates can reduce this count before submission, so a fresh check and a larger buffer are required.

## Structure

- `backend/`: Go service, public source connectors, normalization, tagging, ingestion, Postgres migrations, and tests.
- `web/`: Next.js interface with search, filters, source evidence, CSV export, and a methodology page.
- `data/`: public source registry and curated listing identifiers.
- `scripts/`: reproducible evaluation and integration verification.

The Go service uses chi and pgx. PostgreSQL stores observations, canonical jobs, tag cache, usage, and refresh history. Vercel serves the frontend and backend through one project with `/api/*` routed to Go. The deployment configuration schedules a daily refresh around 6:00 AM IST. The Hobby scheduler has an hourly scheduling window; configured does not mean a run has executed.

## Data rules

Only explicit India locations and evidence of fresher or student eligibility qualify. An entry-level title alone is insufficient. Expired jobs and jobs unchecked for over 48 hours leave the public results. A missing record after a complete source refresh is excluded immediately. Failed or partial fetches do not imply removal.

Duplicate groups retain source observations and employer-scoped identifiers. Greenhouse posting IDs are distinguished from underlying vacancy IDs. Exact-content mirror matching is conservative so separate requisitions remain separate jobs.

The Marriott connector rechecks a curated inventory of public listing URLs. Its completeness means that inventory was checked, not that every employer vacancy was discovered. Employer feeds discover new listings during refreshes; expanding the Marriott inventory is a separate collection step. Apna access was restricted during research and its connector is disabled. It is excluded from the current deployable snapshot.

Production publishes successfully validated model tags. A read-only preview can display real records with pending tags, clearly identified as incomplete. Job descriptions are untrusted input; the model receives a fixed extraction prompt, a closed taxonomy, and a strict response schema. Failed outputs stay outside the production count.

## Checks

From `backend/`, run `go test ./...`, `go build ./...`, and `go vet ./...`. Database integration tests use `TEST_DATABASE_URL` and an isolated test database. From `web/`, run `npm ci`, `npm run lint`, `npm run typecheck`, `npm test`, and `npm run build`.

From the repository root, run `python3 -m unittest discover -s scripts -p 'test_*.py'`. Run `python3 scripts/verify_api.py --base https://YOUR_DEPLOYMENT --min-jobs 200` against the deployed service. This final check requires 200 distinct model-tagged posts and verifies pagination, counts, filters, export, and refresh authorization.

## Submission evidence

The working evidence package must include the frozen dataset, source register, prompt and model settings, actual refresh records, usage and cost calculations, real application screenshots, and the blind 50-row review sheet. Unfinished measurements stay marked pending. The applicant supplies the face-and-screen walkthrough, at most five minutes, and reviews findings written in their own words before submission.
