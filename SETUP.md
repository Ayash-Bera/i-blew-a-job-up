# Setup

Use Go 1.26, Node.js 22, npm, Python 3, and PostgreSQL 14 or newer. Exact dependency versions are pinned in `backend/go.mod` and `web/package-lock.json`.

## Backend configuration

Copy `backend/.env.example` to `backend/.env` and populate values locally. Never commit environment files or credentials. Configuration is read once at process startup. Restart after changing it.

| Variable | Purpose |
| --- | --- |
| `PORT` | HTTP port, defaults to 8080 |
| `DATABASE_URL` | Application Postgres connection; use the Supabase transaction pooler on port 6543 in production |
| `MIGRATION_DATABASE_URL` | Session-compatible migration connection; Supabase IPv4 session pooler on port 5432 |
| `SNAPSHOT_PATH` | Explicit read-only state file when running without a database |
| `SOURCE_REGISTRY` | Optional JSON registry path; empty uses the embedded registry |
| `CRON_SECRET` | Random secret of at least 32 characters, used as a Bearer token on the refresh route |
| `ALLOWED_ORIGIN` | Optional exact frontend origin for cross-origin local requests |
| `MODEL_URL` | Full HTTPS chat-completions endpoint |
| `MODEL_NAME` | Exact deployed model identifier |
| `MODEL_API_KEY` | Server-only provider credential |
| `MODEL_INPUT_USD_PER_MILLION` | Verified input-token price; unknown stays empty |
| `MODEL_OUTPUT_USD_PER_MILLION` | Verified output-token price; unknown stays empty |
| `REFRESH_TIMEOUT_SECONDS` | Ingestion timeout, defaults to 240; keep below hosting request limits |
| `MAX_TAG_POSTS` | Maximum new post versions sent to the model in a run, defaults to 500 |
| `TAG_CONCURRENCY` | Concurrent model calls, defaults to 4; reduce for provider rate limits |

Migrations run automatically on startup. They use a separate session connection because migration advisory locks require session affinity. Application queries use pgx simple protocol, compatible with transaction pooling. The schema is private, with restricted access and row policies. Use a dedicated database for this application.

For local development, point both database variables at a throwaway local Postgres database. Run `go run ./cmd/server` from `backend/`. Run `go run ./cmd/ingest` to collect and tag from the configured sources. Missing model credentials leave records pending rather than creating pretend predictions.

To import a previously collected source response, use `go run ./cmd/ingest -input /path/to/source-results.json`. The import preserves original check timestamps; replaying a file does not prove a new source check.

To make a read-only preview, use `go run ./cmd/ingest -input /path/to/source-results.json -snapshot-out /path/to/snapshot.json`. Set `SNAPSHOT_PATH` and leave database variables empty. This mode cannot perform scheduled updates and is not a completed submission.

`SNAPSHOT_PATH=embedded` explicitly selects the public dataset bundled into the executable. Replace `backend/internal/db/bundled-snapshot.json` and rebuild to update that preview. Remove raw observations from the bundled file. To retain the actual collected responses before normalization, pass `-source-results-out /private/path/source-results.json` to the ingestion command.

## Frontend

From `web/`, run `npm ci`. Set `API_URL=http://127.0.0.1:8080` in `web/.env.local`, then run `npm run dev`. The public interface does not require an account. Browser requests use one API client, and the local Next.js proxy accepts only public read endpoints.

## Vercel deployment

The root `vercel.json` defines Go and Next.js services. The public API is routed directly to Go, so the local proxy's `API_URL` is unnecessary in this configuration. The backend's own configuration also supports a separate Go project if needed.

Use the Vercel CLI to link the intended account and project, configure server-side environment values, and deploy. Use the free database plan only when its limits meet the workload. Do not include `.env`, private snapshots, downloaded descriptions, or test fixtures in the deployment upload. Review the dry-run file list first.

After deployment, verify the public site while signed out, call the public API checks, and trigger one authorized refresh. Confirm that the same unchanged data produces cache hits with no additional tagging requests. Keep the first and second actual scheduled run records; a manual call is not evidence of a scheduled execution.

The UTC cron expression is `30 0 * * *`, around 6:00 AM IST. The Hobby tier may invoke it within the corresponding hourly window. The route requires `Authorization: Bearer <CRON_SECRET>`. The database lease prevents overlap, including overlap between manual and scheduled runs.

If the model rejects requests, inspect the status and credential configuration without logging secrets. If a source returns a challenge or rate limit, stop that source and preserve its error status. Repeated retries must not turn an incomplete fetch into an empty successful result.
