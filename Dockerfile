# syntax=docker/dockerfile:1

# --- build stage ------------------------------------------------------------
# stratum is pure Go (no cgo): the SQL parser is hand-written, so no C toolchain
# is needed and we produce a fully static binary.
FROM golang:1.25-alpine AS build

WORKDIR /src

# Cache module downloads separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w" \
    -o /usr/local/bin/stratum \
    ./cmd/stratum

# --- runtime stage ----------------------------------------------------------
FROM alpine:latest

# git is needed only when the action is invoked with --push.
RUN apk add --no-cache git ca-certificates

COPY --from=build /usr/local/bin/stratum /usr/local/bin/stratum

ENTRYPOINT ["/usr/local/bin/stratum"]
