#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

project="crpe-browser-$$"
scratch=$(mktemp -d)
export POSTGRES_PORT=0 API_PORT=0 MAILPIT_PORT=0 S3_PORT=0 SMTP_PORT=0 SALES_ENABLED=false
export ADMIN_EMAILS=enseignante@example.test
export BILLING_PLANS_JSON='[{"key":"auto-test","name":"Autonomie de test","tier":"autonomie","priceId":"price_test_autonomie"},{"key":"suivi-test","name":"Suivi de test","tier":"suivi","priceId":"price_test_suivi"}]'
repo_dir=$(pwd)
# A stable web origin must be known before constructing the email confirmation URLs.
WEB_PORT=$(node -e 'const s=require("node:net").createServer();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})')
export WEB_PORT
PUBLIC_WEB_URL="http://127.0.0.1:$WEB_PORT"
export PUBLIC_WEB_URL

# Unique image tags prevent parallel checks from replacing the developer's backend image.
cat > "$scratch/override.yaml" <<EOF_COMPOSE
services:
  api:
    image: $project-backend:local
    environment:
      LLM_ENDPOINT: ""
      LLM_MODEL: ""
      LLM_API_KEY: ""
      STT_ENDPOINT: ""
      STT_MODEL: ""
      STT_API_KEY: ""
      TTS_ENDPOINT: ""
      TTS_MODEL: ""
      TTS_API_KEY: ""
      CORRECTION_MODE: local
  sales-api:
    image: $project-backend:local
    extends:
      file: "$repo_dir/infra/compose.yaml"
      service: api
    environment:
      SALES_ENABLED: "true"
      STRIPE_SECRET_KEY: ""
      STRIPE_WEBHOOK_SECRET: ""
  sales-fixture:
    image: $project-web
    entrypoint: [node, /tmp/offers.mjs]
    volumes:
      - "$repo_dir/web/tests/fixtures/offers.mjs:/tmp/offers.mjs:ro"
    depends_on:
      sales-api:
        condition: service_healthy
  sales-web:
    image: $project-web
    environment:
      HOSTNAME: 0.0.0.0
      PORT: 3000
      API_INTERNAL_URL: http://sales-fixture:8080
    ports: ["127.0.0.1::3000"]
    depends_on: [sales-fixture]
    healthcheck:
      test: [CMD, wget, -q, -O, /dev/null, "http://127.0.0.1:3000/"]
      interval: 2s
      timeout: 4s
      retries: 30
  history-web:
    image: $project-web
    environment:
      HOSTNAME: 0.0.0.0
      PORT: 3000
      API_INTERNAL_URL: http://sales-fixture:8081
    ports: ["127.0.0.1::3000"]
    depends_on: [sales-fixture]
    healthcheck:
      test: [CMD, wget, -q, -O, /dev/null, "http://127.0.0.1:3000/"]
      interval: 2s
      timeout: 4s
      retries: 30
  migrate:
    image: $project-backend:local
  worker:
    image: $project-backend:local
    environment:
      LLM_ENDPOINT: ""
      LLM_MODEL: ""
      LLM_API_KEY: ""
      CORRECTION_MODE: local
EOF_COMPOSE

compose() {
  docker compose --project-name "$project" --env-file .env.example -f infra/compose.yaml -f "$scratch/override.yaml" "$@"
}
cleanup() {
  result=$?
  trap - EXIT
  if [ "$result" -ne 0 ]; then compose logs --tail=30 api web worker sales-api sales-fixture sales-web history-web >&2 || :; fi
  compose down --volumes --remove-orphans >/dev/null || :
  docker image rm "$project-backend:local" "$project-web" >/dev/null 2>&1 || :
  rm -rf "$scratch"
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

compose build api web
compose up --no-build --detach --wait --wait-timeout 150 proxy web worker s3 mailpit sales-web history-web
history_port=$(compose port history-web 3000 | sed 's/.*://')
export E2E_HISTORY_URL="http://127.0.0.1:$history_port"
sales_port=$(compose port sales-web 3000 | sed 's/.*://')
export E2E_SALES_URL="http://127.0.0.1:$sales_port"
mail_port=$(compose port mailpit 8025 | sed 's/.*://')
export E2E_COMPOSE_PROJECT="$project"
export E2E_ADMIN_EMAIL="$ADMIN_EMAILS" E2E_WEB_URL="$PUBLIC_WEB_URL" E2E_MAIL_URL="http://127.0.0.1:$mail_port"
E2E_OUTPUT_DIR=$(mktemp -d "${TMPDIR:-/tmp}/crpe-browser-results.XXXXXX")
export E2E_OUTPUT_DIR
printf 'Playwright reports: %s\n' "$E2E_OUTPUT_DIR"
npm run test:e2e -w @crpe/web
