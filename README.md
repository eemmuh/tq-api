# task-queue-api

A small HTTP API for submitting background jobs and polling their status. Jobs are persisted in SQLite and processed by a configurable worker pool.

Supported job types: `sleep`, `hash`, and `fetch` (HTTP GET).

## Requirements

- Go 1.25 or later

## Quick start

```bash
go run ./cmd/server
```

The server listens on `:8080` by default and writes jobs to `jobs.db` in the current directory.

```bash
curl http://localhost:8080/health
```

## Server flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:8080` | HTTP listen address |
| `-workers` | `2` | Number of background workers |
| `-db` | `jobs.db` | SQLite database path |

Example:

```bash
go run ./cmd/server -addr :3000 -workers 4 -db /tmp/jobs.db
```

On startup, any jobs left in `processing` state are reset to `queued` and re-enqueued. Queued jobs from a previous run are also picked up automatically. Jobs waiting for a retry backoff are re-scheduled as their `next_retry_at` time arrives.

## Retries

Transient failures are retried automatically with exponential backoff:

- **Max attempts:** 3 per job (configurable in code via `job.DefaultMaxAttempts`)
- **Backoff:** 1s, 2s, 4s, … capped at 30s
- **Retryable errors:** network failures on `fetch` jobs (connection errors, timeouts, read failures)
- **Permanent errors:** validation failures (bad payload, unknown job type) are not retried

While waiting to retry, a job stays `queued` with `next_retry_at` set and the last error in `error`. Poll `GET /jobs/{id}` to see `attempts`, `max_attempts`, and `next_retry_at`.

Example job mid-retry:

```json
{
  "id": "...",
  "type": "fetch",
  "payload": {"url": "https://example.com"},
  "status": "queued",
  "error": "fetch request failed: connection refused",
  "attempts": 1,
  "max_attempts": 3,
  "next_retry_at": "2026-06-22T12:00:01.123456789Z",
  "created_at": "2026-06-22T12:00:00.123456789Z"
}
```


All JSON request and response bodies use `Content-Type: application/json`.

Errors return a JSON object:

```json
{"error": "message"}
```

### `GET /health`

Health check.

**Response `200 OK`**

```json
{"status": "ok"}
```

### `POST /jobs`

Submit a new job. The job is stored and queued for background execution.

**Request body**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Job type: `sleep`, `hash`, or `fetch` |
| `payload` | object | no | Type-specific parameters (defaults to `{}`) |

**Response `202 Accepted`**

Returns the created job. Initial `status` is always `queued`.

```json
{
  "id": "a1b2c3d4e5f6789012345678901234ab",
  "type": "hash",
  "payload": {"text": "hello"},
  "status": "queued",
  "attempts": 0,
  "max_attempts": 3,
  "created_at": "2026-06-22T12:00:00.123456789Z"
}
```

**Errors**

| Status | When |
|--------|------|
| `400` | Invalid JSON, or missing `type` |
| `500` | Failed to create job |

### `GET /jobs/{id}`

Get a single job by ID, including result or error when finished.

**Response `200 OK`**

```json
{
  "id": "a1b2c3d4e5f6789012345678901234ab",
  "type": "hash",
  "payload": {"text": "hello"},
  "status": "completed",
  "result": {
    "algorithm": "sha256",
    "digest": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
  },
  "created_at": "2026-06-22T12:00:00.123456789Z",
  "started_at": "2026-06-22T12:00:00.234567890Z",
  "finished_at": "2026-06-22T12:00:00.345678901Z"
}
```

**Errors**

| Status | When |
|--------|------|
| `404` | Job not found |
| `500` | Failed to load job |

### `GET /jobs`

List jobs, newest first. Results are paginated.

**Query parameters**

| Parameter | Default | Description |
|-----------|---------|-------------|
| `limit` | `50` | Page size (1–100) |
| `offset` | `0` | Number of jobs to skip |
| `status` | — | Filter by status: `queued`, `processing`, `completed`, or `failed` |
| `type` | — | Filter by job type: `sleep`, `hash`, or `fetch` |

**Response `200 OK`**

```json
{
  "jobs": [
    {
      "id": "...",
      "type": "fetch",
      "payload": {"url": "https://example.com"},
      "status": "completed",
      "result": {"status_code": 200, "bytes": 1256},
      "created_at": "...",
      "started_at": "...",
      "finished_at": "..."
    }
  ],
  "total": 42,
  "limit": 50,
  "offset": 0
}
```

`total` is the number of matching jobs across all pages (after filters, before pagination).

**Errors**

| Status | When |
|--------|------|
| `400` | Invalid `limit`, `offset`, or `status` |
| `500` | Failed to list jobs |

## Job status lifecycle

| Status | Meaning |
|--------|---------|
| `queued` | Accepted and waiting for a worker (including backoff before a retry) |
| `processing` | Currently running |
| `completed` | Finished successfully; see `result` |
| `failed` | Exhausted retries or hit a permanent error; see `error` |

Poll `GET /jobs/{id}` until `status` is `completed` or `failed`. A job may return to `queued` between attempts while waiting for retry backoff.

## Job types

### `sleep`

Waits for a number of seconds, then completes.

**Payload**

| Field | Type | Default | Constraints |
|-------|------|---------|-------------|
| `seconds` | number | `1` | Must be `> 0` and `<= 30` |

**Result**

```json
{"slept_seconds": 2}
```

### `hash`

Computes a SHA-256 digest of a string.

**Payload**

| Field | Type | Required |
|-------|------|----------|
| `text` | string | yes (non-empty) |

**Result**

```json
{
  "algorithm": "sha256",
  "digest": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
}
```

### `fetch`

Performs an HTTP GET request to a URL.

**Payload**

| Field | Type | Required |
|-------|------|----------|
| `url` | string | yes (non-empty, `http` or `https` only) |

**Result**

```json
{
  "url": "https://example.com",
  "status_code": 200,
  "bytes": 1256,
  "truncated": false,
  "content_type": "text/html; charset=UTF-8",
  "body_preview": "<!doctype html>..."
}
```

Response bodies are read up to 64 KiB. The `body_preview` field contains at most 512 bytes. If the body exceeds 64 KiB, `truncated` is `true`.

Fetch requests time out after 30 seconds.

## Example `curl` commands

Health check:

```bash
curl -s http://localhost:8080/health
```

Submit a hash job:

```bash
curl -s -X POST http://localhost:8080/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type":"hash","payload":{"text":"hello"}}'
```

Submit a sleep job:

```bash
curl -s -X POST http://localhost:8080/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type":"sleep","payload":{"seconds":2}}'
```

Submit a fetch job:

```bash
curl -s -X POST http://localhost:8080/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type":"fetch","payload":{"url":"https://example.com"}}'
```

Poll job status (replace `{id}` with the job ID from the create response):

```bash
curl -s http://localhost:8080/jobs/{id}
```

List jobs (first page, default limit 50):

```bash
curl -s http://localhost:8080/jobs
```

List completed fetch jobs with pagination:

```bash
curl -s 'http://localhost:8080/jobs?status=completed&type=fetch&limit=10&offset=0'
```

Submit and poll until complete:

```bash
ID=$(curl -s -X POST http://localhost:8080/jobs \
  -H 'Content-Type: application/json' \
  -d '{"type":"hash","payload":{"text":"hello"}}' \
  | jq -r .id)

until STATUS=$(curl -s "http://localhost:8080/jobs/$ID" | jq -r .status) \
  && [ "$STATUS" = "completed" -o "$STATUS" = "failed" ]; do
  sleep 0.1
done

curl -s "http://localhost:8080/jobs/$ID" | jq
```

## Development

Run tests:

```bash
go test ./...
```

CI runs `go test ./...` on every push and pull request to `main` via GitHub Actions (`.github/workflows/ci.yml`).

Build a binary:

```bash
go build -o server ./cmd/server
```

SQLite database files (`*.db`, `*.db-wal`, `*.db-shm`) are gitignored.

## Project layout

```
cmd/server/          HTTP server entrypoint
internal/api/        REST handlers
internal/job/        Job model and SQLite store
internal/worker/     Worker pool and job executors
```
