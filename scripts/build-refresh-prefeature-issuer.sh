#!/usr/bin/env bash
set -euo pipefail

revision=b4bdd1cf4dce2c23fb65223a0b31a46d29d40008
artifact_dir=bin/refresh-prefeature

if ! git cat-file -e "${revision}^{commit}" 2>/dev/null; then
    git fetch --no-tags --depth=1 origin "$revision"
fi
if [ "$(git rev-parse "${revision}^{commit}")" != "$revision" ]; then
    echo "Historical issuer revision is not $revision" >&2
    exit 1
fi

mkdir -p "$artifact_dir"
artifact_dir="$(cd "$artifact_dir" && pwd)"
worktree="$(mktemp -d)"
cleanup() {
    git worktree remove --force "$worktree" 2>/dev/null || rm -rf "$worktree"
}
trap cleanup EXIT
rmdir "$worktree"
git worktree add --quiet --detach "$worktree" "$revision"

binary="$artifact_dir/agentic-identity-broker"
(
    cd "$worktree"
    GOFLAGS= CGO_ENABLED=0 go build -trimpath -buildvcs=true -ldflags='-s -w' \
        -o "$binary" ./cmd/agentic-identity-broker
)

if command -v sha256sum >/dev/null 2>&1; then
    checksum="$(sha256sum "$binary")"
else
    checksum="$(shasum -a 256 "$binary")"
fi
checksum="${checksum%% *}"
cat > "$binary.json" <<EOF
{
  "source_revision": "$revision",
  "sha256": "$checksum",
  "go_version": "$(go env GOVERSION)",
  "goos": "$(go env GOOS)",
  "goarch": "$(go env GOARCH)",
  "build_command": "GOFLAGS= CGO_ENABLED=0 go build -trimpath -buildvcs=true -ldflags='-s -w' ./cmd/agentic-identity-broker"
}
EOF
printf 'Historical issuer %s (%s/%s), SHA-256 %s\n' "$revision" "$(go env GOOS)" "$(go env GOARCH)" "$checksum"
