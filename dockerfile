FROM golang:1.25-alpine AS builder

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