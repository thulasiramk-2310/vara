VARA RFC: 0005
Title: Index Format
Status: Accepted
Version: 1.1.0
Authors: Thulasiram K
Created: 2026-06-28
Last Updated: 2026-06-28
Depends On: RFC-0000, RFC-0002, RFC-0003, RFC-0006
Supersedes: None
Superseded By: None

# 1. Vision & Purpose
This document defines the binary layout of the VARA Index (`.vara/index`). The index is NOT just a "staging area"—it is the **authoritative repository cache** that bridges the physical filesystem and the object store. 

By caching structural and metadata information, the index acts as the primary engine powering `status`, `add`, `switch`, `merge`, and `commit`. Everything should read the index first whenever possible.

# 2. Binary Format Architecture
The index is a monolithic binary file built for speed, resilience, and extensibility.

## 2.1. Binary Header
The file begins with a strict 16-byte header ensuring instant validation.
```
Magic (4 bytes): "VARA"
Version (4 bytes): uint32 (currently 1)
Entry Count (4 bytes): uint32
Global Flags (4 bytes): uint32 (Reserved for future repository-wide states)
```

## 2.2. Entry Layout
Unlike Git, VARA deliberately sheds decades of POSIX legacy (e.g., UID, GID, device numbers) that serve only backward compatibility rather than correctness.

Entries are sorted strictly lexicographically by path.
Each entry has a fixed structure followed by a variable-length path:
```
CTime (8 bytes): Creation time (seconds since epoch)
MTime (8 bytes): Last modified time (seconds since epoch)
FileSize (8 bytes): File size in bytes (uint64 to support >4GB files)
Mode (4 bytes): File permissions (e.g., 100644, 120000)
Hash (32 bytes): The raw SHA-256 hash of the blob
Fingerprint (8 bytes): A lightweight hash (e.g., xxHash) of the first 64 bytes of the file.
State (1 byte): The Entry State (see Section 3).
PathLength (2 bytes): Length of the filename.
Path (variable): UTF-8 path string.
Padding (variable): Null bytes `\0` padding to align the next entry to an 8-byte boundary.
```

## 2.3. Extensions
To support future features without breaking older clients or changing the format version, an extension area is reserved after the entries.
```
Signature (4 bytes): e.g., "TREE" (cached trees), "FSMS" (filesystem monitor)
Length (4 bytes): Size of the extension payload
Data (variable): The extension payload
```

## 2.4. Trailer (Checksum)
The final 32 bytes of the file contain a SHA-256 checksum over the entire preceding contents (Header, Entries, Extensions). Any partial write or corruption is immediately rejected before parsing.

# 3. Explicit Entry States
Instead of re-deriving status mathematically every time, the index explicitly tracks the state machine of each entry.
The 1-byte `State` field maps to:
* `0x00`: Clean
* `0x01`: Modified
* `0x02`: Staged
* `0x03`: Deleted
* `0x04`: Renamed
* `0x05`: Conflict
* `0x06`: Ignored
* `0x07`: Intent-To-Add

# 4. Racy VARA & The Fingerprint Mitigation
**The "Racy VARA" Problem:** If a file is modified within the same second it is staged, its `MTime` will match the index's `MTime`, but its content has changed.

**Mitigation:** 
VARA goes a step further than relying solely on timestamps. If an entry's `MTime` and `FileSize` match, VARA checks the `Fingerprint` (a fast hash of the first 64 bytes). This catches the vast majority of instant in-place edits with minimal I/O overhead. If the fingerprint mismatches, a full SHA-256 re-hash is forced.

# 5. Index Updates and Repository Transactions
Updates to the index must be atomic and participate in **Repository Transactions** (see RFC-0006).
During a command like `vara commit`:
1. Acquire `VARA_LOCK`.
2. Write objects to store.
3. Write the new index to `.vara/index.lock`.
4. Update references.
5. Rename `.vara/index.lock` over `.vara/index`.
6. Release `VARA_LOCK`.
