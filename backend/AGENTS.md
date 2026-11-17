# Backend context

- Entrypoints: `cmd/api`, `cmd/worker`, `cmd/migrate`. Wiring lives there; reusable logic lives in `internal/`.
- HTTP contract: `api/openapi.yaml`. Change it before client generation; health and configuration are the only public endpoints in J0.
- Packages: `config` parses immutable environment configuration; `database` opens a bounded pool; `httpapi` handles HTTP; `queue` implements durable leases and the internal probe task; `migrations` embeds ordered SQL.
- From this directory: `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`; integration: `TEST_DATABASE_URL=... go test -race -tags=integration ./...` against an isolated test database.
- HTTP liveness must not depend on PostgreSQL. Readiness must check both connectivity and the required schema. Never expose dependency error details to callers.
- Propagate `context.Context` through all I/O. Bound database pools and HTTP durations. Gracefully stop on SIGTERM.
- Queue ownership is a database lease with a random claim token. Every completion/retry checks that token. Expired claims can be reclaimed; jobs have bounded retries.
- At-least-once execution requires idempotency at the side-effect boundary, not just deduplicated enqueueing. The probe records its effect and completes its job in one transaction.
- Keep future payment/email/AI/storage adapters behind narrow consumer-owned interfaces. Do not add empty adapters or production in-memory stores.
- Migrations are immutable once deployed. Add a new numbered SQL file. The migration command serializes runs with an advisory lock and detects edited migrations.
