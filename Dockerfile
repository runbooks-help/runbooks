FROM golang:1.26 AS builder

WORKDIR /app

RUN go install github.com/a-h/templ/cmd/templ@v0.3.1020

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN templ generate ./... && \
    go run ./cmd/css && \
    go run ./cmd/js && \
    CGO_ENABLED=0 GOOS=linux go build -o runbooks .

FROM alpine:3
# git for the notes sync push; openssh-client for SSH remotes; ca-certificates
# for HTTPS remotes.
RUN apk add --no-cache git openssh-client ca-certificates
WORKDIR /app
COPY --from=builder /app/runbooks .
COPY --from=builder /app/content ./content
EXPOSE 8091
CMD ["./runbooks"]
