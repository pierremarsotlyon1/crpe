# Deployment and operations

The repository builds independent Go API/worker images, a Next.js standalone image,
a native Swift iOS application and a native Java Android application. Local tests
do not imply a deployment, App Store approval or Google Play approval. See `STATUS.md`
for observed checks and `mobile/README.md` for signing instructions when available.

## Release inputs

Supply the product domain, EU hosting region, private object bucket and actual
transactional email relay. The teacher account is granted only to configured
`ADMIN_EMAILS`; never deploy the local example address as your production policy.
Approved teaching content and legal text belong to the teacher/business owner.
The legal pages explicitly remain templates until those details are supplied.

Keep `SALES_ENABLED=false` until acceptance of the enabled payment providers and
the approved legal copy are complete. Production does not require Stripe when sales are disabled.
AI/audio features need separately configured real providers; lack of credentials
must show an unavailable/manual-review path instead of simulated success.

## Native subscriptions

The owner selected native purchases on 17 November 2026: StoreKit on iOS and
Google Play Billing on Android. External web purchase links are not the selected
mobile purchase path. Locked features must open the appropriate native offer,
using the store's localized price and billing period. The shared API remains the
authority for access on mobile and web. See `STATUS.md` for implementation and
verification evidence; selecting this architecture is not store acceptance.

Create matching Autonomie and Suivi subscriptions in App Store Connect and Play
Console, with the desired monthly/yearly periods. Configure provider identifiers,
server credentials and notification endpoints before the real sandbox tests.
Separate sandbox and production configuration; never accept test transactions as
production entitlements or embed provider private keys in an app.

Acceptance must cover purchase, cancellation before payment, pending approval,
restore after reinstall, network loss after payment, renewal, upgrade, grace
period, expiration, refund and duplicate/delayed notifications. Verify that one
store transaction cannot be claimed by another member and that an existing
subscription is not duplicated through a different payment provider. Account
deletion and subscription cancellation are distinct store operations: the UI
must explain and expose the store's subscription management where necessary.

Use [Apple's transaction verification](https://developer.apple.com/documentation/storekit/verificationresult),
[App Store Server Notifications](https://developer.apple.com/documentation/appstoreservernotifications/responsebodyv2),
[Google purchase verification](https://developer.android.com/google/play/billing/security)
and [Google's subscription lifecycle](https://developer.android.com/google/play/billing/lifecycle/subscriptions)
as the provider contracts. Local fixtures and unsigned builds do not establish
that purchases work through an actual store account.

## Application deployment

1. Build images from the repository root:

   ```sh
   docker build -t crpe-backend:release backend
   docker build -f web/Dockerfile -t crpe-web:release .
   ```

2. Provision managed PostgreSQL, a private S3-compatible bucket, TLS ingress and
   SMTP in the selected EU region. Enable PostgreSQL point-in-time recovery and
   bucket versioning/lifecycle according to the retention policy.
3. Run `/app/migrate` once with the new backend image and production database
   secret. Migrations are immutable, checksummed and serialized. Do not start a
   rolling upgrade until the migration step succeeds.
4. Run `/app/api` and `/app/worker` as separate disposable services. Use at least
   two API replicas when availability requires it. Add worker replicas for queue
   latency. No volumes, local sessions or sticky routing on these services.
5. Configure the web image with `API_INTERNAL_URL`, `PUBLIC_WEB_URL`, `PORT` and
   `HOSTNAME=0.0.0.0`. Give only backend services database/provider credentials.
6. Point public API/web TLS routes at healthy instances. Allow multipart requests
   up to 21 MiB at ingress; the API applies 8 MiB image and 20 MiB PDF/audio limits.
   Allow at least 80 seconds for audio requests. Stop new traffic before draining
   an old API; give workers at least 60 seconds to finish their current task.
7. Verify the live health probes, teacher/student permissions, emails, uploads,
   correction progression and account deletion in a staging environment first.

Calculate pool capacity before scaling: API replicas × pool + worker replicas ×
pool + migrations + monitoring + reserve must stay below the database connection
budget. Default pool size is five per process. Database capacity remains a shared
limit even though application servers are stateless.

## Configuration

Use a secret manager or the deployment platform's protected environment values.
Never commit a real `.env`, print credentials in logs, or embed API secrets in a
mobile binary. `.env.example` contains development-only values.

| Area               | Variables and behavior                                                                                                                                                                                |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Database           | `DATABASE_URL`, `DATABASE_MAX_CONNS`; use provider-required TLS                                                                                                                                       |
| Web/auth           | `PUBLIC_WEB_URL` HTTPS, `MOBILE_AUTH_URL`, `ADMIN_EMAILS`                                                                                                                                             |
| SMTP               | `SMTP_ADDR`, `SMTP_FROM` bare email, `SMTP_USERNAME`, `SMTP_PASSWORD`; STARTTLS required unless explicitly local plaintext                                                                            |
| Files              | `S3_ENDPOINT`, `S3_BUCKET`, `S3_REGION`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`; private bucket, least-privilege credentials                                                                      |
| Correction         | `CORRECTION_MODE=provider`, `CORRECTION_DAILY_LIMIT`; quota is per student and UTC day, not per worker                                                                                                |
| LLM                | Full OpenAI-compatible chat URL `LLM_ENDPOINT`, `LLM_MODEL`, `LLM_API_KEY`; no provider/model is silently selected                                                                                    |
| Usage estimates    | Optional `LLM_INPUT_USD_PER_MILLION`, `LLM_OUTPUT_USD_PER_MILLION`; unknown prices remain unknown                                                                                                     |
| Oral transcription | Full multipart endpoint `STT_ENDPOINT`, `STT_MODEL`, `STT_API_KEY`                                                                                                                                    |
| Speech             | Full JSON endpoint `TTS_ENDPOINT`, `TTS_MODEL`, `TTS_API_KEY`, `TTS_VOICE`                                                                                                                            |
| Billing            | `SALES_ENABLED`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `BILLING_PLANS_JSON`                                                                                                                   |
| Store catalog      | `STORE_PRODUCTS_JSON`, `STORE_REFERENCE_KEY` (32 random bytes, standard Base64, identical on API and workers)                                                                                         |
| App Store          | `APPLE_STORE_BUNDLE_ID`, numeric `APPLE_STORE_APP_ID`, `APPLE_STORE_KEY_ID`, `APPLE_STORE_ISSUER_ID`, PKCS#8 PEM `APPLE_STORE_PRIVATE_KEY`, `APPLE_STORE_ENVIRONMENT=Sandbox\|Production`             |
| Google Play        | `GOOGLE_PLAY_PACKAGE_NAME`, `GOOGLE_PLAY_SERVICE_ACCOUNT_JSON`, `GOOGLE_PLAY_ENVIRONMENT=Sandbox\|Production`, `GOOGLE_PLAY_PUSH_AUDIENCE`, `GOOGLE_PLAY_PUSH_EMAIL`, `GOOGLE_PLAY_PUSH_SUBSCRIPTION` |
| Abuse controls     | `AUTH_IP_HOURLY_LIMIT`, `TRUSTED_PROXY_CIDRS`; no proxy trusted by default                                                                                                                            |

`BILLING_PLANS_JSON` is an array of objects with `key`, `name`, `priceId` and
`tier` (`autonomie` or `suivi`). An omitted tier defaults to `autonomie`; a plan's
display name or Stripe price ID never grants premium access. Configure the tier
explicitly for every Suivi price, including both monthly and yearly variants.
Prices are fetched from Stripe and must be monthly/yearly recurring prices.
Use separately configured Autonomie/Suivi monthly/yearly prices as required by the
business. The API version is pinned in the adapter; verify version compatibility
when changing the Stripe account webhook configuration.

Set `STORE_ENVIRONMENT=Production` explicitly for the production deployment and
`STORE_ENVIRONMENT=Sandbox` for a separate test deployment, including when new
sales or a provider are temporarily disabled. Configured Apple/Google modes and
recognizable Stripe live/test secret keys must agree. Once a mode is established,
the database records it and rejects a process configured for the other mode.
Never repurpose a test database as the production subscription ledger.

Each store's credentials must form a complete configuration. Keep its whole group
blank to disable it locally; a partial configuration is an error. Apple uses an
App Store Connect In-App Purchase key and trusted Apple signing roots embedded in
the adapter. Google uses a service account authorized for the app in Play Console;
its JSON credentials are server secrets. Configure authenticated Pub/Sub push with
the exact expected HTTPS audience, service-account email and subscription resource
name. Store production configurations reject sandbox/test purchases. Sandbox
acceptance requires a separate deployment and database, not a production toggle
used interchangeably for both transaction classes.

`STORE_PRODUCTS_JSON` maps each approved store product to its server-side tier:

```json
[
  {
    "platform": "apple",
    "tier": "autonomie",
    "planKey": "autonomie-mensuel",
    "productId": "YOUR_APPLE_PRODUCT_ID"
  },
  {
    "platform": "google",
    "tier": "suivi",
    "planKey": "suivi-mensuel",
    "productId": "YOUR_GOOGLE_PRODUCT_ID",
    "basePlanId": "YOUR_BASE_PLAN_ID"
  }
]
```

These placeholders are not purchasable products. Create the products in the
respective store first; apps obtain the actual localized price there. Use one
Apple subscription group for the related levels and configure their upgrade
order. Keep Family Sharing disabled for this account-bound membership. The API
checks the verified product and Google base plan against this allowlist; a client
cannot select its own tier.

`STORE_REFERENCE_KEY` encrypts provider references needed by durable reconciliation
and acknowledgement jobs. Generate it with a cryptographically secure generator,
keep it in the deployment's secret manager and include it in the protected
recovery plan. All API/worker replicas must use the same key for that deployment.
Replacing it without migrating existing ciphertext breaks recovery of pending
purchases. Never commit or print its value in application logs.

Register the direct HTTPS API endpoint `/v1/store-notifications/apple` for App
Store Server Notifications V2 and `/v1/store-notifications/google` as an
authenticated Google Pub/Sub push target. Do not use the browser `/api/v1/`
forwarder for provider callbacks. Configure Apple's separate sandbox/production
URLs and Google's app RTDN topic/subscription for the matching environment. Test
the providers' test notifications before purchasing, and monitor delivery errors
and dead-letter handling. A callback is a signal to fetch current store state;
its body cannot grant access by itself.

Direct API ingress must overwrite `X-Real-IP` and use a dedicated trusted network
before enabling `TRUSTED_PROXY_CIDRS`. Never trust arbitrary client forwarding
headers or configure `0.0.0.0/0`. The web proxy strips client-supplied identity
headers; its auth requests otherwise share the server's IP bucket. Configure a
sanitizing trusted ingress and an appropriate auth limit for the deployment.

## Provider acceptance

Local SMTP goes to Mailpit; it does not prove external deliverability. Verify SPF,
DKIM, DMARC and real inbox delivery for login, double opt-in and unsubscribe links.

Stripe acceptance uses test credentials, real Checkout, signed webhooks, portal,
invoice retrieval, cancellation, refunds and duplicate/delayed event delivery.
Include concurrent checkout clicks, a change of plan before payment, and a
completed payment whose webhook has not yet arrived. The API reuses an open
provider session, confirms expiry before replacing another offer and checks
provider subscriptions before issuing a new checkout. It never grants access
from the browser's success redirect. Configure Stripe's additional
[one-subscription redirection](https://docs.stripe.com/payments/checkout/limit-subscriptions)
and the portal's allowed upgrade prices in the test account as well.
Account deletion cancels Stripe subscriptions by deleting its customer before
removing local data, using the [Stripe customer deletion contract](https://docs.stripe.com/api/customers/delete).
If Stripe is unavailable, the account is retained so billing can be reconciled.
Subscribe to checkout.session.completed, checkout.session.async_payment_succeeded,
customer.subscription.created/updated/deleted, invoice.paid, charge.succeeded/refunded
and refund.created/updated/failed. Refund settlement may be asynchronous; the adapter
keeps a pending refund until its authoritative provider status confirms the outcome.
Register the public HTTPS API route `/v1/billing/webhook` directly with Stripe;
the browser's `/api/v1/` proxy intentionally does not forward provider signature
headers. The API verifies `Stripe-Signature` with `STRIPE_WEBHOOK_SECRET`.

For AI acceptance submit an approved numeric exercise, a free-form proof, a clear
photo and an unreadable photo. Check French feedback, reference consistency,
evidence localization, teacher escalation and usage accounting. All photo
corrections remain reviewable by the teacher. Repeat a full oral session with
real STT/TTS and the configured rubric; feedback must have no grade out of 20.

The deterministic local correction adapter is explicitly marked as local and
`generatedByAI=false`. It verifies final numeric values and choices; it does not
claim handwriting recognition or proof evaluation. Provider idempotency support
varies: a crash after a paid provider response can cause another paid call even
though the persisted result and quota reservation are deduplicated. Reconcile
usage estimates with provider invoices; do not represent them as billing totals.

## Monitoring and recovery

Send JSON stdout logs to a central collector, keeping student work and secrets out
of logs. Monitor readiness failures, API latency/error rate, PostgreSQL connections,
queue oldest age, failed jobs, provider failures and storage errors. Set alert
thresholds against staging measurements; an alert destination must be supplied by
the operator before production. No external alert service is configured locally.

The following read-only queries support queue and correction dashboards:

```sql
SELECT status, count(*), min(created_at) AS oldest FROM jobs GROUP BY status;
SELECT provider, outcome, count(*), sum(input_tokens), sum(output_tokens),
       sum(estimated_cost_usd) FROM correction_usage GROUP BY provider,outcome;
```

Investigate a failed job before retrying: verify its resource still exists,
provider configuration and handler idempotency. A retry is an operator action,
not a silent client loop. Copies already corrected by the teacher must not be
overwritten by an old automatic task.

Use encrypted managed backups and run restore drills into a separate database.
Never restore over production as a test. `scripts/backup.sh` creates a restrictive
custom-format PostgreSQL dump; `scripts/restore-drill.sh` verifies a supplied dump
in a new disposable local Compose database. Object data needs its own versioned
backup and deletion-retention policy; a database dump does not contain files.

For the local Compose stack (no host PostgreSQL client required):

```sh
CRPE_LOCAL_BACKUP=true sh scripts/backup.sh /secure/new-backup.dump
sh scripts/restore-drill.sh /secure/new-backup.dump
```

For a remote database, use PostgreSQL 18 client tools and provide `DATABASE_URL`
through the environment to `scripts/backup.sh`. The script refuses to overwrite
an existing file. A failed backup may leave a partial file; never treat it as a
successful restore point without a completed restore drill.

Protect the release branch with required CI checks and review. Keep workflow
permissions read-only unless a later publishing workflow needs specific writes.
Put release secrets in protected GitHub environments, with approval rules for
production. The current workflow checks code and builds; it does not publish apps
or push production images.
