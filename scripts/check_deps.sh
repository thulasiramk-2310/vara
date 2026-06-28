#!/usr/bin/env bash
set -e

echo "Checking acyclic dependency rules..."

# Ensure internal/object does not exist, everything is in pkg/object
if [ -d "internal/object" ]; then
    echo "ERROR: internal/object should not exist, use pkg/object instead."
    exit 1
fi

# Ensure lower layers don't import higher layers
if go list -f '{{.Imports}}' ./pkg/object | grep -q "github.com/thulasiramk-2310/vara/cmd"; then
  echo "ERROR: pkg/object imports cmd layer!"
  exit 1
fi

if go list -f '{{.Imports}}' ./pkg/object | grep -q "github.com/thulasiramk-2310/vara/internal/repository"; then
  echo "ERROR: pkg/object imports repository layer!"
  exit 1
fi

echo "Architecture rules verified. Acyclic structure intact."
