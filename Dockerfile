# services/<svc>/Dockerfile
ARG SERVICE_DIR=services/user-service
FROM golang:1.24-alpine AS builder

# metadata
LABEL maintainer="you@example.com"

# set working dir for repo root copy
WORKDIR /src

# copy go.mod and go.sum for the specific service (so mod download can run and cache)
# when building with repo root as context, these exist at ${SERVICE_DIR}/go.mod
ARG SERVICE_DIR
COPY ${SERVICE_DIR}/go.mod ${SERVICE_DIR}/go.sum ./

# download dependencies (uses the go.mod from the service)
RUN apk add --no-cache git && go env -w GOPROXY="https://proxy.golang.org,direct" && go mod download

# copy the whole repo into the build context so local modules (siblings) are available
COPY . .

# build from the service directory
WORKDIR /src/${SERVICE_DIR}

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

ENV PORT=8081
EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD ["/home/appuser/app","-health"] || exit 1

ENTRYPOINT ["/home/appuser/app"]
