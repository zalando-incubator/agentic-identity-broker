#!/usr/bin/env bash
set -euo pipefail

binary=$1
mode=$2
source_root=$3

metadata=$(go version -m "$binary")
if [[ $metadata != *$'\tbuild\t-trimpath=true'* ]]; then
    echo "Missing -trimpath in $binary" >&2
    exit 1
fi

sections=$(readelf -SW "$binary")
case $mode in
    stripped)
        if [[ $sections == *'.debug_'* ]]; then
            echo "Unexpected DWARF in $binary" >&2
            exit 1
        fi
        ;;
    debug)
        for section in .debug_info .debug_line; do
            if [[ $sections != *"$section"* ]]; then
                echo "Missing $section in $binary" >&2
                exit 1
            fi
        done
        sources=$(go tool objdump -s '^main\.main$' "$binary")
        if [[ $sources != *'.go:'* || $sources == *"$source_root/"* ]]; then
            echo "Missing trimmed source paths in $binary (source root: $source_root)" >&2
            exit 1
        fi
        ;;
    *)
        echo "Expected stripped or debug mode, got: $mode" >&2
        exit 1
        ;;
esac
printf 'Verified %s: %s, trimmed source paths\n' "$binary" "$mode"
