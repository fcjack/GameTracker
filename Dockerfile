FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o game-tracker cmd/main.go

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder --chown=65532:65532 /app/game-tracker .
COPY --from=builder --chown=65532:65532 /app/templates ./templates
COPY --from=builder --chown=65532:65532 /app/static ./static
COPY --from=builder --chown=65532:65532 /app/migrations ./migrations

EXPOSE 8080

ENTRYPOINT ["./game-tracker"]
