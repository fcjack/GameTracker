<p align="center">
  <img src="static/icons/icon-192.png" alt="Helios Gaming tracker" width="128" height="128">
</p>

# Helios Gaming tracker

[![CI](https://github.com/fcjack/GameTracker/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/fcjack/GameTracker/actions/workflows/ci.yml)

A self-hosted personal game tracking application to manage and track gaming progress across multiple platforms. Installable as a PWA on mobile devices.

## Overview

Helios Gaming tracker is a lightweight web application built with Go that allows you to import your game libraries from Steam and Xbox, search IGDB to add new games from the dashboard, filter your existing collection on the library page, track your gaming progress, and manage your personal backlog without any social features or third-party dependencies.

---

## Tech Stack

| Layer | Technology |
|-------|------------|
| **Backend** | Go 1.25+ |
| **Router** | Gin |
| **Frontend** | HTMX + Go Templates + TailwindCSS |
| **Database** | PostgreSQL 16+ |
| **Containerization** | Docker + Docker Compose |

---

## Features

### Core
- Import game library from Steam and Xbox
- Search IGDB from the dashboard and add games to your library
- Search your imported library by name or platform on the library page
- Choose platform when adding multi-platform games
- Track game status (Playing, Completed, Dropped, Backlog)
- Dashboard with live stats and library management
- Group library by platform or completion year

### PWA
- Mobile-first responsive design
- Installable on mobile devices with custom Helios icon
- Offline support via Service Worker

---

## External APIs

| API | Purpose |
|-----|---------|
| **Steam API** | Game library import |
| **Xbox API** | Game library import |
| **IGDB API** | Dashboard game search and metadata when adding titles |

---

## Project Structure

```
game-tracker/
├── cmd/
│   └── main.go
├── internal/
│   ├── handlers/
│   ├── models/
│   └── database/
├── templates/
├── static/
│   ├── icons/
│   ├── manifest.json
│   └── service-worker.js
├── migrations/
├── Dockerfile
├── docker-compose.yml
└── .env.example
```

---

## Getting Started

### Prerequisites

- Go 1.25+
- PostgreSQL 16+
- Docker and Docker Compose

### Environment Variables

Copy `.env.example` to `.env` and fill in the values:

```env
DATABASE_URL=postgres://user:password@localhost:5432/gametracker
SESSION_SECRET=
ENCRYPTION_KEY=
STEAM_API_KEY=
XBOX_CLIENT_ID=
XBOX_CLIENT_SECRET=
TWITCH_CLIENT_ID=
TWITCH_CLIENT_SECRET=
APP_PORT=8080
# Optional: periodically re-sync linked libraries in the background
LIBRARY_SYNC_ENABLED=false
LIBRARY_SYNC_INTERVAL=6h
```

When `LIBRARY_SYNC_ENABLED=true`, a background scheduler re-imports every linked
library on the interval set by `LIBRARY_SYNC_INTERVAL` (Go duration, default `6h`,
minimum `15m`). It reuses the idempotent import pipeline, so it only adds newly
acquired games and never collides with a manual import already in progress.

### Running With Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/fcjack/GameTracker.git
cd GameTracker

# Copy environment variables
cp .env.example .env

# Start all services
docker compose up -d
```

Application will be available at http://localhost:8080

### Running Without Docker

```bash
# Clone the repository
git clone https://github.com/fcjack/GameTracker.git
cd GameTracker

# Copy environment variables
cp .env.example .env

# Install dependencies
go mod download

# Start the application (migrations run automatically on startup)
go run cmd/main.go
```

### Useful Make Commands

```bash
make run       # Run locally without Docker
make redeploy  # Rebuild and restart the app container
make reset     # Reset database volumes and restart
make tidy      # Tidy Go module dependencies
go test ./...  # Run tests
```

### PWA Installation

1. Open the application in your mobile browser
2. Tap **Add to Home Screen**
3. The Helios icon will appear on your home screen

> **Note:** HTTPS is required for PWA features in production
