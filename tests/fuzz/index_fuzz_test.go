package fuzz

import (
	"testing"

	"github.com/thulasiramk-2310/vara/pkg/index"
)

// FuzzIndexParser sends random byte streams into the Index parser
func FuzzIndexParser(f *testing.F) {
	// Add some valid looking seeds
	f.Add([]byte("VARA\x00\x00\x00\x01\x00\x00\x00\x00"))
	f.Add([]byte("VARA\x00\x00\x00\x02\x00\x00\x00\x00"))
	f.Add([]byte("INVALID\x00\x00"))

	f.Fuzz(func(t *testing.T, data []byte) {
		index.Deserialize(data) // Ignore error, just ensure no panic/deadlock
	})
}
