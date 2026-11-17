# Task-oriented context map

Read root `AGENTS.md`, `PLANS.md`, then the scoped `AGENTS.md` for the area you change. Use this index to locate a narrow slice; do not read every directory before making a change.

| Task                               | Start with                                    | Follow only if needed                                                                     |
| ---------------------------------- | --------------------------------------------- | ----------------------------------------------------------------------------------------- |
| Backend process lifecycle          | `backend/cmd/api/`, `backend/cmd/worker/`     | `backend/internal/config/`, `backend/internal/database/`, `docs/architecture.md`          |
| API route or contract              | `backend/api/openapi.yaml`                    | `backend/internal/httpapi/` and adjacent tests; `packages/api-client/`                    |
| PostgreSQL schema                  | `backend/migrations/`, `backend/cmd/migrate/` | `backend/internal/database/` and migration integration tests                              |
| Durable work and worker recovery   | `backend/internal/queue/`                     | Queue integration tests, `backend/cmd/worker/`                                            |
| Public web page                    | `web/src/app/page.tsx`                        | `web/src/app/layout.tsx`, `globals.css`, `web/src/lib/api.ts`                             |
| Mobile screen                      | `mobile/index.ts`, `mobile/src/App.tsx`       | `mobile/src/api.ts`, `mobile/app.json`                                                    |
| Shared client                      | `packages/api-client/src/index.ts`            | `backend/api/openapi.yaml`, generated `packages/api-client/src/schema.ts`, adjacent tests |
| Local services and replica scaling | `Makefile`, `infra/AGENTS.md`                 | `infra/compose.yaml`, `infra/nginx.conf`, `scripts/`, `docs/development.md`               |
| CI failure                         | `.github/workflows/ci.yaml`                   | The exact Make target or workspace script that failed                                     |
| Product scope or assumption        | `PLANS.md`, `DECISIONS.md`                    | `STATUS.md`, relevant architecture section                                                |

## Context discipline

1. Search filenames with `rg --files`; search symbols in the owning directory with `rg`.
2. Read the interface, the implementation and adjacent meaningful tests. Expand outward only when dependencies require it.
3. OpenAPI is the API source of truth. Regenerate the client after changing it; never patch generated types manually.
4. Skip lockfiles, `node_modules/`, build output and generated schemas unless investigating dependency resolution or generation itself.
5. Keep new instructions close to their scope. Root instructions cover shared invariants; scoped files describe local commands and entry points.
6. Close a task by updating the smallest relevant document and reporting commands actually run. Do not duplicate progress narratives across every document.

The roadmap is not a list of implemented features. Use `STATUS.md` to determine the current delivery boundary.
