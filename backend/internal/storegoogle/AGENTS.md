# Google Play verification

- Use the fixed Google OAuth, Android Publisher and OpenID JWKS origins. Derive account, product, base plan, expiry and environment from a verified provider response.
- A Pub/Sub notification only requests reconciliation. Verify its Google signature, issuer, audience, verified service account email, subscription and package before returning a purchase reference.
- For deferred replacements, use the old entitlement before expiry and the new line only after the transfer. Reject overlapping current products; they require a separate add-ons product model.
- The caller owns the durable ledger, owner check, configured product mapping, event idempotency and acknowledgement job. Keep raw purchase tokens, credentials and HTTP response bodies out of application errors and logs.
- Run `go test -race ./internal/storegoogle` from `backend/`. Fixtures use local HTTP transport and generated RSA keys. Production acceptance also needs a configured Play Console app, products and license-test purchase.
