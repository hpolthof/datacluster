FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o datacluster .

# ─── Final image ──────────────────────────────────────────────────────────────
FROM alpine:3.20

# CA certs needed for TLS connections to PostgreSQL servers
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/datacluster /datacluster

VOLUME ["/data"]
EXPOSE 8000

ENV DATA_DIR=/data
ENV PORT=8000

ENTRYPOINT ["/datacluster"]
