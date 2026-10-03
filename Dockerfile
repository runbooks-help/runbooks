FROM golang:1.26 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go tool templ generate ./... && \
    go run ./cmd/css && \
    go run ./cmd/js && \
    CGO_ENABLED=0 GOOS=linux go build -o runbooks .

FROM alpine:3
# git for the notes sync push; openssh-client for SSH remotes; ca-certificates
# for HTTPS remotes.
RUN apk add --no-cache git openssh-client ca-certificates && \
    adduser -D -u 10001 -h /app runbooks
WORKDIR /app
COPY --from=builder /app/runbooks .
COPY --from=builder /app/content ./content
COPY --from=builder /app/LICENSE /app/NOTICE /app/DEPENDENCIES.md /app/THIRD_PARTY_NOTICES.md ./
# The SQLite identity database lives here; mount a volume to persist it.
RUN mkdir -p /app/data && chown -R runbooks:runbooks /app
VOLUME /app/data
USER runbooks
EXPOSE 8090
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8090/healthz || exit 1
CMD ["./runbooks"]
