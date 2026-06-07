# 🎮 Game Tracker

A self-hosted personal game tracking application to manage and track gaming progress across multiple platforms. Installable as a PWA on mobile devices.

## Overview

Game Tracker is a lightweight web application built with Go that allows you to import your game libraries from Steam and Xbox, track your gaming progress and manage your personal backlog without any social features or third party dependencies.

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
- Track game status (Playing, Finished, Dropped, Backlog)
- Track progress and gaming sessions
- Simple personal dashboard

### PWA
- Mobile first responsive design
- Installable on mobile devices
- Offline support via Service Worker

---

## External APIs

| API | Purpose |
|-----|---------|
| **Steam API** | Game library import |
| **Xbox API** | Game library import |
| **IGDB API** | Game metadata |

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

- Go 1.22+
- PostgreSQL 16+
- Docker and Docker Compose
- Node.js (TailwindCSS only)

### Environment Variables

Copy `.env.example` to `.env` and fill in the values:

```env
DATABASE_URL=postgres://user:password@localhost:5432/gametracker
STEAM_API_KEY=
XBOX_CLIENT_ID=
XBOX_CLIENT_SECRET=
APP_PORT=8080
```

### Running With Docker (Recommended)

```bash
# Clone the repository
git clone https://github.com/yourusername/game-tracker.git
cd game-tracker

# Copy environment variables
cp .env.example .env

# Start all services
docker compose up -d
```

Application will be available at http://localhost:8080

### Running Without Docker

```bash
# Clone the repository
git clone https://github.com/yourusername/game-tracker.git
cd game-tracker

# Copy environment variables
cp .env.example .env

# Install dependencies
go mod download

# Start the application (migrations run automatically on startup)
go run cmd/main.go
```

### Docker Deployment

```dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o game-tracker cmd/main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/game-tracker .
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./game-tracker"]
```

### PWA Installation

1. Open the application in your mobile browser
2. Tap **Add to Home Screen**
3. Enjoy the native app experience

> **Note:** HTTPS is required for PWA features in production
