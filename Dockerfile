# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/api ./cmd/api

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=builder /app/api /app/api
USER appuser
EXPOSE 8080
ENV PORT=8080
ENTRYPOINT ["/app/api"]
