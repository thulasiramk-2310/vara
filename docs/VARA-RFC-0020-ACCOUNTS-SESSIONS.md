VARA RFC: 0020
Title: Accounts & Sessions
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-07-12
Last Updated: 2026-07-12
Depends On: RFC-0016, RFC-0017, RFC-0018, RFC-0019
Supersedes: None
Superseded By: None

# 1. Vision & Purpose

RFC-0017 defined *how a credential becomes an identity* and left the credential
store abstract: its `IdentitySource` interface validates a `Credential` and
returns an `Identity`, but the only sources shipped were static maps wired in at
process start. This document gives that interface a real, persistent backing:
**accounts** (a durable subject with a password), **sessions** (a credential
minted by an interactive login), and **API tokens** (a credential minted for
automation) — all revocable. It is the last foundational Platform RFC; above it
sit user-facing features, not more plumbing.

**The one-sentence scope:**

> RFC-0020 defines durable accounts, the sessions and API tokens a caller obtains
> from them, their lifecycles, and the secret-handling rules for all three,
> exposed as persistent `IdentitySource` implementations that slot into RFC-0017
> unchanged. It answers "who are the durable subjects of this host, and how do
> they prove it over time?" — nothing about *what* a subject may do (RFC-0018) and
> nothing about the `Identity` type or pipeline (RFC-0017).

**Design stance — RFC-0020 fills RFC-0017's interface; it does not change it.**

> Authentication still terminates above the transport, still produces the exact
> `Identity{ID, Method}` of RFC-0017, and still runs in the fixed
> Authenticate→Authorize→act pipeline. RFC-0020 only supplies richer *sources*: a
> password-checking source and a bearer-credential source backed by a persistent,
> revocable store. The engine, the transport, and the `Identity` type are
> untouched — this RFC is proven, like 0017/0018/0019 before it, by an unchanged
> engine.

```
   Authorization: Basic  / Bearer
             │
             ▼
   ┌───────────────────────────┐
   │ RFC-0017 preamble         │  parse → Authenticate (unchanged)
   └───────────────────────────┘
             │  delegates to
             ▼
   ┌───────────────────────────┐
   │ RFC-0020 IdentitySources  │  AccountSource (password) · CredentialSource
   │                           │  (session/token) — persistent, revocable
   └───────────────────────────┘
             │  returns
             ▼
        Identity{ID, Method}        ← RFC-0017 type, unchanged → feeds RFC-0018
```

# 2. Motivation

A host today authenticates against a fixed map baked in at launch (`vara serve
--basic alice:pw`). That cannot grow: no one can create an account, rotate a
password, mint a token for a CI job, or revoke a leaked credential without
restarting the server with new flags. Every user-facing Hub feature — logging
into a dashboard, a CLI that remembers a token, a bot pushing on a schedule —
needs durable accounts and revocable credentials. RFC-0020 is the smallest layer
that turns "a static credential list" into "a managed set of accounts that mint
and revoke their own credentials," while changing nothing about how an identity
is *represented* or *used*.

# 3. Non-Goals

Explicitly **out of scope**, each deferred:

* **Authorization.** RFC-0020 never decides what a subject may do. An account with
  no policy can authenticate and still do nothing (RFC-0018 default-deny). 401 vs
  403 stays exactly as RFC-0017/0018 define it.
* **Organizations, teams, membership, invitations.** An account is a single
  subject. Grouping subjects is a later RFC.
* **Federated / third-party login** — OAuth2, OIDC, SAML, SSH keys, mTLS. RFC-0017
  reserved these `IdentityMethod`s; wiring them to external providers is future
  work. RFC-0020 covers only local accounts.
* **Email verification, password-reset flows, MFA/TOTP, account recovery.** These
  are user-experience features layered on the account primitive; each is its own
  RFC.
* **Session rotation / refresh tokens / sliding expiry.** Reserved (§5.2, §14);
  v1 sessions have fixed bounds and are re-minted by logging in again.
* **Scoped tokens.** A v1 token carries its account's full authority; per-token
  capability subsets are future work.
* **Web UI, REST framing.** The control plane is JSON (RFC-0021/0022 present it).
* **A database.** v1 storage is a server-managed, file-backed store behind an
  interface; a pluggable DB backend is future work.
* **Engine / transport / `Identity` changes.** RFC-0020 adds no field to `pkg/*`,
  no wire message to RFC-0016, and no field to RFC-0017's `Identity`.

# 4. Terminology

* **Account** — a durable subject: a username, a password hash, and a lifecycle
  state. The username **is** the RFC-0018 policy subject key (§5.5). Automation can
  own an account too; "user" and "subject" are the same primitive here.
* **Session** — a bearer credential minted by an interactive login
  (username+password), with a bounded lifetime (idle + absolute expiry). Models
  "logged in from a client."
* **API token** — a bearer credential minted explicitly by an authenticated
  account, named, long-lived until revoked. Models "a key a script carries."
* **Credential secret** — the opaque high-entropy string a session or token is
  presented as (`Authorization: Bearer <secret>`). Shown **once** at creation;
  stored only as a hash; never recoverable (§6.2).
* **Credential store** — the server-managed persistence for accounts, sessions,
  and tokens, owned solely by the identity layer (§9). Lives outside any
  repository, like RFC-0018 policy and RFC-0019 metadata.
* **Identity source** — an RFC-0017 `IdentitySource`. RFC-0020 provides two
  persistent implementations (§7); the interface is unchanged.

# 5. The Account Model

## 5.1 Accounts & account lifecycle

An account is `{ username, password-hash, state, created_at }`. The username is a
stable, unique subject identifier under the same validation family as a repository
name (a portable charset; reserved prefixes still reserved). v1 does not rename
accounts: the username is the subject key and is stable, so RFC-0018 policy keyed
on it stays valid. (An immutable account id under a mutable username — the RFC-0019
ID lesson — is reserved for if/when rename is needed; §14.)

An account has a lifecycle state, mirroring the RFC-0019 discipline:

```
   (nonexistent)
        │ create
        ▼
     Creating ──► (rollback on failure) ──► (nonexistent)
        │  password hashed + record committed
        ▼
      Active ◄──────────► Disabled        (disable / re-enable)
        │  delete
        ▼
     Deleting ──► (nonexistent)           (record + all its credentials removed)
```

Login behavior is defined per state, so there is no ambiguity:

* **Active** — the password is checked (§6.1); success authenticates.
* **Disabled** — authenticates to nothing: every credential check fails closed →
  `401`. Existing sessions/tokens for the account also stop resolving (S6). The
  record is retained (audit, re-enable) rather than deleted.
* **Creating / Deleting** — a tombstone, never a usable account: authenticates to
  nothing, and a **Deleting** account mints no new sessions or tokens. Deleting an
  account cascades to revoking all of its credentials.

## 5.2 Sessions & session lifecycle

A session is minted by presenting a username and password to the login endpoint
(§8). It returns a **session secret** (shown once) and an expiry. A session has two
bounds, both enforced at authentication time (§7):

* an **absolute** lifetime (a session cannot outlive its creation by more than a
  fixed maximum), and
* an **idle** timeout (a session unused for longer than a fixed window expires).

Its lifecycle:

```
   Issued ──► Active ──► Expired          (past an idle or absolute bound)
                 │
                 └─────► Revoked          (logout, or account disabled/deleted)
```

* **Issued** — created at login; the secret is returned exactly once.
* **Active** — within both bounds and not revoked; each successful use refreshes
  the idle marker.
* **Expired** — past a bound; authenticates to nothing (checked at resolution, not
  by a background sweep, so expiry is honored even if a sweep has not run).
* **Revoked** — explicitly invalidated (logout, or the account was disabled or
  deleted); immediately and permanently invalid (§5.4).

Sliding sessions, refresh tokens, and rotation are **reserved** (§14): a v1 session
is re-minted by logging in again, not refreshed.

## 5.3 API tokens & token metadata

An API token is minted by an already-authenticated account (§8), carries a
human-chosen **name**, and is **long-lived until revoked** — the credential
automation carries. It is not tied to a login event and has no idle timeout; it
lives until its owner revokes it or the account is disabled/deleted.

`Identity` stays tiny (RFC-0017), but a **token record** carries metadata, so a
future UI can present and manage tokens without ever touching `Identity`:

```
{ id, name, subject, created_at, last_used_at, expires_at, revoked }
```

`expires_at` is optional (absent = never expires in v1); the **secret** appears in
no token record — only its hash does (§6.2). `last_used_at` is updated on
successful resolution for the owner's visibility, not for authorization.

Sessions and API tokens are both **bearer credentials** on the wire — identical
`Authorization: Bearer <secret>` shape, identical resolution to an `Identity` with
`Method = Bearer`. They differ only in *how they are minted and expire*, not in how
they authenticate. The `Identity` never records which kind it was (S12).

## 5.4 Revocation is immediate

Revocation has **no grace period**. The instant a session is revoked (logout), a
token is revoked, or an account is disabled or deleted:

```
   revoke  ──►  credential is invalid  ──►  every subsequent request using it fails
```

There is no window in which a revoked credential still resolves, and nothing an
authentication caches may outlive the revocation (S6, RFC-0017 C7). Disabling or
deleting an account revokes **all** of its sessions and tokens at once. This is the
property that makes a leaked credential containable: revoke it and it is dead on
the next request.

## 5.5 One namespace: an account is an RFC-0018 subject

An account's username is exactly the subject string RFC-0018 policy is keyed on.
There is no separate "user table" and "subject table": when `alice` logs in, her
resolved `Identity.ID` is `"alice"`, and a policy granting `"alice"` capabilities
applies with no translation. This is the load-bearing integration — it is why
RFC-0020 needs to change nothing in RFC-0018: authentication produces a subject,
and authorization already knows what to do with a subject.

Authentication grants **no** authorization. A freshly created account holds no
capabilities until a policy grants some (RFC-0018 default-deny). "I know who you
are" and "you may do this" stay strictly separate (RFC-0017 C3).

# 6. Secret Handling (Normative Core)

This section is the reason RFC-0020 is security-sensitive; its rules are
normative, not advisory.

## 6.1 Password storage

A password is stored **only** as a salted, memory-hard **KDF** hash — argon2id is
RECOMMENDED, with a per-password random salt and parameters chosen so a single
verification costs tens of milliseconds. A password MUST NOT be stored in
plaintext, MUST NOT be stored under a fast hash (SHA-256/SHA-1/MD5) with or without
salt, and MUST NOT be reversible. Verification is constant-time in the comparison
step, and the KDF's own cost makes a matched-vs-unmatched attempt indistinguishable
by timing (S3). The stored form is self-describing (algorithm + parameters + salt
encoded together) so parameters can be raised over time and old hashes upgraded on
next successful login.

## 6.2 Session & token secret storage

A session/token secret is high-entropy (≥128 bits from a CSPRNG), so it needs no
KDF: it is stored **only** as a fast cryptographic hash (e.g. SHA-256) of the
secret, and looked up by hashing the presented secret and comparing in constant
time. The plaintext secret is returned to the caller **once**, at creation, and is
never stored, never logged, and never recoverable — a lost secret is re-minted, not
retrieved (S4). Because the store holds only hashes, a store compromise leaks no
usable credential.

## 6.3 Password policy (minimal)

The server enforces a **minimal** password policy, and no more:

* a **minimum length** (a configured baseline, e.g. ≥ 8 characters);
* **no maximum below a safe bound** — long passphrases MUST be accepted (support at
  least ~1 KB) so passwords are never silently truncated;
* full **UTF-8** accepted, unnormalized bytes hashed as given;
* **no composition/complexity rules**, no forced rotation, no paste-blocking.

A password failing the minimum is rejected at creation/change with `400`. Length
minimums and long-passphrase support beat character-class rules for real strength.

## 6.4 Logging & audit

Extending RFC-0017's logging rule. Logs MUST NOT contain passwords, session
secrets, token secrets, `Authorization` headers, or KDF/hash outputs. The server
SHOULD emit an **audit** record for each security-relevant event — login success,
login failure, session logout, token created, token revoked, password changed,
account created/disabled/deleted — recording the subject, the method, the token's
opaque id/name where relevant, and the outcome, but **never** the secret material
(S10). Audit records make credential misuse visible without ever storing what an
attacker could replay.

# 7. Identity Sources (Integration with RFC-0017)

RFC-0020 provides two `IdentitySource` implementations. Both satisfy the unchanged
`Authenticate(*Credential) (Identity, error)` interface and return errors only for
*invalid credentials* (never for valid-but-unpermitted, which is RFC-0018's 403).
They compose in the existing `Multi` source exactly as the static sources do.

* **AccountSource** (Basic). Validates a username+password `Credential` against the
  account store: look up the account, reject unless **Active** (§5.1), verify the
  password via §6.1 (constant-time, dummy-hash for absent accounts per §12). On
  success returns `Identity{ID: username, Method: Basic}`.
* **CredentialSource** (Bearer). Validates a bearer `Credential` against the
  session/token store: hash the presented secret (§6.2), find the record, reject if
  absent, revoked, expired, or its account is not Active; on a session, refresh the
  idle marker; on a token, update `last_used_at`. On success returns
  `Identity{ID: account, Method: Bearer}`.

Both MUST NOT read repository state (RFC-0017 C6 preserved): the credential store is
server state, not repository state, so authentication stays deterministic and
repository-agnostic. Resolution is request-scoped (C7); nothing cached outlives a
revocation (S6). The resolved `Identity` carries the subject and the coarse method
only — **not** whether a Bearer was a session or a token (S12), so authorization and
the transport never learn authentication internals.

# 8. Control-Plane API (HTTP Binding)

RFC-0020 extends the RFC-0019 control plane under the reserved `/_vara/` prefix.
Every route runs the standard preamble first (echo headers, version check,
**authenticate**), and every route except login is authenticated.

## 8.1 Sessions

```
POST   /_vara/sessions            {username, password}  → 201 {secret, expires_at}   (login; secret shown once)
DELETE /_vara/sessions/current                          → 204                        (logout: revoke the calling session)
```

Login is the one unauthenticated route (it *is* the authentication). A failed login
is `401` and MUST be rate-limitable and constant-time with respect to account
existence (§12).

## 8.2 API tokens

```
POST   /_vara/tokens              {name}   → 201 {id, name, secret, created_at}       (secret shown once)
GET    /_vara/tokens                       → 200 {tokens:[{id,name,created_at,last_used_at}...]}  (own tokens; NO secrets)
DELETE /_vara/tokens/{id}                  → 204                                       (revoke)
```

All three require an authenticated caller; a caller manages only its **own** tokens.
Listing returns metadata only — never a secret (§6.2).

## 8.3 Accounts (administration)

```
POST   /_vara/accounts            {username, password}   → 201 {username, state, created_at}   (needs manage-accounts)
DELETE /_vara/accounts/{username}                        → 204                                  (needs manage-accounts)
POST   /_vara/accounts/{username}/disable                → 204                                  (needs manage-accounts)
PUT    /_vara/accounts/{username}/password  {password}   → 204                                  (self, or manage-accounts)
```

Account administration is a **server-scoped** authorization (RFC-0018/0019): it
requires the `manage-accounts` capability on the server resource (`_server`), except
that an account MAY change its **own** password when authenticated. This reuses
RFC-0019's server-scope model rather than inventing a new mechanism.

## 8.4 Status codes

| Situation                              | Status | Code                          |
|----------------------------------------|--------|-------------------------------|
| Created (account/token/session)        | 201    | —                             |
| Deleted / disabled / pw changed / logout | 204  | —                             |
| Bad login / bad credential             | 401    | `UNAUTHENTICATED` (RFC-0017)  |
| Authenticated but lacks manage-accounts| 403    | `UNAUTHORIZED` (RFC-0018)     |
| Username already taken                 | 409    | `ACCOUNT_EXISTS`              |
| No such account/token                  | 404    | `NOT_FOUND`                   |
| Malformed body / weak password         | 400    | `MALFORMED_REQUEST`           |

`ACCOUNT_EXISTS` is the one new code; the rest are inherited.

# 9. Storage & Ownership

Accounts, sessions, and tokens live in a **server-managed credential store** outside
any repository (never in `.vara`, never pushable, never reachable by the data plane)
— the same placement discipline as RFC-0018 policy and RFC-0019 metadata (S2).

**Ownership is exclusive (S13).** The credential store is owned by the **identity
layer** alone. Authorization owns the policy store; the repository manager owns the
metadata store; the identity layer owns the credential store. No subsystem reads or
writes another's datastore — each subsystem owns exactly one, and crosses to another
only through that subsystem's API. This keeps the platform's persistence as layered
as its imports.

v1 ships a file-backed store behind a small interface (`AccountStore` /
`CredentialStore`), so a database or KV backend can replace it later without touching
the identity sources or the control plane. The store holds only hashes for secrets
(§6); writes are atomic (temp-file + rename), matching the rest of the codebase.

# 10. Bootstrapping a Host

A fresh host has no accounts. Until one exists, no login can succeed — but
**anonymous remains a valid identity** (RFC-0017), so an anonymous-readable,
policy-gated host still works. An operator bootstraps the first account by either:

* **(a)** a server-scope grant: `_server.json` grants a bootstrap subject
  `manage-accounts`, and that subject creates accounts over the API; or
* **(b)** an on-host admin command (`vara account create` against the credential
  store on the host's own filesystem), which the operator is trusted to run
  directly, bypassing the wire — the same escape hatch RFC-0018 policy and RFC-0019
  repositories use (S8).

# 11. Architectural Constraints (Normative)

* **S1 — RFC-0017 is unchanged.** RFC-0020 adds `IdentitySource` implementations
  only. It changes no field of `Identity`, no `IdentityMethod`, and no step of the
  Authenticate→Authorize→act pipeline. Authentication still terminates above the
  transport.
* **S2 — Credential store is outside repositories.** Accounts, sessions, and tokens
  are server-managed, never inside `.vara`, never pushable, never reachable by the
  data plane (RFC-0018 A3 analogue).
* **S3 — Passwords are KDF-hashed.** Stored only as a salted, memory-hard KDF hash;
  never plaintext, never a fast hash, never reversible; verified in constant time
  (§6.1).
* **S4 — Secrets are hash-stored and shown once.** Session/token secrets are
  high-entropy, stored only as a hash, returned once at creation, never logged,
  never recoverable (§6.2).
* **S5 — Authentication is repository-agnostic.** No identity source reads repository
  state (RFC-0017 C6 preserved); the credential store is server state.
* **S6 — Revocation is immediate.** Deleting/disabling an account, or revoking a
  session or token, makes the next request presenting it fail; no grace period,
  nothing cached outlives a revocation (§5.4, RFC-0017 C7).
* **S7 — Account = subject; authentication ≠ authorization.** An account's username
  is its RFC-0018 subject key; authenticating grants no capability. A new account
  can do nothing until policy grants it (default-deny).
* **S8 — Default-deny bootstrapping.** No accounts ⇒ no logins, but anonymous stays
  valid. Account creation is itself authorized (server-scope `manage-accounts`) or
  an on-host operator action.
* **S9 — Expiry is enforced at authentication.** Sessions honor an absolute and an
  idle bound; an expired credential authenticates to nothing, checked at resolution
  time (§5.2, §7).
* **S10 — Secrets are never logged; events are audited.** No passwords, secrets,
  `Authorization` headers, or hash outputs in logs; security-relevant events are
  audited without secret material (§6.4).
* **S11 — Engine unchanged.** RFC-0020 adds nothing to `pkg/*` and no message to
  RFC-0016.
* **S12 — Identity is mechanism-agnostic.** `Identity` represents the authenticated
  subject only; the authentication mechanism (password vs session vs token) MUST NOT
  be propagated below the identity layer. Authorization and the transport are
  blissfully unaware of *how* a subject authenticated.
* **S13 — One datastore per subsystem.** The credential store is owned solely by the
  identity layer; authz owns policy, the repository manager owns metadata. No
  subsystem touches another's datastore except through its API (§9).

# 12. Security Considerations

* **Login is the attack surface.** It MUST be constant-time with respect to account
  existence (hash a dummy password for an absent or non-Active account so timing does
  not reveal which usernames exist), and rate-limitable per source. A failed login is
  `401` with no hint whether the username or the password was wrong.
* **Password policy** is minimal by design (§6.3): a length floor and long-passphrase
  support, not character-class theater.
* **KDF cost is tunable and self-describing.** Stored hashes carry their parameters so
  cost can be raised over time; a successful login with an under-cost hash SHOULD
  transparently re-hash at the new cost.
* **Token scope.** v1 tokens carry the full authority of their account (no per-token
  capability subset); scoped tokens are future work. A leaked token is contained by
  revocation (§5.4) and by RFC-0018 policy still gating every action.
* **Transport security.** Secrets cross the wire in `Authorization`; a production host
  MUST terminate TLS (RFC-0016 §10). RFC-0020 assumes the transport is confidential and
  does not itself encrypt secrets in flight.
* **Store compromise.** Because only hashes are stored (§6), a stolen store yields no
  usable password (KDF cost) and no usable token (needs the plaintext secret).

# 13. Testing Strategy

* **Round-trips.** create account → login → use session → logout → session rejected;
  create token → use → list (no secret in output) → revoke → rejected.
* **Account states (§5.1).** Active authenticates; Disabled → 401 and its existing
  sessions/tokens stop resolving; delete cascades to revoking all credentials.
* **Session states (§5.2).** Issued secret works; past an idle or absolute bound → 401;
  logout → immediate 401.
* **Password hashing (S3).** A stored hash is not the plaintext and not a bare SHA-256
  of it; verification accepts the right password and rejects the wrong one; two accounts
  with the same password have different stored hashes (per-password salt).
* **Secret handling (S4).** A created secret authenticates; the same secret is never
  returned again by any read; the store on disk contains no plaintext secret.
* **Immediate revocation (S6).** A revoked token/session and a disabled/deleted account
  all fail the *next* request immediately, with no grace window.
* **Password policy (§6.3).** A too-short password is 400; a very long passphrase is
  accepted and not truncated; a UTF-8 password round-trips.
* **Account ≠ authorization (S7).** A brand-new account authenticates (200 on an
  authenticated route) yet is denied every capability-gated action (403) until a policy
  grants one; the account username is the subject key with no translation.
* **Login timing/existence (§12).** A login for an absent username and one for a present
  username with a wrong password take indistinguishable time and both return 401 with
  identical bodies.
* **Mechanism-agnostic identity (S12).** A session-derived and a token-derived request
  for the same account are indistinguishable to authorization and the transport.
* **Audit without secrets (S10).** Login/token creation produce audit records for the
  events but logs contain neither the password nor the secret.
* **Architecture (S1/S11/S13).** The engine diff is empty after implementation; the new
  identity sources import no engine/transport package; the credential store is touched
  only by the identity layer; `tests/architecture` continues to pass.

# 14. Future Work

* **Immutable account id + mutable username** — the RFC-0019 ID lesson applied to
  accounts, if account rename is ever needed (§5.1).
* **Session rotation** — sliding expiry, refresh tokens, rotation on use (§5.2).
* **Scoped / capability-limited tokens** — tokens carrying a subset of their account's
  authority, and per-token expiry.
* **Federated identity** — OAuth2/OIDC/SAML/SSH/mTLS wired to the reserved RFC-0017
  methods.
* **Account-experience features** — email verification, password reset, MFA/TOTP,
  account recovery — each its own RFC.
* **Organizations & teams** — grouping accounts as subjects; membership refining
  ownership (RFC-0019 §14).
* **Pluggable store backends** — a database/KV `CredentialStore` behind the v1 interface
  (§9).
* **`vara account` / `vara login` CLI** — client commands over the control plane, plus
  the on-host bootstrap command (§10b).
