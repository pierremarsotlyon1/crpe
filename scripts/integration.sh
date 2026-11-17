#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
repo=$(pwd)

project="crpe-integration-$$"
export POSTGRES_PORT=0
export S3_PORT=0
compose() {
  docker compose --project-name "$project" --env-file "$repo/.env.example" -f "$repo/infra/compose.yaml" "$@"
}
cleanup() {
  compose down --volumes --remove-orphans >/dev/null
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

compose up --detach --wait postgres s3
port=$(compose port postgres 5432 | sed 's/.*://')
export TEST_DATABASE_URL="postgres://crpe:crpe_local@127.0.0.1:$port/crpe?sslmode=disable"
s3_port=$(compose port s3 8333 | sed 's/.*://')
export S3_ENDPOINT="http://127.0.0.1:$s3_port" S3_BUCKET=crpe-development S3_REGION=us-east-1 S3_ACCESS_KEY_ID=crpe_local S3_SECRET_ACCESS_KEY=crpe_local_secret
cd backend
go test -race -tags=integration,storageintegration -count=1 ./...
