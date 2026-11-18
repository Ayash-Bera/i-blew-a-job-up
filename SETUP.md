# Setup

Use Go 1.26, Node.js 22, npm, Python 3, and PostgreSQL 14 or newer. Exact dependency versions are pinned in `backend/go.mod` and `web/package-lock.json`.

## Backend configuration

Copy `backend/.env.example` to `backend/.env` and populate values locally. Never commit environment files or credentials. Configuration is read once at process startup. Restart after changing it.

| Variable | Purpose |
| --- | --- |
| `PORT` | HTTP port, defaults to 8080 |
| `DATABASE_URL` | Pooled application Postgres connection; Supabase transaction pooler uses port 6543 |
| `MIGRATION_DATABASE_URL` | Direct or session-compatible migration connection; Supabase IPv4 session pooler uses port 5432 |
| `SNAPSHOT_PATH` | Explicit read-only state file when running without a database |
| `SOURCE_REGISTRY` | Optional JSON registry path; empty uses the embedded registry |
| `CRON_SECRET` | Random secret of at least 32 characters, used as a Bearer token on the refresh route |
| `ALLOWED_ORIGIN` | Optional exact frontend origin for cross-origin local requests |
| `MODEL_PROVIDER` | `compatible` (default) or `gemini` for the native Gemini adapter |
| `MODEL_URL` | Full HTTPS chat-completions endpoint, or native generateContent endpoint for Gemini |
| `MODEL_NAME` | Exact deployed model identifier |
| `MODEL_API_KEY` | Server-only provider credential |
| `MODEL_INPUT_USD_PER_MILLION` | Verified input-token price; unknown stays empty |
| `MODEL_OUTPUT_USD_PER_MILLION` | Verified output-token price; unknown stays empty |
| `MODEL_CACHED_INPUT_USD_PER_MILLION` | Verified cached-input price; required for a known cost when Gemini reports cached tokens |
| `MODEL_MIN_INTERVAL_MS` | Minimum request-start interval shared by workers in this process, defaults to 0; 0 to 3600000 |
| `REFRESH_TIMEOUT_SECONDS` | Ingestion timeout, defaults to 240; keep below hosting request limits |
| `MAX_TAG_POSTS` | Maximum new post versions sent to the model in a run, defaults to 500 |
| `TAG_CONCURRENCY` | Concurrent model calls, defaults to 4; reduce for provider rate limits |

Migrations run automatically on startup. They use a separate session connection because migration advisory locks require session affinity. Application queries use pgx simple protocol, compatible with transaction pooling. The schema is private, with restricted access and row policies. Use a dedicated database for this application.

For a Vercel-managed Neon database, retain its pooled `DATABASE_URL` and save its direct `DATABASE_URL_UNPOOLED` value separately as the Production Secret `MIGRATION_DATABASE_URL`. Keep TLS enabled on both connections. The current hosting account uses a dedicated Neon Free database because its Supabase free-project allowance is exhausted.

For local development, point both database variables at a throwaway local Postgres database. Run `go run ./cmd/server` from `backend/`. Run `go run ./cmd/ingest` to collect and tag from the configured sources. Missing model credentials leave records pending rather than creating pretend predictions.

To import a previously collected source response, use `go run ./cmd/ingest -input /path/to/source-results.json`. The import preserves original check timestamps; replaying a file does not prove a new source check.

To make a read-only preview, use `go run ./cmd/ingest -input /path/to/source-results.json -snapshot-out /path/to/snapshot.json`. Set `SNAPSHOT_PATH` and leave database variables empty. This mode cannot perform scheduled updates and is not a completed submission.

`SNAPSHOT_PATH=embedded` explicitly selects the public dataset bundled into the executable. Replace `backend/internal/db/bundled-snapshot.json` and rebuild to update that preview. Remove raw observations from the bundled file. To retain the actual collected responses before normalization, pass `-source-results-out /private/path/source-results.json` to the ingestion command.

### Gemini configuration

The live deployment tested these server-side values with Gemini 3.5 Flash Lite:

```dotenv
MODEL_PROVIDER=gemini
MODEL_NAME=gemini-3.5-flash-lite
MODEL_URL=https://generativelanguage.googleapis.com/v1beta/models/gemini-3.5-flash-lite:generateContent
MODEL_INPUT_USD_PER_MILLION=0.30
MODEL_OUTPUT_USD_PER_MILLION=2.50
MODEL_CACHED_INPUT_USD_PER_MILLION=0.03
MODEL_MIN_INTERVAL_MS=500
TAG_CONCURRENCY=4
MAX_TAG_POSTS=200
```

Set `MODEL_API_KEY` as a Production Secret in the hosting project's environment-variable form or through a private local environment file. Never put it in a URL, browser configuration, source file, or command argument. The adapter sends it in the `x-goog-api-key` header. Restart or redeploy after changing environment values.

The native request uses JSON schema output, minimal thinking, and a 2048-token output cap. It accepts only complete responses and validates source evidence before caching tags. The adapter omits the deprecated temperature setting. See the [model configuration notes](https://aistudio.google.com/docs/whats-new-gemini-3.6) and [structured output documentation](https://aistudio.google.com/docs/structured-output).

Set `MODEL_MIN_INTERVAL_MS` from the project's actual request quota, allowing headroom for retries and other consumers. For a new provider project, begin with a smaller pilot limit, check run records and quotas, and then raise the limit within the hosting timeout. Quotas apply per project, including other applications using the same project. Local pacing covers this process; it is not a project-wide limiter. Retryable responses honor `Retry-After` and Gemini retry-delay metadata. See the [quota documentation](https://aistudio.google.com/docs/rate-limits).

This project's tier and the rates above were verified before the v2 first load. For another project, leave pricing variables empty until its applicable rates are verified. The usage ledger records input tokens, cached input tokens, thinking tokens, and billable output tokens (candidate plus thinking tokens). Cached input is already included in total input. Missing usage or required rates leaves cost unknown. HTTP errors without usage also remain unknown, including retries. Do not replace unknown cost with zero or describe list-price estimates as actual charges. Native usage fields are documented in the [response reference](https://aistudio.google.com/api-reference/generate-content).

### Verified hosted configuration

Vercel project `i-blew-a-job-up` belongs to `ayash-beras-projects`; the CLI account is `ayash-bera`. Deployment `dpl_DawXdHkmoL2Dt3QN6R5EvG7woPZi` serves https://i-blew-a-job-up-ashen.vercel.app. A dedicated Neon Free database in `iad1` provides the pooled application URL and a separate direct migration URL. Credentials remain Production Secrets.

On November 18, 2026, 10:25:42 PM IST, the public API exposed 221 validated tagged posts. A repeated authenticated refresh completed with 221 cache hits, zero provider attempts, and zero errors. The deployed cron target and daily definition match this deployment. Actual scheduled-run evidence and the human review remain pending.

## Frontend

From `web/`, run `npm ci`. Set `API_URL=http://127.0.0.1:8080` in `web/.env.local`, then run `npm run dev`. The public interface does not require an account. Browser requests use one API client, and the local Next.js proxy accepts only public read endpoints.

## Vercel deployment

The root `vercel.json` defines Go and Next.js services. The public API is routed directly to Go, so the local proxy's `API_URL` is unnecessary in this configuration. The backend's own configuration also supports a separate Go project if needed.

Use the Vercel CLI to link the intended account and project, configure server-side environment values, and deploy. Use the free database plan only when its limits meet the workload. Do not include `.env`, private snapshots, downloaded descriptions, or test fixtures in the deployment upload. Review the dry-run file list first.

After deployment, verify the public site while signed out, call the public API checks, and trigger one authorized refresh. Confirm that the same unchanged data produces cache hits with no additional tagging requests. Keep the first and second actual scheduled run records; a manual call is not evidence of a scheduled execution.

The UTC cron expression is `30 0 * * *`, around 6:00 AM IST. The Hobby tier may invoke it within the corresponding hourly window. The route requires `Authorization: Bearer <CRON_SECRET>`. The database lease prevents overlap, including overlap between manual and scheduled runs.

If the model rejects requests, inspect the status and credential configuration without logging secrets. If a source returns a challenge or rate limit, stop that source and preserve its error status. Repeated retries must not turn an incomplete fetch into an empty successful result.
