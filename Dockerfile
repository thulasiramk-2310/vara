# Multi-stage build for the VARA Hub server (a single static binary + web UI).
#
#   docker build -t vara:local .
#   docker run -p 8080:8080 -v vara-data:/data vara:local
#
# See docs/DEPLOYMENT.md for the data layout and first-admin bootstrap.

# --- build -------------------------------------------------------------------
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/vara ./cmd/vara

# --- runtime -----------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 vara \
    && mkdir -p /data \
    && chown vara:vara /data

COPY --from=build /out/vara /usr/local/bin/vara
COPY --from=build /src/web /srv/web
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

USER vara
WORKDIR /data
EXPOSE 8080

# The entrypoint ensures the data subdirectories exist, then execs vara with
# the arguments below. Override CMD to run a different subcommand.
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["serve", \
     "--addr", ":8080", \
     "--root", "/data/repos", \
     "--policy", "/data/policy", \
     "--meta", "/data/meta", \
     "--accounts", "/data/accounts", \
     "--hub", "/srv/web"]
