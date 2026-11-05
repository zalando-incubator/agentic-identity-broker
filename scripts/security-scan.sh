#!/usr/bin/env bash

set -euo pipefail

if (( $# > 1 )); then
    echo "usage: $0 [scan-root]" >&2
    exit 2
fi

scan_root_input="${1:-.}"
if [[ ! -d "$scan_root_input" ]]; then
    echo "scan root must be an existing directory: $scan_root_input" >&2
    exit 2
fi
scan_root="$(cd "$scan_root_input" && pwd -P)"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
trusted_root="$(cd "$script_dir/.." && pwd -P)"
scratch_dir="$(mktemp -d "${TMPDIR:-/tmp}/security-scan.XXXXXX")"
trap 'rm -rf "$scratch_dir"' EXIT

cd "$trusted_root"

# SECURITY_SCAN_TOOLS_DIR names scanners built before an environment that cannot
# compile them, such as the agent sandbox; they must match the pinned builds.
if [[ -n "${SECURITY_SCAN_TOOLS_DIR:-}" ]]; then
    tool_gobin="$SECURITY_SCAN_TOOLS_DIR"
    bash "$script_dir/install-security-tools.sh" --verify "$tool_gobin"
else
    tool_gobin="$scratch_dir/bin"
    bash "$script_dir/install-security-tools.sh" "$tool_gobin"
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
    (
        cd "$scan_root/$module"
        GOTOOLCHAIN=local GOFLAGS=-mod=readonly "$tool_gobin/govulncheck" ./...
    )
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
