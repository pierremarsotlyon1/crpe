# Development workflow

The root Makefile is the executable command reference. Run commands from the repository root. Docker with Compose is sufficient for the local service stack. Host-side checks additionally require the Go version in `backend/go.mod`, Node 24 and npm; run `npm ci` first. Keep local credentials in the ignored `.env`; copy values from `.env.example` when overrides are needed.

All TypeScript workspaces use 5.9.3 to satisfy the OpenAPI generator's TypeScript 5.x peer requirement. Keep the compiler version consistent across clients; see `DECISIONS.md` before upgrading dependencies outside the tested compatibility range.

## Local stack

```sh
make dev
```

`infra/compose.yaml` defines `postgres`, `s3`, `migrate`, `api`, `worker`, `proxy` and `web`. The migration must finish before application services start. API and worker replicas share PostgreSQL; object storage persists independently of application containers.

| Local service | Address                 |
| ------------- | ----------------------- |
| Web           | `http://localhost:3000` |
| API proxy     | `http://localhost:8080` |
| PostgreSQL    | `localhost:55433`       |
| S3 endpoint   | `http://localhost:9000` |

Mobile development runs through Expo on the host, because connecting a simulator or physical device requires a reachable development server.

```sh
npm ci
make dev-mobile
```

The mobile app is a foundation screen; authentication and practice flows are pending. Exporting JavaScript bundles does not build or sign a native application.

`EXPO_PUBLIC_API_URL` is a public, device-reachable API origin, never a secret. Use your development machine's LAN address for a physical phone when API features are added; the default Compose binding is loopback-only and must then be adjusted deliberately. `API_INTERNAL_URL` is the server-side web API origin; Compose points it at the proxy. Neither neutral foundation screen requires a successful API call.

The S3 service uses SeaweedFS 4.53 in `weed mini` mode, configured to create the `crpe-development` bucket at startup. Its container port `8333` is published at host port `9000`; no administration console is published. The local service receives `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and `S3_BUCKET`, following the [SeaweedFS startup contract](https://github.com/seaweedfs/seaweedfs#quick-start). This provisions development infrastructure only: the backend storage adapter and upload routes are still pending.

## Verification

| Command            | Purpose                                                                     |
| ------------------ | --------------------------------------------------------------------------- |
| `make check`       | Formatting/lint, unit tests, type checks and application builds             |
| `make generate`    | Regenerate the shared TypeScript contract from backend OpenAPI              |
| `make integration` | Run PostgreSQL-backed tests for migrations and worker coordination          |
| `make scale-smoke` | Start multiple API/worker replicas and verify local routing and shared work |

Integration tests require an isolated test database; do not point them at a database containing valuable data. The scripts create unique `crpe-integration-<pid>` and `crpe-scale-<pid>` Compose projects with dynamically assigned host ports. The scale smoke test checks routing across two API replicas, completion of 60 durable jobs with two worker processes and readiness after an API replica stops. Cleanup removes only the test project's resources and volumes.

Local and GitHub Actions results are separate evidence. Record unavailable tools, failed checks and unverified native or provider integrations in root `STATUS.md`.

## Backend configuration

| Variable             | Default / purpose                                                 |
| -------------------- | ----------------------------------------------------------------- |
| `DATABASE_URL`       | Required PostgreSQL connection string                             |
| `HTTP_ADDR`          | `:8080`, listener inside each API container                       |
| `DATABASE_MAX_CONNS` | `5`, bounded pool per process                                     |
| `SALES_ENABLED`      | `false`, public runtime configuration; no checkout is implemented |

Readiness allows two seconds for a database query and checks that the latest embedded migration is recorded. API termination allows ten seconds to drain requests. Worker claims use a 30-second lease with a 20-second handler deadline and at most five attempts. Keep these budgets consistent when adding slower work; long AI or audio operations will need an explicit lease strategy before implementation.

## Working on one area

- Backend: read `backend/AGENTS.md`, edit the owning package, and run its focused tests before the full check.
- Clients: read their scoped instructions, edit the route or screen, and run the workspace scripts. Use the generated client for API contracts.
- Contract changes: update `backend/api/openapi.yaml`, run `make generate`, and include both source and generated output in the same change.
- Database changes: add a new ordered migration. Do not rewrite a migration that may already have been deployed; preserve compatibility with old instances during rollouts.
- New background work: define its durable payload, idempotency key, lease budget, retry policy and failure state together with tests for retry and interrupted execution.

## Operational limits

Compose is a development environment, not a production deployment. Local credentials and published development ports must not be copied into production. Application replicas require no persistent volume; stateful services do.

Before increasing production replica counts, calculate the database connection budget described in `architecture.md`. Test removal/replacement of an instance as well as adding one. Deployment must not assume that a request returns to the same API instance or that a worker survives until its next job.

Sales remain disabled by default. The waiting-list subscription, provider integrations and user workflows must be implemented before they can be presented as working features. Refer to `PLANS.md` for those milestones.
