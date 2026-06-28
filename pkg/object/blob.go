package object

import (
	"bytes"

	"github.com/thulasiramk-2310/vara/pkg/hash"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Blob represents a raw file content object.
type Blob struct {
	Data []byte
}

// BlobHash returns the object ID that VARA would assign to a blob with the given
// content. This is the single authoritative definition; any code computing a blob
// ID from raw bytes MUST call this function instead of hashing content directly.
func BlobHash(content []byte) types.BlobID {
	b := NewBlob(content)
	h := hash.Compute(b.Serialize())
	return types.BlobID(h)
}

// NewBlob creates a new Blob.
func NewBlob(data []byte) *Blob {
	return &Blob{Data: data}
}

// Type returns TypeBlob.
func (b *Blob) Type() ObjectType {
	return TypeBlob
}

// Serialize converts the blob to its raw on-disk byte format.
func (b *Blob) Serialize() []byte {
	var buf bytes.Buffer
	WriteHeader(&buf, TypeBlob)
	buf.Write(b.Data)
	return buf.Bytes()
}

// DeserializeBlob reads a blob from the reader (assuming header is already read).
func DeserializeBlob(r *bytes.Reader) (*Blob, error) {
	var buf bytes.Buffer
	buf.ReadFrom(r) // Read the rest of the file
	return &Blob{Data: buf.Bytes()}, nil
}
