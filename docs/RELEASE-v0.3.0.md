# VARA v0.3.0 Release Notes

**Released**: 2026-07-18
**Tag**: `v0.3.0`
**Codename**: Installable & Self-Hostable

---

## What v0.3.0 Is

v0.2.0 completed the backend platform. **v0.3.0 makes it something a stranger can
actually install and run.** Until now, using VARA meant cloning the repo and
compiling it yourself. This release adds the distribution layer: prebuilt
binaries, a one-line installer, and a Docker image for self-hosting the Hub.

Nothing in the engine, transport, or any RFC-governed protocol surface changed —
this release is packaging and delivery around the existing v0.2 platform. The
`pkg/*` engine and transport interface still have an empty diff.

The same single static `vara` binary is both the **client** (like `git`) and the
**Hub server** (like a self-hosted GitHub instance).

---

## Installing

**Prebuilt binary** (Linux · macOS · Windows, `amd64`/`arm64`):

```sh
curl -fsSL https://raw.githubusercontent.com/thulasiramk-2310/vara/main/scripts/install.sh | sh
```

Or download an archive from the [releases page](https://github.com/thulasiramk-2310/vara/releases).

**With Go 1.21+:**

```sh
go install github.com/thulasiramk-2310/vara/cmd/vara@latest
```

`vara --version` now reports the exact release tag (stamped at build time).

---

## Running your own Hub

```sh
docker compose up -d --build
docker compose exec hub vara account create \
  --accounts /data/accounts --policy /data/policy \
  --username admin --password 'choose-a-strong-password'
# open http://localhost:8080
```

The Hub's state lives entirely under one data directory
(`repos/`, `policy/`, `meta/`, `accounts/`) — see
[`docs/DEPLOYMENT.md`](DEPLOYMENT.md) for the full layout, a systemd unit, and
TLS / reverse-proxy guidance.

---

## What's in this release

| Area | What shipped |
|------|--------------|
| **Releases** | `.goreleaser.yaml` cross-compiles the binary for 3 OSes × 2 arches with SHA-256 checksums; `.github/workflows/release.yml` publishes a GitHub Release on every `v*` tag |
| **Install** | `scripts/install.sh` — OS/arch detection, latest-or-pinned version, PATH-aware install dir |
| **Version** | `vara --version` reports the release tag via `-ldflags -X main.version` |
| **Docker** | Multi-stage build to a non-root Alpine image running `vara serve --hub`; entrypoint provisions the data dirs so a fresh volume works first-run |
| **Compose** | `docker-compose.yml` — one command to run the Hub with a persistent volume |
| **Docs** | `docs/DEPLOYMENT.md` — install paths, data layout, first-admin bootstrap, systemd, TLS/reverse-proxy |

---

## Verification

- `go build ./...` green; version injection confirmed via `-ldflags`.
- Docker image builds and serves the Hub UI (`GET /` → 200 `text/html`).
- Full in-container end-to-end: bootstrap admin → cookie login (`201`) →
  `whoami` resolves `admin`/`bearer` → create + list repository (`201`), with no
  restart required — the account store and authorization policy hot-reload from
  disk.

---

## Compatibility

Pre-1.0 still holds: the on-disk object/index format (RFC-0002/0005) is stable and
the transport protocol carries a compatibility promise (`docs/COMPATIBILITY.md`).
This release changes no wire or storage format — it only changes how you get and
run the binary.

---

## What's next

- **RFC-0022 Repository Browser** — tree/blob/file-history views over the read API.
- **RFC-0023 Diff Viewer**, **RFC-0024 Search**.
- Package-manager distribution (Homebrew/Scoop) once there's demand — GoReleaser
  can generate the formulas from the same config.
