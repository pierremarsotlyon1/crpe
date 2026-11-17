# Automatic correction

- `check.go` is deterministic numeric/choice verification; free-form proof needs teacher review.
- `provider.go` validates structured model output against the teacher reference and student evidence. Do not trust model verdicts over deterministic checks. Photo feedback always needs teacher review.
- `handler.go` reserves a per-student UTC-day quota in PostgreSQL, persists usage/result atomically, and handles duplicate delivery. This is not a worker capacity limit.
- Local mode is explicitly `local_deterministic`, with `generatedByAI=false`. Never label local tests as real provider validation.
- A rejected or unavailable provider result falls back to teacher review with `generatedByAI=false`; usage can still be recorded without claiming the displayed fallback came from AI.
- Tests: `go test -race ./internal/correction`; real PostgreSQL tests use `-tags=integration` and an isolated `TEST_DATABASE_URL`.
- Keep prompts/copies/provider response bodies out of logs. Costs are estimates from configured prices; absent prices mean unknown, never zero-cost claims.
