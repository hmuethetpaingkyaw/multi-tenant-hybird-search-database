.PHONY: up down migrate test test-short build

up:        ## build and start db, migrations, and the API
	docker compose up --build -d

down:      ## stop everything (add -v manually to also drop the database volume)
	docker compose down

migrate:   ## re-run migrations as the owner role
	docker compose run --rm migrate

build:
	go build -o bin/api ./cmd/api

test:      ## unit + integration tests (integration needs Docker)
	go test ./...

test-short: ## unit tests only
	go test -short ./...
