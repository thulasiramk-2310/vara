package object

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/thulasiramk-2310/vara/internal/errors"
	"github.com/thulasiramk-2310/vara/pkg/compression"
	"github.com/thulasiramk-2310/vara/pkg/hash"
	"github.com/thulasiramk-2310/vara/pkg/types"
)

// Store provides read/write access to the `.vara/objects` directory.
type Store struct {
	ObjectsDir string
}

// NewStore initializes an ObjectStore.
func NewStore(objectsDir string) *Store {
	return &Store{ObjectsDir: objectsDir}
}

// Write serializes, compresses, and writes an object to the store.
// It returns the generic ObjectID.
func (s *Store) Write(obj Object) (types.ObjectID, error) {
	raw := obj.Serialize()
	h := hash.Compute(raw)
	id := types.ObjectID(h)
	
	// In VARA, the hash is based on the uncompressed data.
	hexHash := id.String()
	dir := filepath.Join(s.ObjectsDir, hexHash[:2])
	path := filepath.Join(dir, hexHash[2:])
	
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return id, err
	}
	
	// If object already exists, do not overwrite (objects are immutable)
	if _, err := os.Stat(path); err == nil {
		return id, nil
	}
	
	compressed := compression.Compress(raw)
	
	// Write to temp file then atomic rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, compressed, 0444); err != nil {
		return id, err
	}
	
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return id, err
	}
	
	return id, nil
}

// Read fetches and deserializes an object by its generic ObjectID.
func (s *Store) Read(id types.ObjectID) (Object, error) {
	hexHash := id.String()
	path := filepath.Join(s.ObjectsDir, hexHash[:2], hexHash[2:])
	
	compressed, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.ErrObjectNotFound
		}
		return nil, err
	}
	
	raw, err := compression.Decompress(compressed)
	if err != nil {
		return nil, fmt.Errorf("%w: compression error", errors.ErrInvalidObject)
	}
	
	// Validate hash integrity
	computed := hash.Compute(raw)
	if computed != types.BaseID(id) {
		return nil, fmt.Errorf("%w: hash mismatch", errors.ErrInvalidObject)
	}
	
	return Deserialize(raw)
}
