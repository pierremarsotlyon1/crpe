# Shared API client

- Source of truth: `../../backend/api/openapi.yaml`. Read only the operations relevant to the task.
- `src/schema.ts` is generated. Never edit it manually; run `npm run generate -w @crpe/api-client` at the root.
- `src/index.ts` is authored, framework-independent transport. Do not import React, Next.js, Expo or backend implementation details.
- All workspaces pin TypeScript 5.9.3 because openapi-typescript 7.13.0 requires TypeScript `^5.x`. Upgrade the generator/compiler together; do not bypass peer-dependency checks.
- Keep server errors and offline states explicit. Presentation flags fail closed; backend authorization remains mandatory.
- `npm run test -w @crpe/api-client` verifies transport and fail-closed behavior.
- `npm run typecheck -w @crpe/api-client` checks the generated contract. `npm run build -w @crpe/api-client` emits `dist/` for inspection; workspace consumers transpile the source.
