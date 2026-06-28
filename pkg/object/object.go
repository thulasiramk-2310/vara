// Package object implements VARA-RFC-0002.
//
// RFC:
// VARA-RFC-0002 Object Format
//
// Responsibilities:
// - Blob serialization
// - Tree serialization
// - Commit serialization
// - Object storage reading and writing
//
// This package MUST NOT:
// - Access refs
// - Access index
// - Perform repository discovery
package object

import (
	"bytes"
	"fmt"
	"io"

	"github.com/thulasiramk-2310/vara/internal/errors"
)

// ObjectType represents the type of a VARA object.
type ObjectType string

const (
	TypeBlob   ObjectType = "vara-blob:v1"
	TypeTree   ObjectType = "vara-tree:v1"
	TypeCommit ObjectType = "vara-commit:v1"
)

// Object is the interface that all VARA objects must implement.
type Object interface {
	Type() ObjectType
	Serialize() []byte
}

// WriteHeader writes the object header (Type + \0) to the buffer.
func WriteHeader(buf *bytes.Buffer, typ ObjectType) {
	buf.WriteString(string(typ))
	buf.WriteByte(0)
}

// ReadHeader reads and validates the object header.
func ReadHeader(r *bytes.Reader) (ObjectType, error) {
	typBytes, err := readUntilNul(r)
	if err != nil {
		if err == io.EOF {
			return "", errors.ErrInvalidObject
		}
		return "", err
	}
	typ := ObjectType(typBytes)
	switch typ {
	case TypeBlob, TypeTree, TypeCommit:
		return typ, nil
	default:
		return "", fmt.Errorf("%w: unknown type %q", errors.ErrInvalidObject, typ)
	}
}

func readUntilNul(r *bytes.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == 0 {
			break
		}
		buf.WriteByte(b)
	}
	return buf.Bytes(), nil
}
