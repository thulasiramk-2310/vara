// Package compression implements VARA-RFC-0002.
//
// RFC:
// VARA-RFC-0002 Object Format
//
// Responsibilities:
// - Zstandard compression and decompression wrappers.
//
// This package MUST NOT:
// - Access disk directly or import higher level VARA packages.
package compression

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

var (
	encoder *zstd.Encoder
	decoder *zstd.Decoder
)

func init() {
	var err error
	// Use default compression level for now
	encoder, err = zstd.NewWriter(nil)
	if err != nil {
		panic(err) // Should never fail with nil writer
	}
	decoder, err = zstd.NewReader(nil)
	if err != nil {
		panic(err)
	}
}

// Compress returns the zstd compressed data.
func Compress(data []byte) []byte {
	return encoder.EncodeAll(data, make([]byte, 0, len(data)))
}

// Decompress returns the uncompressed data.
func Decompress(data []byte) ([]byte, error) {
	return decoder.DecodeAll(data, nil)
}

// NewReader wraps an io.Reader with a zstd decompressor.
func NewReader(r io.Reader) (*zstd.Decoder, error) {
	return zstd.NewReader(r)
}

// NewWriter wraps an io.Writer with a zstd compressor.
func NewWriter(w io.Writer) (*zstd.Encoder, error) {
	return zstd.NewWriter(w)
}
