GOLANGCI_LINT_VERSION ?= v2.4.0
GOLANGCI_LINT = go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: run css migrate-up migrate-down migration tidy reset rebuild redeploy deploy deploy-down deploy-logs fmt fmt-check lint lint-fix check fix pre-commit install-hooks

run:
	go run cmd/main.go

# Regenerate static/css/app.css from templates (requires Node.js).
# Run after adding/changing Tailwind classes in templates/.
css:
	npx -y tailwindcss@3.4.17 -c tailwind.config.js -i tailwind.css -o static/css/app.css --minify

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

# Formatting and linting (matches CI lint job).
fmt:
	gofmt -s -w .

fmt-check:
	@files=$$(gofmt -s -l .); \
	if [ -n "$$files" ]; then \
		echo "Files need formatting:"; \
		echo "$$files"; \
		echo "Run: make fmt"; \
		exit 1; \
	fi

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run --fix ./...

# Verify only — same checks as CI (fmt-check + lint).
check: fmt-check lint

# Auto-fix formatting and linter issues where supported.
fix: fmt lint-fix

pre-commit: fix

install-hooks:
	@chmod +x .githooks/pre-commit .githooks/pre-push
	@git config --local core.hooksPath .githooks
	@echo "Installed git hooks from .githooks (core.hooksPath=.githooks)"
	@echo "  pre-commit: auto-fix fmt/lint on commit"
	@echo "  pre-push:   run make check when pushing to main"
