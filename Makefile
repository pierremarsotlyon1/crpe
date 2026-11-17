SHELL := /bin/sh
COMPOSE := docker compose --project-name crpe --env-file .env -f infra/compose.yaml

.PHONY: dev down logs dev-mobile install check lint test build generate integration scale-smoke backend-lint backend-test backend-build

.env:
	cp .env.example .env

dev: .env
	$(COMPOSE) up --build --detach --wait proxy web worker s3

down: .env
	$(COMPOSE) down

logs: .env
	$(COMPOSE) logs --follow --tail=100

dev-mobile:
	npm run dev --workspace @crpe/mobile

install:
	npm ci
	cd backend && go mod download

check: lint test build

lint: backend-lint
	npm run lint
	npm run typecheck
	npm run generate:check
	docker compose --env-file .env.example -f infra/compose.yaml config --quiet

test: backend-test
	npm test

build: backend-build
	CI=1 NEXT_TELEMETRY_DISABLED=1 EXPO_NO_TELEMETRY=1 EXPO_OFFLINE=1 npm run build

backend-lint:
	cd backend && test -z "$$(gofmt -l .)"
	cd backend && go vet ./...

backend-test:
	cd backend && go test -race ./...

backend-build:
	cd backend && go build ./...

generate:
	npm run generate

integration:
	sh scripts/integration.sh

scale-smoke:
	sh scripts/scale-smoke.sh
