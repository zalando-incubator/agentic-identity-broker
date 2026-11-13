#!/usr/bin/env bash

# Installs the pinned OpenTelemetry Weaver release next to the other Go tools and prints
# the path of the binary. The telemetry recipes in the justfile call it. It downloads
# the release only when the binary is missing or has another version, and checks the
# archive against the SHA-256 digest recorded here.

set -euo pipefail

version="0.28.0"

case "$(uname -s)-$(uname -m)" in
    Linux-x86_64)
        asset="weaver-x86_64-unknown-linux-musl"
        digest="69069769ec3509c358ddbfbc29ff9965590fe4f8dd8b03e1c1e1be1ae56122e8"
        ;;
    Linux-aarch64 | Linux-arm64)
        asset="weaver-aarch64-unknown-linux-musl"
        digest="55a47e3d6226a372570b65853e56413266c149aaa7716172bdffb0dc8422d544"
        ;;
    Darwin-x86_64)
        asset="weaver-x86_64-apple-darwin"
        digest="40dd65d72b43bc97041b849203e450b322cd786629af8f926cfd2d05798d9ca4"
        ;;
    Darwin-arm64)
        asset="weaver-aarch64-apple-darwin"
        digest="b334b37372325f77374b461fd6a11d38bc050122832eb83a9732fcd6831a5ab1"
        ;;
    *)
        echo "install-weaver: no Weaver release for $(uname -s) $(uname -m)" >&2
        exit 1
        ;;
esac

bin_dir="$(go env GOBIN)"
if [[ -z "$bin_dir" ]]; then
    bin_dir="$(go env GOPATH)/bin"
fi
weaver="$bin_dir/weaver"

if [[ -x "$weaver" ]] && [[ "$("$weaver" --version 2>/dev/null)" == "weaver $version" ]]; then
    echo "$weaver"
    exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

archive="$tmp/$asset.tar.xz"
curl --proto '=https' --tlsv1.2 -fsSL -o "$archive" \
    "https://github.com/open-telemetry/weaver/releases/download/v$version/$asset.tar.xz"

if command -v sha256sum >/dev/null; then
    actual="$(sha256sum "$archive" | cut -d' ' -f1)"
else
    actual="$(shasum -a 256 "$archive" | cut -d' ' -f1)"
fi
if [[ "$actual" != "$digest" ]]; then
    echo "install-weaver: $asset.tar.xz has SHA-256 $actual, expected $digest" >&2
    exit 1
fi

tar -xJf "$archive" -C "$tmp"
mkdir -p "$bin_dir"
install -m 0755 "$tmp/$asset/weaver" "$weaver"
echo "$weaver"
