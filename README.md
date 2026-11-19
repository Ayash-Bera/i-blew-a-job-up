# I Blew a Job Up

Fresher jobs and internships in India, with original source links, duplicate handling, role and city tags, and a daily refresh pipeline.

Repository: https://github.com/Ayash-Bera/i-blew-a-job-up

Live tool: https://i-blew-a-job-up-ashen.vercel.app

## Status

The verified production export on November 19, 2026, at 12:39:58 PM IST contains 240 unique India fresher and internship posts across 22 employers and 23 configured sources. All sources completed their latest refresh successfully. Role and city tags use prompt v3 and pass output validation. The app stores observations, tags and refresh history in a dedicated Neon Free Postgres database.

Refresh `e653bbe91819fb3e0f5eb4e6` completed on November 19 at 12:39:14 PM IST with 240 eligible and published posts, 240 cached results, zero errors and zero additional model cost. The two physical hardware internships previously assigned software roles now have the `Other` tag. Eligibility checks also reject explicit fresher exclusions and experience requirements with domain qualifiers.

An actual scheduled refresh completed on November 19 at 6:26:14 AM IST on the earlier deployment. The hosting request record identifies `vercel-cron/1.0`, returned HTTP 200, and matches persisted run `1a6b7b55b3f9eb1d09c45b58`. That historical run published 221 posts with 221 cached results, zero errors and zero model cost. The current v3 deployment has passed a manual refresh and cache verification; its next scheduled invocation remains to be observed.

The v3 revision used $0.1345335 of recorded list-price tagging: $0.58748253 per 1,000 attempted versions, or $0.56055625 per 1,000 posts in the final 240-post snapshot. This includes all three rejected outputs and their retries. Billing and hosting are separate. The interface's initial-collection cost remains the historical v2 result, $0.49966833 per 1,000 posts in its 221-post cohort. See [EVALUATION.md](EVALUATION.md) for accounting methods, provenance and audit commands.

Marriott International contributes 186 of the current 240 posts (77.5%). This concentration must accompany any evaluation result. Seventeen posts have a known expiry before November 22 at 11:59 PM IST, and 132 have no supplied expiry. Recheck availability before submission.

Independent source checks of 50 posts matched 50 roles and 47 exact locations. Three location disagreements remain documented for human review. The requested human review, applicant video and forms remain pending; no human accuracy result is claimed. The assignment is not ready for submission. See [SETUP.md](SETUP.md) for the verified runtime configuration.

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
