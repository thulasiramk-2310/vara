#!/bin/sh
# Build the Hub web interface (the React app in frontend/) into a folder that
# `vara serve --hub <folder>` can serve.
#
#   scripts/build-hub-ui.sh [OUT_DIR]        default OUT_DIR: ./hub-ui
#
# Needs Node.js 20 or newer. The build is reproducible from package-lock.json
# (npm ci), and nothing is fetched at run time: the built UI loads only its own files.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
out=${1:-$root/hub-ui}
# A relative folder is relative to where the command was run, not to frontend/ (the script
# moves there below), so the release workflow's `build-hub-ui.sh hub-ui` lands in ./hub-ui.
case "$out" in
    /*) ;;
    *) out=$(pwd)/$out ;;
esac

if ! command -v npm >/dev/null 2>&1; then
    echo "build-hub-ui: npm is required (Node.js 20 or newer)" >&2
    exit 1
fi

cd "$root/frontend"
npm ci --no-audit --no-fund
npm run build

if [ ! -f dist/index.html ]; then
    echo "build-hub-ui: the build did not produce dist/index.html" >&2
    exit 1
fi

rm -rf "$out"
mkdir -p "$out"
cp -R dist/. "$out/"
echo "Hub UI built in $out"
echo "Serve it with:  vara serve --hub $out ..."
