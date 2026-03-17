# services/rider-service/Dockerfile
FROM golang:1.24-alpine AS builder

# metadata
LABEL maintainer="you@example.com"

# set working dir
WORKDIR /src

# copy go.mod and go.sum for dependency caching
COPY go.mod go.sum ./

# download dependencies
RUN apk add --no-cache git && go env -w GOPROXY="https://proxy.golang.org,direct" && go mod download

# copy source code
COPY . .

# disable CGO and build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/app .

# disable CGO and build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/app .

# ---- runtime image ----
FROM alpine:latest

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
RUN apk add --no-cache ca-certificates

WORKDIR /home/appuser

# copy binary from builder
COPY --from=builder /app/app /home/appuser/app

# do NOT copy .env (we write it at build-time if needed, or provide at runtime)
# COPY .env .

RUN chown appuser:appgroup /home/appuser/app && chmod +x /home/appuser/app

USER appuser

ENV PORT=8083
EXPOSE 8083

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD ["/home/appuser/app", "-health"]

ENTRYPOINT ["/home/appuser/app"]
