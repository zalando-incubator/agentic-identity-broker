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
tool_gobin="$(mktemp -d "${TMPDIR:-/tmp}/security-scan.XXXXXX")"
trap 'rm -rf "$tool_gobin"' EXIT

cd "$trusted_root"

# gosec v2.29.0: proxy VCS commit deb54465fea23d19a77f037e11e6589021f8501d; module Go 1.25.0.
GOTOOLCHAIN=local GOBIN="$tool_gobin" go install github.com/securego/gosec/v2/cmd/gosec@v2.29.0
# govulncheck v1.8.0: proxy VCS commit 709015412431dd2b5b28a53c06c70bc02d49074c; module Go 1.26.0.
GOTOOLCHAIN=local GOBIN="$tool_gobin" go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
# OSV-Scanner v2.5.1: proxy VCS commit c84fa4568f2526d0333e9a914ea8a0a5f74ad68; module Go 1.26.5.
GOTOOLCHAIN=local GOBIN="$tool_gobin" go install github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.5.1

trusted_config="$trusted_root/osv-scanner.toml"
if [[ ! -f "$trusted_config" ]]; then
    trusted_config="$tool_gobin/osv-scanner.toml"
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
