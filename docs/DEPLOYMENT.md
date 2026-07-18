# Installing & Deploying VARA

VARA ships as a single static Go binary with no runtime dependencies. The same
`vara` executable is both the **client** (like `git`) and the **server** (the
self-hosted Hub, like a GitHub instance).

- [Install the `vara` client](#install-the-vara-client)
- [Run your own Hub](#run-your-own-hub)
- [Data layout](#data-layout)
- [TLS & reverse proxies](#tls--reverse-proxies)

---

## Install the `vara` client

### Prebuilt binaries (recommended)

Every release publishes archives for Linux, macOS, and Windows on `amd64` and
`arm64` at **[github.com/thulasiramk-2310/vara/releases](https://github.com/thulasiramk-2310/vara/releases)**.
Download the archive for your platform, extract it, and put `vara` on your PATH.

### One-line install (Linux / macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/thulasiramk-2310/vara/main/scripts/install.sh | sh
```

Pin a version or install location with environment variables:

```sh
VARA_VERSION=v0.3.0 VARA_INSTALL="$HOME/.local/bin" \
  curl -fsSL https://raw.githubusercontent.com/thulasiramk-2310/vara/main/scripts/install.sh | sh
```

### With the Go toolchain

If you have Go 1.21+ installed:

```sh
go install github.com/thulasiramk-2310/vara/cmd/vara@latest
```

### From source

```sh
git clone https://github.com/thulasiramk-2310/vara
cd vara
go build -o vara ./cmd/vara
```

Verify any of the above with `vara --version`.

---

## Run your own Hub

The Hub is `vara serve` with the identity, authorization, repository, account,
and web-UI layers enabled. There are two supported ways to run it.

### Option A — Docker (recommended)

```sh
docker compose up -d --build
```

This builds the image, starts the Hub on `http://localhost:8080`, and stores all
state in the `vara-data` named volume. Bootstrap the first administrator (this
runs *on the host*, the only account that needs no pre-existing credentials).
Passing `--policy` grants the new account `manage-accounts`, `create-repo`, and
`list-repos` at server scope so it can administer the Hub over the wire:

```sh
docker compose exec hub vara account create \
  --accounts /data/accounts --policy /data/policy \
  --username admin --password 'choose-a-strong-password'
```

Open `http://localhost:8080`, sign in as `admin`, and you have a working Hub.
(Omit `--policy` and the account is created but can't administer accounts/repos
remotely until you grant it in `<policy>/_server.json`.)

To run the raw image without Compose:

```sh
docker build -t vara:local .
docker run -d --name vara -p 8080:8080 -v vara-data:/data vara:local
docker exec vara vara account create \
  --accounts /data/accounts --policy /data/policy \
  --username admin --password 'choose-a-strong-password'
```

### Option B — the binary directly

```sh
mkdir -p srv/{repos,policy,meta,accounts}

# bootstrap the first admin (--policy grants server-scope admin capabilities)
vara account create \
  --accounts srv/accounts --policy srv/policy \
  --username admin --password 'choose-a-strong-password'

vara serve \
  --addr :8080 \
  --root     srv/repos \
  --policy   srv/policy \
  --meta     srv/meta \
  --accounts srv/accounts \
  --hub      web
```

Run it under a process supervisor (systemd, runit, supervisord) for production.
A minimal systemd unit:

```ini
[Unit]
Description=VARA Hub
After=network.target

[Service]
ExecStart=/usr/local/bin/vara serve --addr :8080 \
  --root /var/lib/vara/repos --policy /var/lib/vara/policy \
  --meta /var/lib/vara/meta --accounts /var/lib/vara/accounts \
  --hub /usr/share/vara/web
User=vara
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

---

## Data layout

Every flag points at a directory; each holds one concern, and **none of it lives
inside a repository** — policy, metadata, and accounts are server-managed and are
never pushable (a core VARA invariant).

| Flag | Directory | Holds |
|------|-----------|-------|
| `--root`     | `repos/`    | the repositories themselves (object stores + refs) |
| `--policy`   | `policy/`   | authorization policy (RFC-0018) — who can do what |
| `--meta`     | `meta/`     | repository metadata & ownership (RFC-0019) |
| `--accounts` | `accounts/` | accounts, sessions, API tokens (RFC-0020, argon2id hashes at rest) |
| `--hub`      | `web/`      | the static Hub UI (RFC-0021), served same-origin |

Back up the whole parent directory to back up the Hub. `--meta` requires
`--policy` (ownership is seeded as policy). Omitting `--accounts`/`--hub` yields a
headless API server; omitting `--policy` yields an anonymous allow-all server
(fine only for a trusted network).

---

## TLS & reverse proxies

The Hub speaks plain HTTP and is designed to sit behind a TLS-terminating
reverse proxy (nginx, Caddy, Traefik). Session cookies are issued `HttpOnly`,
`SameSite=Strict`, and `Secure` — the `Secure` flag is set when the request
arrives over TLS **or** carries `X-Forwarded-Proto: https`, so a correctly
configured proxy preserves it.

Minimum proxy requirements:

- terminate TLS and forward to the Hub's `--addr`;
- set `X-Forwarded-Proto: https` on forwarded requests (so cookies stay `Secure`);
- forward the `Host` header unchanged.

Example Caddy config:

```
hub.example.com {
    reverse_proxy localhost:8080
}
```

Caddy sets `X-Forwarded-Proto` automatically and provisions TLS for you.

> **Note:** only enable `X-Forwarded-Proto` trust when the Hub is actually behind
> a proxy you control — a client that can reach the Hub directly could otherwise
> spoof the header. Bind the Hub to `localhost` (or an internal interface) and let
> only the proxy reach it.
