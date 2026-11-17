# Store verification contract

- Adapters authenticate provider responses and normalize their current state. They never grant access or trust client-provided expiry, tier, price or account claims.
- Membership owns product/base-plan allowlists, member matching, immutable purchase ownership, serialized reconciliation, durable acknowledgement retries and SQL entitlements.
- Notifications are authenticated refresh signals; fetch the current provider state before applying them. Never apply client or notification status as an entitlement.
- Keep provider references, private keys and bearer tokens out of logs and public errors. No production verifier accepts local StoreKit signatures or arbitrary signing roots.
