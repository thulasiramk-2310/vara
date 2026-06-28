ADR-0007: Object Identity Includes the Serialized Header
Status: Accepted
Date: 2026-06-28
Deciders: Thulasiram K

# Context

VARA content-addresses objects using SHA-256. A question arises about WHAT is
hashed: just the raw payload, or the payload plus a type header?

VARA uses:
```
object_id = SHA-256(type_string + '\x00' + payload)
```

where `type_string` is one of `"vara-blob:v1"`, `"vara-tree:v1"`, or
`"vara-commit:v1"` (the full versioned type string defined in `pkg/object`).

The bug that prompted this ADR had two phases:

**Phase 1** (fixed in the scanner rewrite): the scanner's `hashFile` originally
computed `SHA-256(content)` — the raw file bytes — while the object store
recorded `SHA-256("vara-blob:v1\x00" + content)`. These produced different IDs,
so the scanner could never confirm a file matched its stored blob.

**Phase 2** (fixed post-alpha): the phase-1 fix introduced `"blob\x00"` as the
prefix, which is shorter than the real header `"vara-blob:v1\x00"`. Although
phase 2 prevented false-negatives in the flaky merge test (because fingerprint
mismatch caught the changes before reaching the hash comparison), it caused
false-positives in `vara status`: tracked, unchanged files always reported as
Modified because the hash comparison could never succeed.

The final fix: `pkg/object.BlobHash(content)` calls `NewBlob(content).Serialize()`
and hashes the result — guaranteeing that every call site uses exactly the same
pre-image as `Store.Write`.

# Decision

**Object identity MUST always include the type prefix.** The canonical form is:

```
SHA-256("<type>\x00<serialized_content>")
```

This is enforced at the serialization boundary: every `Object.Serialize()` method
prepends the header before returning bytes, and the `Store.Write` method computes
the ID from the result of `Serialize()`. Any component that needs to verify
whether a file matches a stored blob must replicate this exact computation.

The invariant: **every component that identifies an object by content must use
the same identity function.**

# Why Include the Header?

1. **Type safety across the same content**: A file `README` and a tree that
   happens to serialize to the same bytes would collide under raw-content
   addressing. The type prefix prevents this — `blob/<content>` and
   `tree/<content>` produce different IDs even for identical bytes.

2. **Protocol correctness**: When verifying an object retrieved from the store,
   the reader can recompute the expected ID from the stored bytes (including the
   header it receives) and check against the filename. Without the header,
   different object types could masquerade as each other.

3. **Alignment with Git**: Git uses `"<type> <size>\x00<content>"` for the same
   reasons. VARA simplifies by omitting the size (the object store knows its own
   file sizes) but retains the type discriminant.

# Consequences

**Good:**
- Object IDs are type-safe: blobs, trees, and commits occupy separate ID spaces.
- Object verification (`vara verify`) can check type consistency: reading an
  object whose computed ID matches the filename but whose header says `commit`
  when a `blob` was expected is a detectable corruption.
- The invariant is simple to state and test: serialize → hash → that is the ID.
  Any tool computing an object ID must call `Serialize()` before hashing.

**Bad:**
- Any component that needs to check whether a working-directory file matches a
  stored blob must NOT hash raw file bytes. It must call `object.BlobHash(content)`
  (or equivalent for other types). This is non-obvious and led to the two-phase
  scanner bug described in Context.
- External tools that want to compute VARA object IDs must know the header
  format. This is documented in RFC-0002 but easy to miss.

# Enforcement

The invariant is enforced at two boundaries:

1. **Write path**: `Store.Write(obj)` calls `obj.Serialize()` (which includes
   the header) before computing the hash. No caller can inject a raw hash.

2. **Read path**: `Store.Read(id)` decompresses and calls `object.Deserialize()`,
   which validates the header. A file whose content doesn't start with a known
   type string is rejected as corrupt.

Any new component that computes object IDs must use `object.BlobHash(data)` (or
the equivalent for other types) as the single authoritative function. Never hash
raw bytes; never hardcode the header string. The object package owns the header.
