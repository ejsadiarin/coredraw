# coredraw — local tasks. Reads .env when present (MIGRATION_URL etc.).
-include .env
export

GOOSE := goose -dir internal/db/migrations postgres

.PHONY: run build test test-unit db-setup db-rotate-password migrate-up migrate-down migrate-status migrate-create sqlc sqlc-check docker-build

run:
	go run ./cmd/api

build:
	go build -o bin/coredraw-api ./cmd/api

test: ## unit + Docker-backed integration (testcontainers)
	go test ./... -count=1

test-unit: ## no Docker needed
	go test ./... -short -count=1

# One-time per environment: bootstrap.sql, coredraw_role password into .env,
# migrations, check. Safe to re-run. See scripts/db-setup.sh.
db-setup:
	./scripts/db-setup.sh

# New coredraw_role password into .env; restart coredraw-api afterwards.
db-rotate-password:
	./scripts/db-setup.sh --rotate

# Migrations run against MIGRATION_URL (owner, direct host,
# options=-c search_path=coredraw). Run `make db-setup` once first.
# Needs the goose CLI: go install github.com/pressly/goose/v3/cmd/goose@latest
migrate-up:
	$(GOOSE) "$(MIGRATION_URL)" up

migrate-down:
	$(GOOSE) "$(MIGRATION_URL)" down

migrate-status:
	$(GOOSE) "$(MIGRATION_URL)" status

migrate-create:
	@test -n "$(name)" || (echo "usage: make migrate-create name=<snake_case>" && exit 1)
	goose -dir internal/db/migrations -s create $(name) sql

# Regenerates internal/db/sqlc from internal/db/queries + the migrations.
# Needs sqlc v1.31.1: go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
sqlc:
	sqlc generate

# Fails when the generated code is stale.
sqlc-check:
	sqlc diff

docker-build:
	docker build -f Dockerfile.coredraw-api --target api -t coredraw-api .
