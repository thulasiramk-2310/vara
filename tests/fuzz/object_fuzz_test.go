package fuzz

import (
	"bytes"
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/object"
)

// FuzzObjectParser sends random byte streams into the object deserializers
// to ensure they never panic, deadlock, or leak memory.
func FuzzObjectParser(f *testing.F) {
	f.Add([]byte("vara-blob:v1\000hello"))
	f.Add([]byte("vara-blob:v1\000"))
	f.Add([]byte("vara-tree:v1\000"))
	f.Add([]byte("vara-commit:v1\000"))
	f.Add([]byte("invalid\000hello"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bytes.NewReader(data)
		typ, err := object.ReadHeader(r)
		if err == nil {
			switch typ {
			case object.TypeBlob:
				object.DeserializeBlob(r)
			case object.TypeTree:
				object.DeserializeTree(r)
			case object.TypeCommit:
				object.DeserializeCommit(r)
			}
		}
	})
}
