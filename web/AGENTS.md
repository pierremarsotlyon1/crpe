# Web application

- Next.js App Router. Entry points: `src/app/layout.tsx`, `src/app/page.tsx`; global styles beside them.
- Read only the relevant route and nearby components. Keep server components as the default; introduce client components for actual interaction.
- This foundation has a neutral landing page only. Do not imply that signup, content, authentication or payments work.
- Use `@crpe/api-client` for API calls. `src/lib/api.ts` provides server-only origin configuration via `API_INTERNAL_URL`; never expose private configuration through `NEXT_PUBLIC_*`.
- The landing page builds and runs independently of the backend. Do not perform network requests at build time.
- French UI, semantic HTML, responsive layout, no external font downloads. Backend authorization is mandatory even if the UI hides a feature.
- Root commands: `npm run dev -w @crpe/web`, `npm run lint -w @crpe/web`, `npm run typecheck -w @crpe/web`, `npm run build -w @crpe/web`.
- Read Next.js documentation for the pinned version before using unfamiliar APIs. Generated `.next/` and `next-env.d.ts` are not authoring context.
