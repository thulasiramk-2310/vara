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

# --- hub UI ------------------------------------------------------------------
# The Hub's web interface is the React app in frontend/. It is built here, from the
# lock file, so the image needs no network at run time and ships only static files.
FROM node:22-alpine AS ui
WORKDIR /ui
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# --- runtime -----------------------------------------------------------------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && adduser -D -u 10001 vara \
    && mkdir -p /data \
    && chown vara:vara /data

COPY --from=build /out/vara /usr/local/bin/vara
COPY --from=ui /ui/dist /srv/web
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
