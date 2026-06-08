.PHONY: run migrate-up migrate-down migration tidy reset

run:
	go run cmd/main.go

migrate-up:
	go run cmd/main.go migrate up

migrate-down:
	go run cmd/main.go migrate down

reset:
	docker compose down -v
	docker compose up -d

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
