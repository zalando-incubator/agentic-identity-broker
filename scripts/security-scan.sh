#!/usr/bin/env bash

set -euo pipefail
# An exported CDPATH makes `cd` print the directory, corrupting resolved paths.
unset CDPATH

if (( $# > 2 )); then
    echo "usage: $0 [scan-root] [repository|broker]" >&2
    exit 2
fi

scan_root_input="${1:-.}"
scan_scope="${2:-repository}"
case "$scan_scope" in
    repository|broker) ;;
    *) echo "scan scope must be repository or broker" >&2; exit 2 ;;
esac
if [[ ! -d "$scan_root_input" ]]; then
    echo "scan root must be an existing directory: $scan_root_input" >&2
    exit 2
fi
scan_root="$(cd "$scan_root_input" && pwd -P)"

# SECURITY_SCAN_TOOLS_DIR names scanners built before an environment that cannot
# compile them, such as the agent sandbox; they must match the pinned builds.
prebuilt_tools=""
if [[ -n "${SECURITY_SCAN_TOOLS_DIR:-}" ]]; then
    if [[ ! -d "$SECURITY_SCAN_TOOLS_DIR" ]]; then
        echo "SECURITY_SCAN_TOOLS_DIR must be an existing directory: $SECURITY_SCAN_TOOLS_DIR" >&2
        exit 2
    fi
    prebuilt_tools="$(cd "$SECURITY_SCAN_TOOLS_DIR" && pwd -P)"
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
trusted_root="$(cd "$script_dir/.." && pwd -P)"
scratch_dir="$(mktemp -d "${TMPDIR:-/tmp}/security-scan.XXXXXX")"
trap 'rm -rf "$scratch_dir"' EXIT

cd "$trusted_root"

if [[ -n "$prebuilt_tools" ]]; then
    tool_gobin="$prebuilt_tools"
    bash "$script_dir/install-security-tools.sh" --verify "$tool_gobin"
else
    tool_gobin="$scratch_dir/bin"
    bash "$script_dir/install-security-tools.sh" "$tool_gobin"
fi

if [[ "$scan_scope" == broker ]]; then
    cd "$scan_root"
    module_path="$(GOTOOLCHAIN=local GOFLAGS=-mod=readonly go list -m -f '{{.Path}}')"
    package_list="$(GOTOOLCHAIN=local GOFLAGS=-mod=readonly go list -deps \
        -f "{{with .Module}}{{if eq .Path \"$module_path\"}}{{\$.Dir}}{{end}}{{end}}" \
        ./cmd/agentic-identity-broker)"
    packages=()
    while IFS= read -r package; do
        if [[ -n "$package" ]]; then
            packages+=("$package")
        fi
    done <<< "$package_list"
    if (( ${#packages[@]} == 0 )); then
        echo "production broker import closure is empty" >&2
        exit 1
    fi
    echo "==> gosec: ${#packages[@]} first-party production broker packages"
    GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$tool_gobin/gosec" \
        -nosec-require-rules -nosec-require-justification "${packages[@]}"
    echo "==> govulncheck: production broker and reachable dependencies"
    GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$tool_gobin/govulncheck" ./cmd/agentic-identity-broker
    exit 0
fi

trusted_config="$trusted_root/osv-scanner.toml"
if [[ ! -f "$trusted_config" ]]; then
    trusted_config="$scratch_dir/osv-scanner.toml"
    printf 'ScanGoModVersion = false\n' > "$trusted_config"
fi

modules=(
    .
    infra/cdk
    mocks/cimd-server
    mocks/mcp-server
    mocks/sample-agent
    mocks/third-party-service
    mocks/upstream-oauth2-server
)

# govulncheck has no ignore file. Each entry names a module, a called advisory it may
# report, and the UTC date the exception ends. Every entry needs an osv-scanner.toml
# entry for the same advisory that records the reason.
govulncheck_exceptions=(
    "mocks/sample-agent GO-2026-6911 2026-12-31"
)

run_govulncheck() {
    local module="$1" output status today id excepted entry entry_module entry_id entry_until
    local -a ids
    set +e
    output="$(cd "$scan_root/$module" && GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$tool_gobin/govulncheck" ./... 2>&1)"
    status=$?
    set -e
    printf '%s\n' "$output"
    if (( status == 0 )); then
        return 0
    fi
    if (( status != 3 )); then
        return "$status"
    fi

    mapfile -t ids < <(awk '/^=== (Package|Module) Results ===$/ { exit } match($0, /^Vulnerability #[0-9]+: GO-[0-9]+-[0-9]+$/) { print $3 }' <<<"$output")
    if (( ${#ids[@]} == 0 )); then
        echo "govulncheck reported vulnerabilities in $module that this script cannot identify" >&2
        return 1
    fi

    today="$(date -u +%F)"
    for id in "${ids[@]}"; do
        excepted=false
        for entry in "${govulncheck_exceptions[@]}"; do
            read -r entry_module entry_id entry_until <<<"$entry"
            if [[ "$entry_module" == "$module" && "$entry_id" == "$id" && "$today" < "$entry_until" ]]; then
                excepted=true
                echo "govulncheck: $id in $module is excepted until $entry_until; see osv-scanner.toml" >&2
            fi
        done
        if ! $excepted; then
            echo "govulncheck: $id in $module has no current exception" >&2
            return 1
        fi
    done
}

for module in "${modules[@]}"; do
    echo "==> gosec: $module"
    (
        cd "$scan_root/$module"
        GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$tool_gobin/gosec" \
            -nosec-require-rules \
            -nosec-require-justification \
            ./...
    )

    echo "==> govulncheck: $module"
    run_govulncheck "$module"
done

lockfiles=(
    go.mod
    infra/cdk/go.mod
    mocks/cimd-server/go.mod
    mocks/mcp-server/go.mod
    mocks/sample-agent/go.mod
    mocks/third-party-service/go.mod
    mocks/upstream-oauth2-server/go.mod
    web/package-lock.json
    assets/docusaurus/package-lock.json
    tools/wireloom/package-lock.json
)

osv_args=(
    scan source
    --all-vulns
    --no-call-analysis=go
    "--config=$trusted_config"
)
for lockfile in "${lockfiles[@]}"; do
    osv_args+=(--lockfile "$scan_root/$lockfile")
done

echo "==> osv-scanner: dependency manifests"
"$tool_gobin/osv-scanner" "${osv_args[@]}"
