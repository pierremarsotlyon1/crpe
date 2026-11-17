# Prépa Maths CRPE

Foundation for a French mathematics preparation platform: Go API and workers,
Next.js web, Expo mobile, and one shared OpenAPI client. Product features are tracked
in [PLANS.md](PLANS.md); the current implementation is the runtime foundation.

## Repository

| Directory              | Responsibility                                                 |
| ---------------------- | -------------------------------------------------------------- |
| `backend/`             | Stateless Go API, independent workers, SQL migrations, OpenAPI |
| `web/`                 | Next.js public site; future member and teacher interfaces      |
| `mobile/`              | Expo iOS/Android application                                   |
| `packages/api-client/` | Generated API types and shared typed HTTP client               |
| `infra/`               | Local PostgreSQL, S3, proxy and application containers         |
| `docs/`                | Architecture, context map and development guide                |
| `scripts/`             | Repeatable checks and isolated integration environments        |

## Run locally

With Docker and Docker Compose installed:

```sh
make dev
```

This creates `.env` from `.env.example` if missing, builds the applications, applies
migrations once, and starts the local stack. Open [the website](http://localhost:3000)
or [the API readiness endpoint](http://localhost:8080/health/ready).
`make down` stops the stack and preserves development data.

For host development and checks, install Node.js 24 LTS, npm 11+, and the Go version
declared in `backend/go.mod`:

```sh
make install
make check
make integration
make scale-smoke
make dev-mobile
```

The mobile command starts Expo; a device or simulator is required to run the app.
The automated mobile build exports iOS/Android JavaScript bundles; it does not
produce signed native binaries. See [development setup](docs/development.md).

## Scaling and agent workflow

The API and worker containers are disposable and have read-only root filesystems.
PostgreSQL owns the queue and shared state. Files will use the S3 service when
uploads are implemented. API replicas sit behind a proxy without session affinity.
`make scale-smoke` exercises two APIs and two workers in an isolated environment.
See [architecture and limits](docs/architecture.md), including the database connection budget.

Agents start with [AGENTS.md](AGENTS.md) and [the context map](docs/context-map.md),
then read only the relevant scoped instructions. The API schema is the source of
truth; `make generate` regenerates client types and CI checks for drift.

Sales are disabled by default. No registration, authentication, teaching content,
payment or AI correction is presented as implemented. [STATUS.md](STATUS.md)
records what was actually verified and what remains.
