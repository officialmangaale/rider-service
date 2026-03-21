# services/rider-service/Dockerfile
FROM golang:1.24-alpine AS builder

LABEL maintainer="you@example.com"

WORKDIR /src

COPY go.mod go.sum ./

RUN apk add --no-cache git && go env -w GOPROXY="https://proxy.golang.org,direct" && go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/app ./cmd/server

# ---- runtime image ----
FROM alpine:latest

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN apk add --no-cache ca-certificates

WORKDIR /home/appuser

COPY --from=builder /app/app /home/appuser/app

RUN chown appuser:appgroup /home/appuser/app && chmod +x /home/appuser/app

USER appuser

EXPOSE 8084

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://localhost:8084/health || exit 1

ENTRYPOINT ["/home/appuser/app"]
