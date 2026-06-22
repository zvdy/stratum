# syntax=docker/dockerfile:1

# --- build stage ------------------------------------------------------------
# pg_query_go vendors the PostgreSQL parser in C, so cgo (gcc + musl-dev) is
# required to build. We build statically against musl so the binary runs as-is
# on the alpine runtime image.
FROM golang:1.25-alpine AS build

RUN apk add --no-cache gcc musl-dev

WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 go build \
    -ldflags="-s -w" \
    -o /usr/local/bin/stratum \
    ./cmd/stratum

# --- runtime stage ----------------------------------------------------------
FROM alpine:latest

# git is needed only when the action is invoked with --push.
RUN apk add --no-cache git ca-certificates

COPY --from=build /usr/local/bin/stratum /usr/local/bin/stratum

ENTRYPOINT ["/usr/local/bin/stratum"]
