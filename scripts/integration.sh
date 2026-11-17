#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
repo=$(pwd)

project="crpe-integration-$$"
export POSTGRES_PORT=0
compose() {
  docker compose --project-name "$project" --env-file "$repo/.env.example" -f "$repo/infra/compose.yaml" "$@"
}
cleanup() {
  compose down --volumes --remove-orphans >/dev/null
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

compose up --detach --wait postgres
port=$(compose port postgres 5432 | sed 's/.*://')
export TEST_DATABASE_URL="postgres://crpe:crpe_local@127.0.0.1:$port/crpe?sslmode=disable"
cd backend
go test -race -tags=integration -count=1 ./...
