# Decision index

- **Native mobile override (user, voice):** replace Expo/React Native with Swift for iOS and Java for Android. Keep root `mobile/` with separate native projects and preserve the shared backend contract. This supersedes the attached stack; native builds and signing must be verified independently.
- **Replaceable task transport (user, voice):** keep PostgreSQL now, separate queue delivery/acknowledgement from pure job handlers through small Go interfaces. RabbitMQ remains a future adapter, not an installed service.

- **Scope:** the user expanded the task to all product features, interfaces, APIs and native clients. The attached roadmap remains product reference; no automatic sales activation, publication or third-party account creation.
- **Architecture:** modular Go monolith, explicit constructor injection, independently replicated API and workers. This keeps a solo-maintained codebase small while allowing horizontal process scaling.
- **State:** PostgreSQL for durable/shared state and queue; S3-compatible storage for objects. Authentication uses shared/revocable sessions rather than instance-local sessions.
- **Repository:** `backend/`, `web/`, `mobile/`, `packages/`, `infra/`, `docs/`, `scripts/` directly at the root. No redundant `apps/` layer.
- **Agent context:** root invariants plus scoped `AGENTS.md`, task-oriented context map, explicit source/generated boundaries and commands. No duplicated rule files for each AI editor.
- **Product name:** use the working name “Prépa Maths CRPE” until branding is chosen.
- **Delivery evidence:** local and CI results are distinct; native debug/simulator builds are not signed store releases. Incomplete features remain explicitly pending.
- **TypeScript compatibility:** use 5.9.3 across clients because the current OpenAPI generator requires TypeScript 5.x; avoid forcing incompatible peer dependencies. React is scoped to the web after the native override.
- **Local object storage:** SeaweedFS 4.53 provides the S3-compatible development service and initial bucket. The application contract remains S3; production storage is not selected. MinIO's historical public images were unavailable during verification.

- **Video scope audit:** the original PDF specifies typed/photo AI corrections, recorded oral practice and teacher-authored video corrections in the Suivi offer. It does not explicitly specify analysis of an uploaded learner video. Suivi delivery and acceptance are tracked separately from any future learner-video analysis.
- **Visible subscription locks (user, voice):** show unavailable features with a lock and the required plan on web and native clients; the API remains authoritative. When sales are enabled, the web directs learners to the corresponding offer. No client token may be passed in an upgrade URL.
- **Native purchases (user, 17 November 2026):** use StoreKit on iOS and Google Play Billing on Android instead of external purchase links. Validate purchases on the backend, tie each purchase to its member, and share the resulting rights across clients. Store credentials, sandbox acceptance and signed distribution remain separate release requirements.
- **Math rendering dependency:** pin KaTeX 0.19.1 for direct and transitive web usage. The override prevents Markdown plug-ins from resolving their older vulnerable range; retain rendering acceptance when updating it.
- **Correction progress:** expose persisted queue stages to authenticated clients, poll only active visible screens, and distinguish offline uploads from server-side work. Never invent a percentage or completion estimate.
- **Correction safeguards:** use currently published reference content, exact arithmetic, evidence-grounded feedback and human review for uncertain proof/photo interpretations. Quotas are per student, independent of worker concurrency.

Detailed architecture and consequences live in `docs/architecture.md`; operational limits in `docs/development.md` and `STATUS.md`.
