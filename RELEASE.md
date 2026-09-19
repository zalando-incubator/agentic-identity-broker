# Release procedure

## New minor series

1. Cut the documentation snapshot with `just docs-version X.Y` (for example, `just docs-version 0.2`).
2. Delete the six maintainer documents listed in the docs preset's `exclude` array from the new snapshot.
3. Set `lastVersion` in `assets/docusaurus/docusaurus.config.js` to the new minor series.
4. Merge the documentation changes to `main` before you create the release tag.
5. Create a `vX.Y.Z` tag on the release commit with `git tag vX.Y.Z`.
6. Push the tag to the repository with `git push <remote> vX.Y.Z`.

Patch releases need no new snapshot. Use steps 5 and 6 for a patch release.

A patch release needing no snapshot does not mean the released documentation needs no
maintenance. `/docs/` serves the snapshot, so a correction applied only to `docs/` reaches
`/docs/next/` and never reaches the readers running the released version. Apply corrections
that affect a supported release to that release's snapshot as well. Keep documentation for
unreleased behaviour out of the snapshot.

## Release automation

The [release workflow](.github/workflows/release.yml) runs when a `v*.*.*` tag reaches the repository.
It builds and pushes three GHCR images: the broker, migrations (`-migrate`), and ExtProc (`-extproc`).
It packages and pushes the Helm chart as an OCI artifact, then creates the GitHub Release with generated notes.
If the GitHub Release already exists, the workflow leaves it unchanged.

The [documentation workflow](.github/workflows/docs.yml) deploys documentation changes from `main` to GitHub Pages.
The release workflow does not deploy documentation.

## Documentation build selection

By default, documentation builds include all versions.
`DOCS_ONLY_INCLUDE_VERSIONS` accepts comma-separated version names, such as `current,0.1`.
The selection must include `lastVersion`. `current` refers to the `next 🚧` documentation.

```sh
DOCS_ONLY_INCLUDE_VERSIONS=current,0.1 just docs-build
```
