# Execution plan

The attached mission describes the product roadmap. This change establishes the foundation requested in chat: root-level separation, a stateless horizontally scalable backend, and concise agent context. Later product milestones remain pending.

## J0 — Repository and runtime foundation (verified locally)

- [x] Root applications and shared contract separated.
- [x] Scoped agent instructions, context map, architecture decisions.
- [x] Go API and separate durable PostgreSQL worker, explicit migrations.
- [x] Next.js and Expo foundations with generated shared API contract.
- [x] Local Docker orchestration and GitHub Actions workflow defined.
- [x] Verification: `make check`, `make integration`, `make scale-smoke`, `make dev`.
- [x] Report observed results and remaining prerequisites in `STATUS.md`.
- [ ] Observe a green GitHub Actions run after the branch is published. No remote CI result is claimed.

## Product roadmap (not implemented)

| Milestone | Scope                                                            | Acceptance evidence to add with implementation                                  |
| --------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| J1        | Auth, roles, educational schema, consent and access              | PostgreSQL integration tests for cross-replica authentication and authorization |
| J2        | Double opt-in waiting list, legal templates, validated SEO pages | Browser signup test and mobile Lighthouse >= 90                                 |
| J3        | Teacher editor, imports and validation                           | Browser test creating and validating a complete chapter                         |
| J4        | Student course, diagnostic and revision plan                     | Browser test for the full student journey                                       |
| J5        | AI correction and deterministic checks                           | 30-case local fixture suite plus documented provider test                       |
| J6        | Stripe and entitlements, behind sales flag                       | Stripe test-mode webhook, idempotency and access tests                          |
| J7        | Mobile practice, spaced repetition, offline sync                 | iOS/Android development builds and scheduling tests                             |
| J8        | Validated mock examinations                                      | Full examination test and draft-access denial tests                             |
| J9        | Oral practice, transcription and speech                          | Full local session plus documented provider test                                |
| J10       | Security, accessibility and operations                           | Fresh-clone checks and deployment runbook                                       |

Do not mark a milestone complete when only its scaffolding exists. Introduce executable acceptance commands as each feature is implemented.
