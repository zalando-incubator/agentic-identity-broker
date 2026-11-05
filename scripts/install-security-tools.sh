#!/usr/bin/env bash

# Builds the pinned security scanners into a directory, or with --verify checks that a
# directory already holds exactly those builds. security-scan.sh uses the same pins.

set -euo pipefail
# An exported CDPATH makes `cd` print the directory, corrupting resolved paths.
unset CDPATH

tools=(
    "github.com/securego/gosec/v2/cmd/gosec@v2.30.0"
    "golang.org/x/vuln/cmd/govulncheck@v1.9.0"
    "github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.7.0"
)

usage() {
    echo "usage: $0 [--verify] <tools-dir>" >&2
    exit 2
}

verify=false
if [[ "${1:-}" == "--verify" ]]; then
    verify=true
    shift
fi
(( $# == 1 )) || usage
tools_dir="$1"

verify_tool() {
    local package="$1" version="$2" binary="$3" build_info
    if [[ ! -x "$binary" ]]; then
        echo "missing security scanner: $binary" >&2
        return 1
    fi
    if ! build_info="$(GOTOOLCHAIN=local go version -m "$binary")"; then
        echo "cannot read build information: $binary" >&2
        return 1
    fi
    if ! grep -qxF "$(printf '\tpath\t%s' "$package")" <<<"$build_info"; then
        echo "security scanner is not built from $package: $binary" >&2
        return 1
    fi
    if ! grep -qE "^"$'\t'"mod"$'\t'"[^"$'\t'"]+"$'\t'"${version//./\\.}("$'\t'"|$)" <<<"$build_info"; then
        echo "security scanner is not version $version: $binary" >&2
        return 1
    fi
}

if ! $verify; then
    mkdir -p "$tools_dir"
fi
if [[ ! -d "$tools_dir" ]]; then
    echo "security scanner directory does not exist: $tools_dir" >&2
    exit 1
fi
tools_dir="$(cd "$tools_dir" && pwd -P)"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
cd "$script_dir/.."

for tool in "${tools[@]}"; do
    package="${tool%@*}"
    version="${tool##*@}"
    if ! $verify; then
        GOTOOLCHAIN=local GOBIN="$tools_dir" go install "$tool"
    fi
    verify_tool "$package" "$version" "$tools_dir/${package##*/}"
done
