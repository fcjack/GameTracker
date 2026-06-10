FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o game-tracker cmd/main.go

FROM node:22-alpine AS assets

WORKDIR /app

COPY tailwind.config.js tailwind.css ./
COPY templates ./templates
RUN npx -y tailwindcss@3.4.17 -c tailwind.config.js -i tailwind.css -o app.css --minify

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=65532:65532 /app/game-tracker .
COPY --from=builder --chown=65532:65532 /app/templates ./templates
COPY --from=builder --chown=65532:65532 /app/static ./static
COPY --from=assets --chown=65532:65532 /app/app.css ./static/css/app.css
COPY --from=builder --chown=65532:65532 /app/migrations ./migrations

EXPOSE 8080

ENTRYPOINT ["./game-tracker"]
