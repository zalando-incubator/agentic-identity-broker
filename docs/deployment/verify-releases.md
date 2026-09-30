---
title: "Verify release artifacts"
description: Check that the images, binaries and Helm chart of a release come from our release workflow at that tag before you deploy them.
---

# Verify release artifacts

Every `vX.Y.Z` tag runs our release workflow, and the workflow signs SLSA build provenance for
everything it publishes. Before you deploy a release, check that its artifacts come from that
workflow at that tag. It takes one command per artifact.

We attest two groups of artifacts:

| Artifact | Where we store the attestation |
| --- | --- |
| Images `ghcr.io/zalando-incubator/agentic-identity-broker`, `-migrate` and `-extproc` | In GHCR, next to each image |
| Binary archives (`*.tar.gz`), Helm chart (`*.tgz`) and `checksums.txt` on the GitHub Release | In one Sigstore bundle on the same release, `agentic-identity-broker_<version>_provenance.sigstore.json` |

Each command below pins the repository, the release workflow and the tag. An attestation from a
fork, from another workflow or from an older release fails the check.

## Verify with GitHub CLI

You need a logged-in `gh`. To check images, `gh` also has to read them from GHCR, so log in there
first (`docker login ghcr.io`). Set the release you plan to install:

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

Download the release assets you need and verify each file. The example uses the broker binary
for amd64:

```bash
FILE="agentic-identity-broker_${TAG#v}_linux_amd64.tar.gz"
gh release download "$TAG" --repo "$REPO" --pattern "$FILE"
gh attestation verify "$FILE" --repo "$REPO" \
  --signer-workflow "$WORKFLOW" --source-ref "refs/tags/$TAG"
```

## Verify with cosign

Use cosign 3.0 or later. Older versions look for a different attestation format by default and
don't find ours. Set the release and the expected signing identity:

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

Download the release assets you need together with the provenance bundle, and verify each file
against the bundle:

```bash
FILE="agentic-identity-broker_${TAG#v}_linux_amd64.tar.gz"
BUNDLE="agentic-identity-broker_${TAG#v}_provenance.sigstore.json"
for NAME in "$FILE" "$BUNDLE"; do
  curl --fail --location --remote-name \
    "https://github.com/$REPO/releases/download/$TAG/$NAME"
done
cosign verify-blob-attestation --bundle "$BUNDLE" --type slsaprovenance1 \
  --certificate-identity "$IDENTITY" --certificate-oidc-issuer "$ISSUER" \
  "$FILE"
```

## Verify many files at once

We also attest `checksums.txt`, which lists every archive and the chart. Verify it once with
either tool above (set `FILE=checksums.txt`), then check the files you downloaded against it:

```bash
sha256sum --check --ignore-missing checksums.txt
```

On macOS, use `shasum -a 256 --check --ignore-missing checksums.txt`.

## Verify the Helm chart from GHCR

We push the same chart archive to `oci://ghcr.io/zalando-incubator/agentic-identity-broker`.
Pull it and set `FILE` to the pulled archive:

```bash
helm pull oci://ghcr.io/zalando-incubator/agentic-identity-broker --version "${TAG#v}"
FILE="agentic-identity-broker-${TAG#v}.tgz"
```

Then run the `gh attestation verify` or `cosign verify-blob-attestation` command from above,
without the download step, and install from the verified file. Deploy images by the exact `vX.Y.Z`
tag you verified. We also move the `vX.Y` tag to each new patch release.

If a check fails, don't deploy the artifact. Report it through our
[security policy](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/SECURITY.md).
