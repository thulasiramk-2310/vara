// Package repository implements VARA-RFC-0003.
//
// RFC:
// VARA-RFC-0003 Repository Layout
//
// Responsibilities:
// - Repository directory structure creation (`vara init`).
// - Finding the `.vara` directory from the current working path.
// - Reading/writing core layout files (VERSION).
//
// This package MUST NOT:
// - Parse index or objects (handled by pkg/index and pkg/object).
package repository

import (
	"os"
	"path/filepath"

	"github.com/thulasiramk-2310/vara/internal/errors"
)

const (
	VaraDir     = ".vara"
	VersionFile = "VERSION"
	HeadFile    = "HEAD"
)

// Repository represents a local VARA repository instance.
type Repository struct {
	RootDir string
	VaraDir string
}

// Discover recursively searches upward for a .vara directory.
func Discover(startPath string) (*Repository, error) {
	current, err := filepath.Abs(startPath)
	if err != nil {
		return nil, err
	}

	for {
		vd := filepath.Join(current, VaraDir)
		if stat, err := os.Stat(vd); err == nil && stat.IsDir() {
			return &Repository{
				RootDir: current,
				VaraDir: vd,
			}, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	return nil, errors.ErrRepositoryNotFound
}
