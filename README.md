# I Blew a Job Up

Fresher jobs and internships in India, with original source links, duplicate handling, role and city tags, and a daily refresh pipeline.

Repository: https://github.com/Ayash-Bera/i-blew-a-job-up

Live tool: https://i-blew-a-job-up-ashen.vercel.app

## Status

The live tool publishes 221 unique, current India fresher and internship posts with validated role and city tags. The database deployment uses a dedicated Neon Free Postgres database. Public API verification on November 18, 2026, 10:25:42 PM IST passed pagination, count consistency, filters, CSV export, and refresh authorization.

An authenticated repeat refresh checked all 221 posts, reused 221 cached results, made zero provider requests, and reported zero errors. The daily schedule is deployed, but an actual scheduled invocation has not yet been observed. The 50-row human review, applicant video, and forms remain pending. The assignment is not ready for submission.

First-load tagging list-price spend is $0.1104267: $0.52584143 per 1,000 distinct attempted versions, or $0.49966833 per 1,000 published posts. These figures include failed attempts and retries in the selected first-load runs, exclude v1 development, and are not reconciled cash charges. See [EVALUATION.md](EVALUATION.md) for reproducible accounting and audit commands.

The frozen dataset represents 11 employers. Marriott International contributes 186 of 221 posts (84.2%), so the later accuracy result must disclose that concentration. 17 posts have known expiry before the submission deadline, and 113 have no supplied expiry. Recheck freshness on submission day.

The blind 50-post sample excludes 28 pilot and diagnostic IDs. No human accuracy result is claimed. See [SETUP.md](SETUP.md) for the tested runtime configuration.

## Structure

- `backend/`: Go service, public source connectors, normalization, tagging, ingestion, Postgres migrations, and tests.
- `web/`: Next.js interface with search, filters, source evidence, CSV export, and a methodology page.
- `data/`: public source registry and curated listing identifiers.
- `scripts/`: reproducible evaluation and integration verification.

The Go service uses chi and pgx. PostgreSQL stores observations, canonical jobs, tag cache, usage, and refresh history. Vercel serves the frontend and backend through one project with `/api/*` routed to Go. The deployment configuration schedules a daily refresh around 6:00 AM IST. The Hobby scheduler has an hourly scheduling window; configured does not mean a run has executed.

## Data rules

Only explicit India locations and evidence of fresher or student eligibility qualify. An entry-level title alone is insufficient. Expired jobs and jobs unchecked for over 48 hours leave the public results. A missing record after a complete source refresh is excluded immediately. Failed or partial fetches do not imply removal.

Duplicate groups retain source observations and employer-scoped identifiers. Greenhouse posting IDs are distinguished from underlying vacancy IDs. Exact-content mirror matching is conservative so separate requisitions remain separate jobs.

The Marriott connector rechecks a curated inventory of public listing URLs. Its completeness means that inventory was checked, not that every employer vacancy was discovered. Employer feeds discover new listings during refreshes; expanding the Marriott inventory is a separate collection step. Apna access was restricted during research and its connector is disabled. It is excluded from the deployed source set.

Production publishes successfully validated model tags. A read-only preview can display real records with pending tags, clearly identified as incomplete. Job descriptions are untrusted input; the model receives a fixed extraction prompt, a closed taxonomy, and a strict response schema. Failed outputs stay outside the production count.

## Checks

From `backend/`, run `go test ./...`, `go build ./...`, and `go vet ./...`. Database integration tests use `TEST_DATABASE_URL` and an isolated test database. From `web/`, run `npm ci`, `npm run lint`, `npm run typecheck`, `npm test`, and `npm run build`.

From the repository root, run `python3 -m unittest discover -s scripts -p 'test_*.py'`. Run `python3 scripts/verify_api.py --base https://YOUR_DEPLOYMENT --min-jobs 200` against the deployed service. This final check requires 200 distinct model-tagged posts and verifies pagination, counts, filters, export, and refresh authorization.

## Submission evidence

The working evidence package must include the frozen dataset, source register, prompt and model settings, actual refresh records, usage and cost calculations, real application screenshots, and the blind 50-row review sheet. Unfinished measurements stay marked pending. The applicant supplies the face-and-screen walkthrough, at most five minutes, and reviews findings written in their own words before submission.
