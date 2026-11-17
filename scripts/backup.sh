#!/bin/sh
set -eu
target=${1:?Supply a new output dump path}
umask 077
set -C
# PGDATABASE accepts a connection URI without exposing it in command arguments.
if [ "${CRPE_LOCAL_BACKUP:-false}" = true ]; then
  (
    cd "$(dirname "$0")/.."
    docker compose --project-name crpe --env-file .env -f infra/compose.yaml exec -T postgres \
      pg_dump -U crpe -d crpe --format=custom --no-owner --no-privileges
  ) > "$target"
else
  : "${DATABASE_URL:?Set DATABASE_URL, or CRPE_LOCAL_BACKUP=true for the local Compose stack}"
  PGDATABASE="$DATABASE_URL" pg_dump --format=custom --no-owner --no-privileges > "$target"
fi
printf '%s\n' 'Backup completed. Store the dump in encrypted storage.'
