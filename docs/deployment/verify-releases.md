---
title: "Verify release artifacts"
description: Verify that release images, archives, and the Helm chart come from our workflow at the selected tag before deployment.
---

# Verify release artifacts

Every `vX.Y.Z` tag starts our release workflow. The workflow signs SLSA build provenance
for its images and release assets.

Before you deploy a release, verify that each artifact came from this workflow
at that tag.

The workflow stores attestations in two places:

- GHCR stores attestations with each image: `ghcr.io/zalando-incubator/agentic-identity-broker`,
  `-migrate`, and `-extproc`.
- The GitHub Release stores one Sigstore bundle for the binary archives
  (`*.tar.gz`), the Helm chart (`*.tgz`), and `checksums.txt`.
  The bundle is `agentic-identity-broker_<version>.intoto.jsonl`.

Each verification command specifies the repository, the release workflow,
and the tag.
Attestations from a fork, a different workflow, or a different tag fail verification.

## Verify with GitHub CLI

Log in to GitHub with `gh`. Before you verify images, log in to GHCR with
`docker login ghcr.io`.

Set the repository, release tag, and workflow:

```bash
REPO=zalando-incubator/agentic-identity-broker
TAG=vX.Y.Z
WORKFLOW="$REPO/.github/workflows/release.yml"
```

Verify the images:

```bash
for IMAGE in "ghcr.io/$REPO" "ghcr.io/$REPO-migrate" "ghcr.io/$REPO-extproc"; do
  gh attestation verify "oci://$IMAGE:$TAG" --repo "$REPO" \
    --signer-workflow "$WORKFLOW" --source-ref "refs/tags/$TAG"
done
```

To verify the broker archive for amd64, run these commands:

```bash
FILE="agentic-identity-broker_${TAG#v}_linux_amd64.tar.gz"
gh release download "$TAG" --repo "$REPO" --pattern "$FILE"
gh attestation verify "$FILE" --repo "$REPO" \
  --signer-workflow "$WORKFLOW" --source-ref "refs/tags/$TAG"
```

To verify a different release asset, replace the `FILE` value in the example
with its filename.

## Verify with cosign

Use cosign 3.0 or later.

Older versions search for a different attestation format by default.
As a result, they do not find our attestations.

Set the repository, release tag, identity, and issuer:

```bash
REPO=zalando-incubator/agentic-identity-broker
TAG=vX.Y.Z
IDENTITY="https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$TAG"
ISSUER=https://token.actions.githubusercontent.com
```

Verify the images:

```bash
for IMAGE in "ghcr.io/$REPO" "ghcr.io/$REPO-migrate" "ghcr.io/$REPO-extproc"; do
  cosign verify-attestation --type slsaprovenance1 \
    --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER" \
    "$IMAGE:$TAG" > /dev/null
done
```

To verify the broker archive for amd64, run these commands:

```bash
FILE="agentic-identity-broker_${TAG#v}_linux_amd64.tar.gz"
BUNDLE="agentic-identity-broker_${TAG#v}.intoto.jsonl"
for NAME in "$FILE" "$BUNDLE"; do
  curl --fail --location --remote-name \
    "https://github.com/$REPO/releases/download/$TAG/$NAME"
done
cosign verify-blob-attestation --bundle "$BUNDLE" --type slsaprovenance1 \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER" \
  "$FILE"
```

To verify a different release asset, replace the `FILE` value in the example
with its filename.

## Verify many files at once

The `checksums.txt` file lists every archive and the Helm chart.

In either asset example, replace the `FILE` assignment with `FILE=checksums.txt`.
Run that example to verify the checksum file.
Then compare the downloaded files with it:

```bash
sha256sum --check --ignore-missing checksums.txt
```

On macOS, use `shasum -a 256 --check --ignore-missing checksums.txt`.

## Verify the Helm chart from GHCR

The workflow publishes the same chart archive to GHCR and the GitHub Release.

Pull the archive from `oci://ghcr.io/zalando-incubator/agentic-identity-broker`:

```bash
helm pull oci://ghcr.io/zalando-incubator/agentic-identity-broker --version "${TAG#v}"
FILE="agentic-identity-broker-${TAG#v}.tgz"
```

If you use GitHub CLI, run the `gh attestation verify` command from the GitHub
CLI asset example.

If you use cosign, set `BUNDLE` as shown in the cosign asset example.
Download that bundle from the GitHub Release.
Then run the `cosign verify-blob-attestation` command from the cosign asset example.

Install the chart from the verified archive.

Deploy images with the exact `vX.Y.Z` tag that you verified.

The `vX.Y` tag changes with each patch release.

If verification fails, do not deploy the artifact. Report the failure through our
[security policy](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/SECURITY.md).
