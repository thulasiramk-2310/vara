# VARA RFC Lifecycle

This document defines the lifecycle process that every VARA RFC (Request for Comments) must follow. VARA treats its documentation as a rigorous protocol specification; code is merely an implementation of an Accepted RFC.

## RFC States

1. **Draft**
   - The RFC is actively being written, debated, and revised.
   - It is not final. Implementations must not rely on it.

2. **Review**
   - The draft is complete and awaiting formal approval from project architects.

3. **Accepted**
   - The RFC is finalized and frozen. It acts as the immutable design contract for the feature or protocol.
   - Changes to an Accepted RFC should be extremely rare and deliberate.

4. **Implemented**
   - The specifications within the RFC have been fully implemented in the VARA codebase.

5. **Deprecated**
   - The feature or protocol is still supported for backward compatibility but is slated for removal.

6. **Superseded**
   - The RFC has been entirely replaced by a newer RFC. The header must specify `Superseded By: RFC-XXXX`.
