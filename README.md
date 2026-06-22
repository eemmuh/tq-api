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

On startup, any jobs left in `processing` state are reset to `queued` and re-enqueued. Queued jobs from a previous run are also picked up automatically.

## API

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

List all jobs, newest first.

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
  ]
}
```

## Job status lifecycle

| Status | Meaning |
|--------|---------|
| `queued` | Accepted and waiting for a worker |
| `processing` | Currently running |
| `completed` | Finished successfully; see `result` |
| `failed` | Finished with an error; see `error` |

Poll `GET /jobs/{id}` until `status` is `completed` or `failed`.

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

List all jobs:

```bash
curl -s http://localhost:8080/jobs
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
