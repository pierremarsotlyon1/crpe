#!/bin/sh
set -eu
backup=${1:?Supply a PostgreSQL custom-format dump path}
test -f "$backup"
backup=$(cd "$(dirname "$backup")" && pwd)/$(basename "$backup")
cd "$(dirname "$0")/.."
project="crpe-restore-$$"
export POSTGRES_PORT=0
compose() {
  docker compose --project-name "$project" --env-file .env.example -f infra/compose.yaml "$@"
}
cleanup() { compose down --volumes --remove-orphans >/dev/null; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
compose up --detach --wait postgres
compose exec -T postgres pg_restore -U crpe -d crpe --no-owner --no-privileges --exit-on-error < "$backup"
compose exec -T postgres psql -U crpe -d crpe -v ON_ERROR_STOP=1 -c 'SELECT version,applied_at FROM schema_migrations ORDER BY version;'
compose exec -T postgres psql -U crpe -d crpe -v ON_ERROR_STOP=1 -c "SELECT 'users' AS entity,count(*) FROM users UNION ALL SELECT 'content',count(*) FROM content UNION ALL SELECT 'submissions',count(*) FROM submissions;"
printf '%s\n' 'PASS: dump restored into an isolated disposable database.'
