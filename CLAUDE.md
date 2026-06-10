# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Self-hosted personal game tracker. Import libraries from Steam and Xbox, track status/progress/sessions, no social features. Installable as a PWA. Built with Go + Gin, HTMX + Go Templates + TailwindCSS, PostgreSQL.

## Commits and releases

All commits merged to `main` **must** use [Conventional Commits](https://www.conventionalcommits.org/). Release Please parses commit messages to determine version bumps and generate changelogs; non-conventional messages are ignored.

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Common types:
- `feat:` — new feature (minor bump before v1.0.0)
- `fix:` — bug fix (patch bump)
- `chore:` — tooling, docs, deps (included in changelog; usually no version bump unless combined with `Release-As`)
- `docs:` — documentation only
- `refactor:`, `test:`, `ci:` — internal changes

Examples:
```
feat: add Xbox library import
fix: prevent duplicate Steam game entries
chore: update golangci-lint
```

To force a specific version, add a footer to the commit body:
```
Release-As: 0.2.0
```

Releases are automated via `.github/workflows/release-please.yml` on push to `main`. Release Please opens a PR that bumps `internal/version/version.go`, `CHANGELOG.md`, and `.release-please-manifest.json`. Merging that PR creates the GitHub release and tag (e.g. `v0.1.1`). The app displays the current version in the sidebar and auth pages.

## Commands

```bash
# Run the app (migrations run automatically on startup)
go run cmd/main.go

# Run with Docker (recommended)
docker compose up -d

# Reset database (removes volumes, recreates from scratch)
make reset

# Tidy dependencies
make tidy

# Run tests
go test ./...

# Run a single test
go test ./internal/... -run TestName

# Rebuild static/css/app.css after changing Tailwind classes in templates/ (requires Node.js)
make css
```

Tailwind CSS is built locally (no CDN): `tailwind.config.js` + `tailwind.css` compile to `static/css/app.css` via `make css` (also rebuilt in the Docker image's `assets` stage). HTMX is vendored at `static/js/htmx.min.js`. Both are referenced with `?v={{.version}}` for cache busting.

## Environment

Copy `.env.example` to `.env`. Required variables:
- `DATABASE_URL` — PostgreSQL connection string
- `SESSION_SECRET` — random 32-char string for session signing
- `ENCRYPTION_KEY` — 32-byte key hex-encoded (generate: `python3 -c "import os; print(os.urandom(32).hex())"`) for encrypting secrets before DB storage
- `STEAM_API_KEY` — Steam Web API key
- `XBOX_CLIENT_ID` / `XBOX_CLIENT_SECRET` — Xbox Live OAuth credentials
- `APP_PORT` — defaults to 8080

## Architecture

**Entry point** — `cmd/main.go`:
- Loads `.env`, connects to DB via `database.Connect()`, runs migrations via `database.RunMigrations()` (migrations are automatic on every startup, no separate command)
- Loads all `*.html` files recursively from `templates/` into a single `*template.Template`
- Sets up cookie-based sessions (`gin-contrib/sessions`), then registers routes

**Routes:**
- Public: `GET /`, `GET/POST /login`, `GET/POST /register`, `POST /logout`
- Protected by `handlers.AuthRequired()`:
  - Dashboard: `GET /dashboard`, `GET /dashboard/stats`
  - Library: `GET /library`, `GET /library/games`, `GET /library/search` (search user's imported games), `GET /library/search/igdb` (IGDB search — dashboard only), `POST /library/games`, `DELETE /library/games/:game_id`, status/complete endpoints
  - Profile, Steam auth/import endpoints

**Internal packages:**

- `internal/database/db.go` — `Connect()` returns a `*pgxpool.Pool`
- `internal/database/migrate.go` — `RunMigrations()` applies pending `migrations/*.sql` files in sorted order, each in a transaction; tracks applied files in the `schema_migrations` table
- `internal/crypto/crypto.go` — AES-256-GCM encryption/decryption with random nonces; `NewEncrypter(keyHex)` validates key length, `Encrypt(plaintext)` returns base64, `Decrypt(ciphertext)` reverses it
- `internal/models/` — domain structs and DB query functions together (no separate repository layer); handlers pass `db *pgxpool.Pool` into model functions directly
  - `user.go` — User struct, `CreateUser()`, `GetUserByUsername()`, `CheckPassword()`
  - `linked_account.go` — LinkedAccount struct, `UpsertLinkedAccount()`, `GetLinkedAccount()`, `DeleteLinkedAccount()`, `ListLinkedAccounts()`
  - `game.go` — Game/UserGame structs, `FindOrCreateGame()`, `ListUserGames()`, `SearchUserGames()`, `AddToLibrary()`, status helpers
- `internal/handlers/` — Gin handlers; `LibraryHandler` holds `db` + `igdb.Client`; `middleware.go` contains `AuthRequired()`

**Templates** (`templates/`):
- Each `.html` file wraps its content in `{{define "folder/filename"}}...{{end}}` (e.g. `templates/auth/login.html` → `{{define "auth/login"}}`)
- Handlers render by that defined name: `c.HTML(200, "auth/login", gin.H{...})`
- Session data (`user_id`, `username`) is read from the Gin session and passed to templates via `gin.H`

**Migrations** (`migrations/`):
- Plain SQL files named `NNN_description.sql` (e.g. `001_create_users.sql`)
- Applied in alphabetical order; never re-applied once recorded in `schema_migrations`

**Auth:**
- Passwords hashed with bcrypt (`golang.org/x/crypto`); minimum 5 characters
- Session stores `user_id` (int64) and `username` (string); `AuthRequired()` checks for a non-nil `user_id`
- External accounts linked via `linked_accounts` table; supports multiple providers (Steam, Xbox) per user; tokens stored encrypted

## Database

**Tables:**
- `users` — user accounts with bcrypt-hashed passwords
- `linked_accounts` — external provider identities (Steam, Xbox) with encrypted access/refresh tokens; UNIQUE(user_id, provider); supports upsert on re-auth
- `games` — canonical game records (IGDB id, Steam app id, metadata)
- `user_games` — per-user library entries with status, platform, completion/dropped dates; UNIQUE(user_id, game_id)
- `categories` — IGDB game categories
- `schema_migrations` — applied migration tracking

## External APIs

| API | Purpose | Status |
|-----|---------|--------|
| Steam OpenID 2.0 | Link Steam account, get SteamID64 | Implemented |
| Steam Web API | Import game library via encrypted SteamID | Implemented |
| Xbox Live API (OAuth2) | Link Xbox account, get tokens | Planned |
| IGDB API | Dashboard search + metadata when adding/importing games | Implemented |
