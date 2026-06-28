package repository

import (
	"fmt"
	"os"
	"path/filepath"
)

// Init creates a new VARA repository at the specified path.
func Init(path string) (*Repository, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	vd := filepath.Join(absPath, VaraDir)

	// Create core directories
	dirs := []string{
		vd,
		filepath.Join(vd, "objects"),
		filepath.Join(vd, "refs", "heads"),
		filepath.Join(vd, "refs", "tags"),
		filepath.Join(vd, "logs", "refs", "heads"),
		filepath.Join(vd, "locks"),
		filepath.Join(vd, "journal"),
		filepath.Join(vd, "snapshots"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", d, err)
		}
	}

	// Create VERSION file
	versionPath := filepath.Join(vd, VersionFile)
	if err := os.WriteFile(versionPath, []byte("1\n"), 0644); err != nil {
		return nil, err
	}

	// Create HEAD file
	headPath := filepath.Join(vd, HeadFile)
	if err := os.WriteFile(headPath, []byte("ref: refs/heads/main\n"), 0644); err != nil {
		return nil, err
	}

	return &Repository{
		RootDir: absPath,
		VaraDir: vd,
	}, nil
}
