# Apple subscription adapter

- `client.go` calls fixed App Store endpoints and normalizes current subscription state. `signature.go` checks ES256, Apple's signing certificate profile and embedded public trust anchors.
- Never enable Xcode/local signatures or arbitrary roots in production. Unit tests generate their own chain in memory and replace unexported test-client fields only.
- A notification is only an authenticated refresh trigger. Always query the current subscription before changing access; old receipts and delayed notices cannot directly reinstate entitlement.
- Account ownership and the configured product/tier allowlist belong to `storeprovider`'s consumer. Family Sharing is not enabled for this account-bound product.
- Credentials and provider payloads must never appear in errors/logs. The API key stays on the server. Run `go test -race ./internal/storeapple` after changes.
