#!/usr/bin/env bash
# Builds bin/golangci-lint-custom, golangci-lint with the module plugins in .custom-gcl.yml, and
# prints its path. It rebuilds only when .custom-gcl.yml, a plugin source file, or the Go version
# changed since the last build. Requires golangci-lint (just install-tools).

set -euo pipefail
# An exported CDPATH makes `cd` print the directory, corrupting resolved paths.
unset CDPATH

cd "$(dirname "${BASH_SOURCE[0]}")/.."

binary="bin/golangci-lint-custom"
stamp="bin/.golangci-lint-custom.sha256"

if ! command -v golangci-lint > /dev/null; then
    echo "golangci-lint is not installed; run 'just install-tools'." >&2
    exit 1
fi

fingerprint="$(
    {
        go env GOVERSION
        sha256sum .custom-gcl.yml
        git ls-files --cached --others --exclude-standard -- tools/telemetrylint \
            | grep -E '(\.go|/go\.(mod|sum))$' \
            | grep -v '/testdata/' \
            | LC_ALL=C sort \
            | xargs sha256sum
    } | sha256sum | cut -d' ' -f1
)"

if [[ ! -x "$binary" || ! -f "$stamp" || "$(cat "$stamp")" != "$fingerprint" ]]; then
    echo "Building $binary from .custom-gcl.yml..." >&2
    rm -f "$stamp"
    golangci-lint custom >&2
    printf '%s\n' "$fingerprint" > "$stamp"
fi

echo "$binary"
