package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/thulasiramk-2310/vara/pkg/compression"
	"github.com/thulasiramk-2310/vara/pkg/object"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// VPCK stream layout (RFC-0014 §8):
//
//	"VPCK"        magic         (4 bytes)
//	version       uint8 = 1     (1 byte)
//	objectCount   uint32 BE     (4 bytes)
//	records       count × [uvarint len][len bytes: zstd(serialized object)]
//	trailer       sha256 of all preceding bytes (32 bytes)
var packMagic = [4]byte{'V', 'P', 'C', 'K'}

const packVersion = 1

// WritePack encodes the given object IDs as a VPCK stream to w. Objects are
// read from store, re-serialized, and zstd-compressed per record.
func WritePack(store *object.Store, ids []types.ObjectID, w io.Writer) error {
	h := sha256.New()
	mw := io.MultiWriter(w, h) // everything except the trailer is hashed

	header := make([]byte, 0, 9)
	header = append(header, packMagic[:]...)
	header = append(header, packVersion)
	header = binary.BigEndian.AppendUint32(header, uint32(len(ids)))
	if _, err := mw.Write(header); err != nil {
		return err
	}

	var lenBuf [binary.MaxVarintLen64]byte
	for _, id := range ids {
		obj, err := store.Read(id)
		if err != nil {
			return fmt.Errorf("pack: read %s: %w", id.String()[:7], err)
		}
		compressed := compression.Compress(obj.Serialize())
		n := binary.PutUvarint(lenBuf[:], uint64(len(compressed)))
		if _, err := mw.Write(lenBuf[:n]); err != nil {
			return err
		}
		if _, err := mw.Write(compressed); err != nil {
			return err
		}
	}

	// Trailer is written to w only (not part of the hashed content).
	if _, err := w.Write(h.Sum(nil)); err != nil {
		return err
	}
	return nil
}

// PackResult reports the outcome of ingesting a pack.
type PackResult struct {
	ObjectsRead    int
	ObjectsWritten int // excludes objects already present (idempotent writes)
}

// ReadPack decodes a VPCK stream from r, verifies its trailer and every
// object's identity, and writes each object into store. Writing an object that
// already exists is a safe no-op (objects are immutable).
func ReadPack(store *object.Store, r io.Reader) (PackResult, error) {
	var res PackResult

	all, err := io.ReadAll(r)
	if err != nil {
		return res, err
	}
	if len(all) < 9+sha256.Size {
		return res, fmt.Errorf("pack: stream too short (%d bytes)", len(all))
	}

	body := all[:len(all)-sha256.Size]
	trailer := all[len(all)-sha256.Size:]
	sum := sha256.Sum256(body)
	if !bytes.Equal(sum[:], trailer) {
		return res, fmt.Errorf("pack: checksum mismatch (corrupt or truncated stream)")
	}

	if !bytes.Equal(body[:4], packMagic[:]) {
		return res, fmt.Errorf("pack: bad magic %q", body[:4])
	}
	if body[4] != packVersion {
		return res, fmt.Errorf("pack: unsupported version %d", body[4])
	}
	count := binary.BigEndian.Uint32(body[5:9])

	buf := bytes.NewReader(body[9:])
	for i := range count {
		clen, err := binary.ReadUvarint(buf)
		if err != nil {
			return res, fmt.Errorf("pack: record %d length: %w", i, err)
		}
		compressed := make([]byte, clen)
		if _, err := io.ReadFull(buf, compressed); err != nil {
			return res, fmt.Errorf("pack: record %d body: %w", i, err)
		}
		raw, err := compression.Decompress(compressed)
		if err != nil {
			return res, fmt.Errorf("pack: record %d decompress: %w", i, err)
		}
		obj, err := object.Deserialize(raw)
		if err != nil {
			return res, fmt.Errorf("pack: record %d deserialize: %w", i, err)
		}
		res.ObjectsRead++
		// Store.Write recomputes the hash from the serialized form and is a
		// no-op if the object already exists, giving us identity verification
		// and idempotency for free.
		if _, err := store.Write(obj); err != nil {
			return res, fmt.Errorf("pack: record %d write: %w", i, err)
		}
		res.ObjectsWritten++
	}
	if buf.Len() != 0 {
		return res, fmt.Errorf("pack: %d trailing bytes after %d records", buf.Len(), count)
	}
	return res, nil
}
