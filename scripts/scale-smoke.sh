#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

project="crpe-scale-$$"
export POSTGRES_PORT=0 API_PORT=0 SALES_ENABLED=false
scratch=$(mktemp -d)
compose() {
  docker compose --project-name "$project" --env-file .env.example -f infra/compose.yaml "$@"
}
cleanup() {
  result=$?
  if [ "$result" -ne 0 ]; then
    compose logs --tail=40 api proxy worker >&2
  fi
  compose down --volumes --remove-orphans >/dev/null
  rm -rf "$scratch"
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

compose build api
compose up --no-build --detach --wait --scale api=2 --scale worker=2 proxy worker
port=$(compose port proxy 8080 | sed 's/.*://')
url="http://127.0.0.1:$port"
test "$(compose ps --status running -q api | wc -l | tr -d ' ')" -eq 2
test "$(compose ps --status running -q worker | wc -l | tr -d ' ')" -eq 2

attempt=0
while [ "$attempt" -lt 30 ]; do
  curl --fail --silent --show-error --max-time 5 -D "$scratch/headers" "$url/v1/config" > "$scratch/config"
  node -e 'const fs=require("node:fs"); if (JSON.parse(fs.readFileSync(process.argv[1])).salesEnabled !== false) process.exit(1)' "$scratch/config"
  awk 'tolower($1)=="x-local-upstream:" {gsub("\r", "", $2); print $2}' "$scratch/headers" >> "$scratch/upstreams"
  attempt=$((attempt + 1))
done
test "$(sort -u "$scratch/upstreams" | wc -l | tr -d ' ')" -eq 2

compose exec -T postgres psql -U crpe -d crpe -v ON_ERROR_STOP=1 -c "INSERT INTO jobs(kind,deduplication_key) SELECT 'system.probe', 'scale-smoke-' || n FROM generate_series(1,60) n;"
attempt=0
completed=0
while [ "$attempt" -lt 60 ]; do
  completed=$(compose exec -T postgres psql -U crpe -d crpe -Atc "SELECT count(*) FROM jobs j JOIN job_probe_results p ON p.job_id=j.id WHERE j.status='completed';")
  [ "$completed" -eq 60 ] && break
  attempt=$((attempt + 1))
  sleep 1
done
test "$completed" -eq 60

# Killing an owned replica proves requests do not depend on one API process.
first_api=$(compose ps -q api | head -n 1)
docker stop "$first_api" >/dev/null
# Docker DNS and the proxy refresh asynchronously. Require bounded recovery,
# then uninterrupted successful requests; this is not a zero-downtime SLO test.
attempt=0
recovered=0
while [ "$attempt" -lt 10 ]; do
  if curl --fail --silent --show-error --max-time 3 "$url/health/ready" >/dev/null; then
    recovered=1
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
test "$recovered" -eq 1
attempt=0
while [ "$attempt" -lt 10 ]; do
  curl --fail --silent --show-error --max-time 5 "$url/health/ready" >/dev/null
  attempt=$((attempt + 1))
done
printf '%s\n' 'PASS: two API replicas, two workers, 60 durable jobs, and recovery after API replica shutdown.'
