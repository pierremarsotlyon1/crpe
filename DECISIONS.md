# Decision index

- **Scope:** implement the repository/runtime foundation requested in chat. The attached roadmap remains product reference; no automatic sales activation, publication or third-party account creation.
- **Architecture:** modular Go monolith, explicit constructor injection, independently replicated API and workers. This keeps a solo-maintained codebase small while allowing horizontal process scaling.
- **State:** PostgreSQL for durable/shared state and queue; S3-compatible storage for objects. Future authentication uses shared/revocable sessions rather than instance-local sessions.
- **Repository:** `backend/`, `web/`, `mobile/`, `packages/`, `infra/`, `docs/`, `scripts/` directly at the root. No redundant `apps/` layer.
- **Agent context:** root invariants plus scoped `AGENTS.md`, task-oriented context map, explicit source/generated boundaries and commands. No duplicated rule files for each AI editor.
- **Product name:** use the working name “Prépa Maths CRPE” until branding is chosen.
- **Delivery evidence:** local and CI results are distinct; native bundles are not signed iOS/Android builds. Incomplete features remain explicitly pending.
- **TypeScript compatibility:** use 5.9.3 across clients because the current OpenAPI generator requires TypeScript 5.x; avoid forcing incompatible peer dependencies. Expo and React follow the SDK 58 compatibility matrix.
- **Local object storage:** SeaweedFS 4.53 provides the S3-compatible development service and initial bucket. The application contract remains S3; production storage is not selected. MinIO's historical public images were unavailable during verification.

Detailed architecture and consequences live in `docs/architecture.md`; operational limits in `docs/development.md` and `STATUS.md`.
