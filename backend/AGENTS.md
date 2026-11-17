# Backend context

- Wire production processes in `cmd/api`, `cmd/worker` and `cmd/migrate`. The optional development command `DEMO_MODE=true go run ./cmd/demo` uses `DATABASE_URL` to create three labelled draft chapters.
- HTTP contract: `api/openapi.yaml`. Change it before client generation. `learning` owns route registration, authorization and product workflows.
- Adapters: `mailer`, `objectstore`, `billing`, `dialog`, `voice`. `correction` verifies automatic results; `worker` is transport-neutral, `queue` is the PostgreSQL delivery adapter. Wiring lives in `cmd/`.
- From this directory: `go test -race ./...`, `go vet ./...`, `go build ./cmd/...`; integration: `TEST_DATABASE_URL=... go test -race -tags=integration ./...` against an isolated test database.
- HTTP liveness must not depend on PostgreSQL. Readiness must check both connectivity and the required schema. Never expose dependency error details to callers.
- Propagate `context.Context` through all I/O. Bound database pools and HTTP durations. Gracefully stop on SIGTERM.
- Queue ownership is a database lease with a random claim token. Every completion/retry checks that token. Expired claims can be reclaimed; jobs have bounded retries.
- At-least-once execution requires idempotency at the side-effect boundary, not just deduplicated enqueueing. Handlers commit effects before delivery acknowledgement; a replay must safely detect existing results.
- Read student materials through `published_content`. Only validated ancestors expose descendants. Editing a published item or changing an attachment returns it to draft. Public articles use the separately authored public correction.
- Read correction progress through `submission_tracking`. The database derives each stage from the submission, queue state and current lease. All reads enforce owner or teacher access.
- `/v1/me/access` reports the same entitlement checks enforced by routes. Keep locked features visible in clients; never return premium links merely because the client hides them. Exported personal reviews belong to the account even after its plan expires.
- `billing/checkout.go` checks provider subscriptions and reuses or expires pending Checkout sessions under the shared user lock. `billing/upgrade.go` opens a customer-confirmed change to the existing subscription. Only verified webhooks grant access.
- `grading` defines exact numeric comparison used by both practice and the tutor. Generated mock exercises pass arithmetic verification before they can be saved as teacher drafts.
- Keep external services behind their consumer interfaces. API and workers must share provider, S3 and database configuration. Never bypass email verification in development.
- Trust `X-Real-IP` only behind an ingress that overwrites the header and whose network is explicitly listed in `TRUSTED_PROXY_CIDRS`.
- Migrations are immutable once deployed. Add a new numbered SQL file. The migration command serializes runs with an advisory lock and detects edited migrations.

## Membership and examinations

- Billing plan configuration accepts `tier` values `autonomie` or `suivi`. A missing tier defaults to `autonomie`. Compute access from current, unrefunded entitlements. The member capability response supplies upgrade links only when sales are enabled. Stripe confirms paid upgrades before any webhook grants access.
- Keep support URLs behind the Suivi authorization check. Members can export their own personal teacher feedback after expiry. A teacher supplies external video links and controls privacy settings at the video host.
- Written exam answers retain the teacher's reference and a pending assessment. Only a teacher may assign points within the saved rubric. Automatic points and provisional scores must remain distinguishable from the final grade.
- Annals-based generation sends a candidate list to the model, accepts only selected candidate IDs, and snapshots validated teacher content into a new draft. Shared immutable PDF bytes are deleted only when their final document reference is removed.
