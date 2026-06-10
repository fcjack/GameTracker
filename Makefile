GOLANGCI_LINT_VERSION ?= v2.4.0
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: run migrate-up migrate-down migration tidy reset rebuild redeploy deploy deploy-down deploy-logs fmt fmt-check lint lint-fix pre-commit

run:
	go run cmd/main.go

migrate-up:
	go run cmd/main.go migrate up

migrate-down:
	go run cmd/main.go migrate down

reset:
	docker compose down -v
	docker compose up -d

rebuild:
	docker compose build app

redeploy:
	docker compose up -d --build --no-deps app

deploy:
	docker compose -f docker-compose.deploy.yml up -d --build

deploy-down:
	docker compose -f docker-compose.deploy.yml down

deploy-logs:
	docker compose -f docker-compose.deploy.yml logs -f

migration:
	@test -n "$(name)" || (echo "Error: name is required. Usage: make migration name=<name>"; exit 1)
	@n=$$(ls migrations/*.sql 2>/dev/null | grep -v '\.down\.sql' | wc -l | tr -d ' '); \
	 seq=$$(printf "%03d" $$(($$n + 1))); \
	 touch "migrations/$${seq}_$(name).sql"; \
	 touch "migrations/$${seq}_$(name).down.sql"; \
	 echo "Created migrations/$${seq}_$(name).sql"; \
	 echo "Created migrations/$${seq}_$(name).down.sql"

tidy:
	go mod tidy

fmt:
	gofmt -s -w .

fmt-check:
	@test -z "$$(gofmt -s -l .)" || (echo "Files need formatting. Run: make fmt"; exit 1)

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run --fix ./...

pre-commit: fmt lint-fix
