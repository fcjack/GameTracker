# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Self-hosted personal game tracker. Import libraries from Steam and Xbox, track status/progress/sessions, no social features. Installable as a PWA. Built with Go + Gin, HTMX + Go Templates + TailwindCSS, PostgreSQL.

## Commands

```bash
# Run (without Docker)
go run cmd/main.go

# Run migrations
go run cmd/migrate/main.go

# Build
go build -o game-tracker cmd/main.go

# Run with Docker (recommended)
docker compose up -d

# Download dependencies
go mod download

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

**Entry points:**
- `cmd/main.go` — starts the Gin server, loads `.env`, registers routes
- `cmd/migrate/main.go` — runs SQL migrations from `migrations/` against the database

**Internal packages** (`internal/`):
- `handlers/` — Gin route handlers; one file per feature area (auth, dashboard, games, sessions, imports)
- `models/` — domain structs and business logic; no DB access
- `database/` — pgx/v5 connection pool and query functions; called only from handlers

**Templates** (`templates/`): Go HTML templates, organized by feature (`auth/`, `dashboard/`). Rendered server-side by handlers. HTMX swaps are partial templates returned from the same handler routes.

**Static** (`static/`): `manifest.json` and `service-worker.js` for PWA support.

**Migrations** (`migrations/`): Plain SQL files run in order by `cmd/migrate/main.go`.

**Key dependencies:**
- `gin-gonic/gin` — HTTP router and middleware
- `gin-contrib/sessions` — cookie-based session store
- `jackc/pgx/v5` — PostgreSQL driver (no ORM)
- `joho/godotenv` — loads `.env` at startup
- `golang.org/x/crypto` — password hashing

## External APIs

| API | Use |
|-----|-----|
| Steam Web API | Import game library via `steamid` |
| Xbox Live API (OAuth2) | Import game library; requires OAuth flow |
| IGDB API | Fetch game metadata (cover art, genres, etc.) |
