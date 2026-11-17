# Delivery status — 2026-11-17

The repository/runtime foundation is implemented and verified locally. The attached
product roadmap is not complete: J1–J10 remain pending. No production deployment,
remote CI run or native store build is claimed.

## Implemented

- Root-level Go backend, Next.js web, Expo mobile, shared API client, infrastructure,
  documentation and developer scripts.
- Concise root and scoped agent instructions, a task-oriented context map,
  architecture decisions and an explicit milestone plan.
- Disposable API and worker containers, read-only backend root filesystems,
  bounded database pools, graceful process shutdown and JSON logs.
- Liveness, database/schema readiness, public runtime configuration and OpenAPI.
  Client types are generated and checked for drift.
- PostgreSQL migrations serialized by an advisory lock, with recorded checksums.
- Durable PostgreSQL job claims, expiry recovery, ownership fencing, retry limits
  and transactional/idempotent probe effects. The probe is infrastructure-only;
  no email, AI or payment processing is simulated.
- Local reverse proxy with dynamic service discovery, PostgreSQL and S3-compatible
  SeaweedFS with the `crpe-development` bucket.
- Neutral French web/mobile screens. Sales default to disabled; no registration
  or purchase interface is presented as functional.

## Observed verification

| Check              | Result                                                                                                                                                                       |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `make check`       | Passed: Go formatting/vet/race tests/build, Biome, OpenAPI validation, TypeScript, generated drift, client tests, Next.js build, Expo exports                                |
| `make integration` | Passed against isolated PostgreSQL 18.7: concurrent claims/migrations, delayed retry, exhausted retries, expired and replaced leases, idempotent effects, database readiness |
| `make scale-smoke` | Passed: two API replicas reached through proxy, two worker processes, 60 durable job effects, recovery after stopping one API                                                |
| `make dev`         | Passed from absent `.env`: images built, migration completed, all services started, health-checked services healthy                                                          |
| S3 signed request  | Passed: local bucket exists and can be listed with development credentials                                                                                                   |
| Browser check      | Desktop page rendered correctly from the standalone Docker image                                                                                                             |
| `git diff --check` | Passed                                                                                                                                                                       |

Checks used Go 1.27.4 and Node 24.22.3. Docker also verified dependency installation
from the lockfile in Linux. Expo exported both native-platform JavaScript bundles;
these are **not** signed native binaries. Mobile device execution, native builds,
mobile browser layout, Lighthouse, accessibility audit and production load testing
have not been performed.

The first immediate post-shutdown proxy check returned a transient 504. Docker DNS
discovery converges asynchronously; the final smoke test verifies bounded recovery
followed by repeated successful requests. It does not prove zero-downtime failover.
Production readiness probes, ingress draining and availability testing remain needed.

## Pending product work

Authentication and roles, shared sessions, educational schema and admin workflows,
double opt-in emails, consent/export/deletion, member course and progress,
teacher-authored content, AI correction, quotas, payment, offline practice,
notifications, mock exams and oral practice remain in `PLANS.md`.
Provider interfaces and adapters should be introduced alongside their first real
feature; there are no empty adapters claiming those integrations are complete.

## Owner actions for later release

1. Choose the product name and domain; supply approved teaching content.
2. Publish the development branch to GitHub and observe CI before merging.
3. Choose EU hosting, PostgreSQL and S3 providers; configure production credentials,
   TLS, DNS, backups, restoration checks and operational monitoring.
4. Supply email/domain credentials, then payment test keys and webhook secrets when
   implementing those features. Configure approved AI/audio/push providers as needed.
5. Complete business/legal copy and required product approvals before enabling sales.
6. Configure Apple/Google developer accounts, application IDs and signing credentials.
   Build and test on devices. Before store submission, verify the then-current rules
   for account access, external purchase links, subscriptions, account deletion,
   privacy disclosures, photo/audio permissions and AI-generated feedback.

No external accounts, keys or DNS changes are required to run the current local
foundation. Development services are running at `localhost:3000` (web),
`localhost:8080` (API) and `localhost:9000` (S3); `make down` stops them without
removing their data. Changes are on `codex/stateless-foundation`; no remote push
or deployment was performed.
