# Development workflow

Run commands at the repository root. Docker with Compose starts the service stack. Host checks use the Go version in `backend/go.mod`, Node 24, npm 11+ and Ruby for native contract generation. Xcode and the Android SDK are separate native toolchains.

## Local services

```sh
make dev
```

The Makefile creates a local `.env` from `.env.example` when needed. Compose builds the applications, runs the one-shot migration, then starts the API, worker, proxy, web, PostgreSQL, SeaweedFS and Mailpit.

| Service                                | Default local address                |
| -------------------------------------- | ------------------------------------ |
| Site, student area and teacher atelier | `http://localhost:3000`              |
| API readiness                          | `http://localhost:8080/health/ready` |
| Mailpit mailbox                        | `http://localhost:8025`              |
| SMTP                                   | `localhost:11025`                    |
| PostgreSQL                             | `localhost:55433`                    |
| S3-compatible object storage           | `http://localhost:9000`              |

`PUBLIC_WEB_URL` is the absolute origin used in email links and public metadata. `API_INTERNAL_URL` stays on the web server and points to the API proxy. `ADMIN_EMAILS` selects the teacher addresses. Local SMTP terminates inside Mailpit. `SALES_ENABLED=false` closes new sales and preserves account management. `make down` preserves the local database and object volumes.

## Teacher and learner verification

1. Open **Mon espace** on the public site. Use `enseignante@example.test` for the default teacher account. Open the message in Mailpit, follow the link and confirm the login.
2. In **Contenus**, create a chapter. Use **Renseigner la source du sujet** to record an official source URL, title and year. Save, then attach the source under **Documents du sujet**. PDF uploads support up to 20 MiB.
3. Use **Ajouter un contenu** to attach a lesson or exercise. Preview the Markdown and KaTeX. Set the reference answer, solution and criteria applicable to the exercise format. Validate each item intended for learners. Editing published material returns the edited content to draft.
4. Alternatively use **Importer** for a Markdown lesson or CSV exercises. Download the header template, preview the rows, then import the batch as drafts.
5. In a separate browser session, log in with a student address. Open a chapter, answer an exercise or submit a written response or photograph. JPEG and PNG copies support up to 8 MiB. The teacher can review submissions and send individual feedback.
6. In **Épreuves et suivi**, grade free-text mock responses using the configured marking scale. An incomplete review leaves the grade provisional. Learners with active Suivi access see private teacher video feedback, the weekly meeting and the WhatsApp invitation when the teacher has supplied their links.
7. The member menu uses `GET /v1/me/access` to display restricted activities with a lock. With sales closed, the link explains the access level. When sales are enabled, it opens the matching offer. The API checks the entitlement independently on every restricted request.
8. A full diagnostic needs 20 validated eligible questions. The revision plan uses recent attempts by skill. Mock exams use server deadlines and a teacher-configured marking scale. Oral sessions use criteria. The student can read the questions and enter a transcript when speech providers are unavailable.

For optional development examples, run `DEMO_MODE=true DATABASE_URL='postgres://crpe:crpe_local@localhost:55433/crpe?sslmode=disable' go run ./cmd/demo` from `backend/`. It creates three labelled, unpublishable draft chapters.

The local deterministic correction mode exercises numeric checks and routes uncertain work for teacher review. Provider-based text/vision, transcription, synthesis and billing require their configured external services for live acceptance. Keep credentials outside version control.

## Commands

| Command                           | What it checks                                                            |
| --------------------------------- | ------------------------------------------------------------------------- |
| `make install`                    | Host JavaScript dependencies and Go module downloads                      |
| `make generate`                   | Regenerates the API contract outputs configured by the root Makefile      |
| `ruby mobile/generate.rb --check` | Checks generated native models and operation paths                        |
| `make check`                      | Root lint, contract drift, unit tests, type checks and configured builds  |
| `make integration`                | Integration suites against a disposable PostgreSQL project                |
| `make scale-smoke`                | Multiple API/worker replicas, durable work and replica recovery           |
| `make browser-e2e`                | Isolated full-stack browser checks including real SMTP, PostgreSQL and S3 |
| `npm run dev -w @crpe/web`        | Next.js development server while a local API is running                   |

The progress test pauses its own worker, locks a test submission row, then releases both to verify the real processing states. It does not change the displayed statuses directly.

Install Chromium for Playwright once using `npx playwright install chromium`. On a machine with Google Chrome installed, set `E2E_BROWSER_CHANNEL=chrome` to use that browser. The browser script builds an independently named Compose project with ephemeral loopback ports, dedicated data volumes and a test teacher. It cleans up its containers, volumes and test images. A second sales-enabled Go API tests paid access and offer targeting against the same isolated database. Browser tests insert owner-scoped, synthetic App Store and Play membership records into that ledger. A sales-disabled history view uses an invoice display fixture; the sales-enabled view uses test prices. The suite never contacts Stripe or either app store. A test-only SQL grant activates Suivi to cover video, live and WhatsApp visibility. `E2E_CAPTURE_PUBLIC=true` keeps screenshots of the public pages, atelier and locked member view in the test output directory. Test accounts and validated fixture content belong only in this isolated environment. The script prints the Playwright results location.

For a reproducible local mobile Lighthouse audit, run the production web build with its API, then use `npx lighthouse http://localhost:3000/ --form-factor=mobile --chrome-flags=--headless`. The local score measures the served page and local dependencies. A deployed domain needs its own audit.

## Native clients

- **iOS** uses the `PrepaCRPE` scheme in `mobile/ios/PrepaCRPE.xcodeproj`. Open the project in Xcode, select an installed simulator and run the scheme or its tests. Debug defaults to `http://localhost:8080`. The `CRPE_API_URL` build setting supplies the HTTPS release origin. Provision the app identifier, team and signing credentials before creating a distribution archive.
- **Android** uses Java 17 and the checked-in Gradle wrapper with Android SDK 37. Run `mobile/android/gradlew -p mobile/android :app:testDebugUnitTest :app:assembleDebug`. The emulator uses `http://10.0.2.2:8080` by default. Override with `-PCRPE_DEBUG_API_URL=http://...`. Release builds use `CRPE_API_URL` or `-PCRPE_API_URL=https://...`. Physical phones need a reachable API origin. Release signing belongs to the application owner.
- Native clients authenticate through email links and provide learning workflows. Stripe checkout is web-only. A member with a current App Store or Play subscription sees the corresponding management link and expiry in **Mon abonnement**. The Go store adapters validate provider-signed or provider-authenticated data. The native flows and external store sandboxes require their own acceptance. Follow the scoped [mobile instructions](../mobile/AGENTS.md) when updating the clients.

## Routine changes

Change the canonical schema and regenerate consumers together. Append database migrations. Keep durable jobs idempotent and run the integration tests for retries and lease expiry. The web proxy uses same-origin mutations and forwards only the session cookie to the API. Private data reads are uncached. Configure production origins, TLS, credentials, backups, monitoring and horizontal connection budgets separately from these development defaults.
