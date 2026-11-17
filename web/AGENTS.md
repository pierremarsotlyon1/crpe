# Web application

- Next.js App Router. Public signup and authentication start in `src/app/`, `connexion/` and `liste-attente/`. Member routes live in `espace/`. The teacher workspace lives in `atelier/`.
- Use server components for initial reads. `src/lib/api.ts` resolves `API_INTERNAL_URL`, forwards only the session cookie, and checks role via the API. Authorize each protected data entry point as well as its layout.
- Interactive forms call the generated shared client through the same-origin `/api/v1/` route. Its origin check, streaming uploads and no-store responses preserve the backend’s access controls. Never expose secrets, session tokens or answer keys in student props.
- The public waiting list builds without a running backend. Legal pages are publishing templates. Keep sales links and prices absent while the sales flag is disabled. Set `PUBLIC_WEB_URL` for canonical metadata and the sitemap.
- Teacher and student Markdown share the same renderer, with KaTeX and locally bundled fonts. Use French UI copy and teacher-validated educational material.
- Use `npm run lint -w @crpe/web`, `npm run typecheck -w @crpe/web`, `npm run build -w @crpe/web` and `npm run dev -w @crpe/web` from the root.
- `make browser-e2e` creates an isolated full-stack browser fixture with real HTTP, PostgreSQL, Mailpit, object storage and workers. It provisions a teacher and two read-only payment display fixtures for sales-enabled offers and historical invoices. Use `E2E_BROWSER_CHANNEL=chrome` for an installed local Chrome. Store membership tests use owner-scoped database fixtures and never contact commercial accounts.
- Update the backend contract and regenerate shared types before wiring new endpoints. Do not edit `.next/`, `next-env.d.ts` or generated API types by hand.
