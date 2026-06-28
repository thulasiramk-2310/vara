package object

import (
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/object"
)

func BenchmarkBlobSerialization(b *testing.B) {
	blob := object.NewBlob([]byte("hello world performance test payload"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = blob.Serialize()
	}
}

func BenchmarkStoreWrite(b *testing.B) {
	tempDir := b.TempDir()
	store := object.NewStore(tempDir)
	blob := object.NewBlob([]byte("benchmark object write payload"))
	
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Write(blob)
	}
}
