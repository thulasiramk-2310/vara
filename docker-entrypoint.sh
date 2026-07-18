#!/bin/sh
# Ensure the VARA Hub data directories exist before serving, so a fresh empty
# volume works on first run. The Hub's data lives entirely under $VARA_DATA.
set -eu

DATA="${VARA_DATA:-/data}"
mkdir -p "$DATA/repos" "$DATA/policy" "$DATA/meta" "$DATA/accounts"

exec vara "$@"
