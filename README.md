# URL Shortener + Link Tracking API (Go)

Production-ready URL shortener service with analytics, Redis caching, async click persistence, rate limiting, and graceful shutdown.

## Stack
- Go (`net/http` + Chi router)
- PostgreSQL
- Redis
- Docker + Docker Compose
- Structured logging with Zap

## Project Structure
```
url-shortener-tracking-api/
├── cmd/server/                 # Application entrypoint
├── internal/
│   ├── cache/                  # Redis cache + click queue implementation
│   ├── config/                 # Env config parsing
│   ├── handlers/               # HTTP handlers/routes
│   ├── logger/                 # Zap logger setup
│   ├── middleware/             # Logging + rate limiter middleware
│   ├── models/                 # Domain models and DTOs
│   ├── repositories/           # Postgres data access
│   ├── services/               # Business logic + click flush worker
│   └── utils/                  # URL validation and short code generation
├── migrations/                 # SQL schema
├── Dockerfile
├── docker-compose.yml
└── .env.example
```

## Features
- `POST /shorten` to create short URLs
- `GET /{short_code}` redirect with click tracking
- `GET /stats/{short_code}` analytics
- `GET /health` service health check
- Duplicate URL dedupe support (same URL returns existing short code)
- Redis cache for `short_code -> original_url`
- Redis-backed click queue + periodic write-behind flush to Postgres
- Redis fixed-window rate limiting middleware
- Graceful shutdown with final click flush

## Database Schema
### `urls`
- `id UUID PK`
- `original_url TEXT UNIQUE`
- `short_code VARCHAR(16) UNIQUE`
- `created_at TIMESTAMPTZ`

### `clicks`
- `id UUID PK`
- `url_id UUID FK -> urls.id`
- `accessed_at TIMESTAMPTZ`

## Configuration
Set via environment variables:
- `DB_URL`
- `REDIS_URL`
- `BASE_URL`

Additional useful configs:
- `PORT`
- `CACHE_TTL`
- `CODE_LENGTH`
- `RATE_LIMIT_REQUESTS`
- `RATE_LIMIT_WINDOW`
- `CLICK_FLUSH_INTERVAL`
- `CLICK_FLUSH_BATCH_SIZE`
- `HTTP_READ_TIMEOUT`
- `HTTP_WRITE_TIMEOUT`
- `HTTP_IDLE_TIMEOUT`
- `SHUTDOWN_TIMEOUT`

Copy `.env.example` to `.env` and adjust values.

## Run with Docker Compose
```bash
cp .env.example .env
docker compose up --build
```

Service: `http://localhost:8080`

## API Examples
### 1) Shorten URL
```bash
curl -X POST http://localhost:8080/shorten \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'
```

With custom code:
```bash
curl -X POST http://localhost:8080/shorten \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com","custom_code":"my-link"}'
```

With expiry (seconds):
```bash
curl -X POST http://localhost:8080/shorten \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com","expires_in_seconds":3600}'
```

Example response:
```json
{
  "short_code": "abc123",
  "short_url": "http://localhost:8080/abc123"
}
```

### 2) Redirect
```bash
curl -i http://localhost:8080/abc123
```

Returns `302 Found` with `Location: https://example.com`.
If the link is expired, returns `410 Gone`.

### 3) Stats
```bash
curl http://localhost:8080/stats/abc123
```

Example response:
```json
{
  "url": "https://example.com",
  "total_clicks": 123,
  "created_at": "2026-04-09T10:00:00Z",
  "last_accessed_at": "2026-04-09T10:15:10Z"
}
```

### 4) Health
```bash
curl http://localhost:8080/health
```

## Running Tests
```bash
go test ./...
```

Includes basic service-layer unit tests under `internal/services`.

## Architecture Notes
- Handlers only deal with HTTP concerns (request parsing, status codes, responses).
- Services contain business logic, including:
  - URL shortening + dedupe
  - Redirect resolution + click tracking
  - Analytics aggregation from Postgres + pending Redis counters
  - Background flush of queued click events to Postgres
- Repositories isolate SQL and persistence details.
- Cache layer centralizes Redis concerns (URL cache, pending counters, queue, last-access timestamps).

## GCP Deployment Notes
This service is container-ready and can run on Cloud Run / GKE.
- Build and push container image via Cloud Build or Artifact Registry.
- Set env vars (`DB_URL`, `REDIS_URL`, `BASE_URL`) in the runtime environment.
- Use Cloud SQL Postgres and Memorystore Redis for managed dependencies.
- Ensure network connectivity (VPC connector/private IP) from compute to DB/Redis.
