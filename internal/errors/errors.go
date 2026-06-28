// Package errors implements shared error types for the VARA repository.
//
// Responsibilities:
// - Define typed errors used across all packages.
//
// This package MUST NOT:
// - Import any other VARA packages.
package errors

import "errors"

var (
	ErrRepositoryNotFound = errors.New("vara: repository not found")
	ErrInvalidObject      = errors.New("vara: invalid object format")
	ErrCorruptIndex       = errors.New("vara: corrupt index file")
	ErrLockTimeout        = errors.New("vara: lock acquisition timeout")
	ErrMergeConflict      = errors.New("vara: merge conflict")
	ErrRefNotFound        = errors.New("vara: reference not found")
	ErrObjectNotFound     = errors.New("vara: object not found")
	ErrRefValidation      = errors.New("vara: invalid reference name")
)
