# Infrastructure

- Read `docs/architecture.md` before changing runtime topology.
- `compose.yaml` is local-only. Bind published ports to loopback; replicas must not have `container_name` or published host ports.
- Keep API and worker root filesystems read-only, all persistent state in backing services, and migration a one-shot dependency.
- Proxy routing must refresh Docker DNS when replicas change; never enable sticky sessions.
- Test projects are isolated and disposable. Never remove the development database volume in checks.
- Verify `docker compose --env-file .env.example -f infra/compose.yaml config --quiet`, then `make integration` and `make scale-smoke` for topology changes.
