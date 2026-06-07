# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Self-hosted personal game tracker. Import libraries from Steam and Xbox, track status/progress/sessions, no social features. Installable as a PWA. Built with Go + Gin, HTMX + Go Templates + TailwindCSS, PostgreSQL.

## Commands

```bash
# Run the app (migrations run automatically on startup)
go run cmd/main.go

# Run with Docker (recommended)
docker compose up -d

# Tidy dependencies
make tidy

# Run tests
go test ./...

# Run a single test
go test ./internal/... -run TestName
```

## Environment

Copy `.env.example` to `.env`. Required variables:
- `DATABASE_URL` — PostgreSQL connection string
- `SESSION_SECRET` — random 32-char string for session signing
- `STEAM_API_KEY` — Steam Web API key
- `XBOX_CLIENT_ID` / `XBOX_CLIENT_SECRET` — Xbox Live OAuth credentials
- `APP_PORT` — defaults to 8080

## Architecture

**Entry point** — `cmd/main.go`:
- Loads `.env`, connects to DB via `database.Connect()`, runs migrations via `database.RunMigrations()` (migrations are automatic on every startup, no separate command)
- Loads all `*.html` files recursively from `templates/` into a single `*template.Template`
- Sets up cookie-based sessions (`gin-contrib/sessions`), then registers routes

**Routes:**
- Public: `GET/POST /login`, `GET/POST /register`, `POST /logout`
- Protected by `handlers.AuthRequired()`: `GET /` (dashboard)

**Internal packages:**

- `internal/database/db.go` — `Connect()` returns a `*pgxpool.Pool`
- `internal/database/migrate.go` — `RunMigrations()` applies pending `migrations/*.sql` files in sorted order, each in a transaction; tracks applied files in the `schema_migrations` table
- `internal/models/` — domain structs and DB query functions together (no separate repository layer); handlers pass `db *pgxpool.Pool` into model functions directly
- `internal/handlers/` — Gin handlers; `AuthHandler` holds `db`; `middleware.go` contains `AuthRequired()`

**Templates** (`templates/`):
- Each `.html` file wraps its content in `{{define "folder/filename"}}...{{end}}` (e.g. `templates/auth/login.html` → `{{define "auth/login"}}`)
- Handlers render by that defined name: `c.HTML(200, "auth/login", gin.H{...})`
- Session data (`user_id`, `username`) is read from the Gin session and passed to templates via `gin.H`

**Migrations** (`migrations/`):
- Plain SQL files named `NNN_description.sql` (e.g. `001_create_users.sql`)
- Applied in alphabetical order; never re-applied once recorded in `schema_migrations`

**Auth:**
- Passwords hashed with bcrypt (`golang.org/x/crypto`)
- Session stores `user_id` (int64) and `username` (string); `AuthRequired()` checks for a non-nil `user_id`

## External APIs (not yet implemented)

| API | Purpose |
|-----|---------|
| Steam Web API | Import game library |
| Xbox Live API (OAuth2) | Import game library |
| IGDB API | Game metadata (cover art, genres) |
