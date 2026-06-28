VARA RFC: 0002
Title: Object Format
Status: Accepted
Version: 1.0.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0001
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines the byte-level representation of objects within the VARA version control ecosystem. It establishes the standard for hashing, canonical serialization, object layout, and compression.

# 2. Hashing Scheme
VARA uses **SHA-256** as its hashing algorithm. To prevent type confusion attacks, the object type, format version, and metadata are explicitly included in the hash preimage.

# 3. General Object Layout & Compression
All objects are compressed using **zstd** (Zstandard) before being stored on disk. 

## Object Integrity Header
Every object contains structured metadata before the payload. This header ensures that commands like `vara verify` can quickly validate an object's integrity.

```
<magic>:<version>\0<type>\0<compression>\0<size>\0<content>
```
* `<magic>`: `VARA`
* `<version>`: Object format version (e.g., `1`)
* `<type>`: `blob`, `tree`, or `commit`
* `<compression>`: `zstd`
* `<size>`: ASCII decimal representation of the uncompressed content's byte size.
* `\0`: The null byte serves as the strict field delimiter.

The uncompressed byte sequence (including headers and delimiters) acts as the exact preimage for the SHA-256 hash calculation.

# 4. Object Types and Binary Formats

## 4.1. Blob Object
A blob stores the raw contents of a file.

**Format:**
```
VARA:1\0blob\0zstd\0<size>\0<content>
```

## 4.2. Tree Object
A tree represents a directory structure.

**Format:**
```
VARA:1\0tree\0zstd\0<size>\0<entries>
```

**Tree Entry Format:**
```
<mode> <filename>\0<sha256-hash>
```
* `<mode>`: 6-digit ASCII octal representation of file permissions.
* `<filename>`: The name of the file or directory in valid UTF-8.
* `<sha256-hash>`: The 32-byte **raw binary** representation of the object's SHA-256 hash.

## 4.3. Commit Object
A commit represents a snapshot of the repository at a fixed point in time.

**Format:**
```
VARA:1\0commit\0zstd\0<size>\0<content>
```

**Commit Content Layout:**
```
tree <sha256-hex>
parent <sha256-hex>
author <Name> <<email>> <timestamp> <timezone-offset>
committer <Name> <<email>> <timestamp> <timezone-offset>

<message>
```

# 5. Canonical Ordering and Serialization Rules
Objects MUST be serialized using strict canonical rules:

1. **Fixed Field Order:** Commit headers must strictly follow: `tree`, `parent` (chronological, then lexicographical tie-break), `author`, `committer`.
2. **UTF-8 Only:** All text fields MUST be valid UTF-8. Null bytes (`\0`) are strictly forbidden.
3. **Line Endings:** Commit metadata and messages MUST use LF (`\n`) for line endings. No CRLF (`\r\n`).
4. **Lexicographic Sorting:** Tree entries MUST be sorted lexicographically by their `<filename>` bytes. Directories implicitly append a trailing slash `/` for sorting purposes.
5. **Stable Timestamps:** Timestamps must be Unix epoch seconds (e.g., `1719532800`).
6. **Stable Timezones:** Timezone offsets must be `+HHMM` or `-HHMM`.
