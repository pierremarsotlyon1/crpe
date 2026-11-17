SHELL := /bin/sh
COMPOSE := docker compose --project-name crpe --env-file .env -f infra/compose.yaml

.PHONY: dev down logs install check lint test build generate integration scale-smoke backend-lint backend-test backend-build browser-e2e ios-test ios-build android-test android-build native-check

.env:
	cp .env.example .env

dev: .env
	$(COMPOSE) build api web
	$(COMPOSE) up --no-build --detach --wait proxy web worker s3 mailpit

down: .env
	$(COMPOSE) down

logs: .env
	$(COMPOSE) logs --follow --tail=100

install:
	npm ci
	cd backend && go mod download

check: lint test build

lint: backend-lint
	npm run lint
	npm run typecheck
	npm run generate:check
	ruby mobile/generate.rb --check
	docker compose --env-file .env.example -f infra/compose.yaml config --quiet

test: backend-test
	npm test

build: backend-build
	CI=1 NEXT_TELEMETRY_DISABLED=1 npm run build

backend-lint:
	cd backend && test -z "$$(gofmt -l .)"
	cd backend && go vet ./...

backend-test:
	cd backend && go test -race -shuffle=on ./...

backend-build:
	cd backend && go build ./...

generate:
	npm run generate
	ruby mobile/generate.rb

integration:
	sh scripts/integration.sh

scale-smoke:
	sh scripts/scale-smoke.sh

browser-e2e:
	sh scripts/browser-e2e.sh

ios-test:
	sh scripts/ios-test.sh

ios-build:
	xcodebuild -project mobile/ios/PrepaCRPE.xcodeproj -scheme PrepaCRPE -configuration Debug -sdk iphonesimulator -destination 'generic/platform=iOS Simulator' -derivedDataPath /tmp/crpe-ios-build CODE_SIGNING_ALLOWED=NO build

android-test:
	cd mobile/android && ./gradlew --no-daemon :app:testDebugUnitTest

android-build:
	cd mobile/android && ./gradlew --no-daemon :app:assembleDebug

native-check: ios-test android-test android-build
