FROM golang:1.27 AS builder

# The version the binary reports via --version; CI passes the tag (or ref).
ARG VERSION=dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go tool templ generate ./... && \
    go run ./cmd/designsystem && \
    go run ./cmd/css && \
    go run ./cmd/js && \
    go run ./cmd/notices -version "${VERSION}" && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.version=${VERSION}" -o runbooks .

FROM alpine:3 AS runtime

# OCI labels so an image traces back to the release it was built from.
ARG VERSION=dev
ARG REVISION=
LABEL org.opencontainers.image.title="Runbooks" \
      org.opencontainers.image.source="https://github.com/runbooks-help/runbooks" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

# The image needs no system git: content and notes sync use go-git, and SSH
# remotes use its pure-Go client. ca-certificates is for HTTPS remotes.
RUN apk add --no-cache ca-certificates && \
    adduser -D -u 10001 -h /app runbooks
WORKDIR /app
COPY --from=builder /app/runbooks .
COPY --from=builder /app/LICENSE /app/NOTICE /app/DEPENDENCIES.md /app/THIRD_PARTY_NOTICES.md ./
# Content is the operator's: the image ships an empty dir they mount (CONTENT_DIR),
# never the maintainer's runbooks. The SQLite identity database lives in /app/data.
RUN mkdir -p /app/content /app/data && chown -R runbooks:runbooks /app
VOLUME /app/data
VOLUME /app/content
USER runbooks
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8090/healthz || exit 1
CMD ["./runbooks"]

# Derived docs-site image: the runtime plus a build-time seed of the public docs
# repo. The app reads it with CONTENT_SOURCE=git (CONTENT_GIT_PATH=docs) and
# refreshes in the background; the seed keeps cold start offline-safe and
# independent of the remote. Build with `--target docs`.
FROM alpine:3 AS docs-seed
RUN apk add --no-cache git && \
    git clone --branch main https://github.com/runbooks-help/runbooks-docs /seed

FROM runtime AS docs
COPY --from=docs-seed --chown=runbooks:runbooks /seed /app/data/content
