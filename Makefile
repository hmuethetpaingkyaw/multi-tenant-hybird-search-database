.PHONY: up down migrate seed test test-short build

up:        ## build and start db, migrations, and the API
	docker compose up --build -d

down:      ## stop everything (add -v manually to also drop the database volume)
	docker compose down

migrate:   ## re-run migrations as the owner role
	docker compose run --rm migrate

seed:      ## create dev tenants A/B/C + users and print bearer tokens (needs `make up`)
	@set -a; . ./.env; set +a; \
	SEED_DATABASE_URL="postgres://search_owner:$$OWNER_PASSWORD@localhost:$${DB_PORT:-5433}/search?sslmode=disable" \
	go run ./cmd/seed

build:
	go build -o bin/api ./cmd/api

test:      ## unit + integration tests (integration needs Docker)
	go test ./...

test-short: ## unit tests only
	go test -short ./...
