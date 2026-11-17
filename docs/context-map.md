# Task-oriented context map

Start with root `AGENTS.md`, `PLANS.md` and the scoped instructions for the area being changed.

| Task                                    | Start here                                                                                   | Related code                                                                                            |
| --------------------------------------- | -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Runtime and deployment                  | `infra/compose.yaml`, root Makefile                                                          | `backend/cmd/{api,worker,migrate}/`, `docs/architecture.md`                                             |
| Contract or client generation           | `backend/api/openapi.yaml`                                                                   | `packages/api-client/src/index.ts`, `mobile/generate.rb`                                                |
| Identity, roles, consent, exports       | `backend/internal/learning/identity.go`, `app.go`                                            | `backend/internal/mailer/`, `web/src/components/Identity.tsx`                                           |
| Public site and waiting list            | `web/src/app/page.tsx`, `web/src/app/liste-attente/`                                         | `web/src/app/informations/`, `web/src/app/sujets/`, `web/src/app/sitemap.ts`                            |
| Server auth and HTTP forwarding         | `web/src/lib/api.ts`, `web/src/app/api/v1/[...parts]/route.ts`                               | `web/src/lib/browser-api.ts`, `backend/internal/learning/app.go`                                        |
| Teacher content, sources and import     | `web/src/app/atelier/contenus/`, `web/src/components/Editor.tsx`                             | `Importer.tsx`, `Documents.tsx`, `backend/internal/learning/{content,files}.go`                         |
| Courses, diagnostic and revision        | `web/src/app/espace/`, `web/src/components/Practice.tsx`                                     | `backend/internal/learning/{practice,mock}.go`                                                          |
| Copy uploads and corrections            | `backend/internal/learning/files.go`, `backend/internal/correction/`                         | `backend/internal/objectstore/`, `web/src/components/{SubmissionList,CorrectionFeedback}.tsx`           |
| Mock exams and oral practice            | `backend/internal/learning/{mock,oral,draft,bank,support}.go`                                | `backend/internal/{dialog,voice}/`, `web/src/components/{MockExam,MockDraft,Support,Oral,Recorder}.tsx` |
| Billing and access                      | `backend/internal/billing/`, `backend/internal/learning/{billing,access}.go`                 | `web/src/app/{offres,espace/acces,espace/abonnement,atelier/paiements}/`, `web/src/lib/access.ts`       |
| Native store verification               | `backend/internal/storeprovider/`, `backend/internal/{storeapple,storegoogle,storebilling}/` | `web/src/components/Membership.tsx`, `backend/migrations/0013_store_memberships.sql`                    |
| Mobile clients                          | `mobile/AGENTS.md`, `mobile/ios/Sources/`                                                    | `mobile/android/app/src/main/java/fr/prepamathscrpe/mobile/`, `mobile/shared/`                          |
| Queue, migrations and integration tests | `backend/migrations/`, `backend/internal/queue/`                                             | `backend/internal/worker/`, `scripts/integration.sh`                                                    |
| Browser regression or CI failure        | `web/tests/product.spec.ts`, `scripts/browser-e2e.sh`                                        | `web/playwright.config.ts`, `.github/workflows/ci.yaml`, `STATUS.md`                                    |

Read the source operation and adjacent tests before widening a change. Regenerate client outputs from the contract. Use `STATUS.md` for observed results, `DECISIONS.md` for project decisions and `PLANS.md` for remaining acceptance criteria.
