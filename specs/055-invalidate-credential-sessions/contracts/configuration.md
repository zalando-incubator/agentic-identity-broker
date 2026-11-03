# Proposed configuration contract: monotonic credential publications

**Status**: Proposed API-CONFIG and D-PUBLICATION. Requires maintainer sign-off. No new dependency or HTTP route.

## Required filesystem binding key

```yaml
third_party_oauth2:
  credential_files:
    github-production:
      client_id_file: /meta/github/client-id
      client_secret_file: /meta/github/client-secret
      credential_revision_file: /meta/github/credential-revision
```

`third_party_oauth2.credential_files.<canonical-id>.credential_revision_file` is a required non-empty path for every configured filesystem binding, with no default. It follows the existing credential-path validation and configuration precedence through `internal/ports/config.go`, loader and validator; environment/CLI representations follow the existing binding-map mechanism rather than inventing a second loader. Missing configuration rejects startup. Stored services require no publication file.

The opened revision file must be regular and pass the same descriptor/path/size safety as existing credential files. Its content is 1–19 ASCII decimal digits representing an integer from 1 through 9223372036854775807, optionally followed by a single LF. Leading zeros are permitted and compare numerically; whitespace other than the optional final LF, signs, empty values and overflow are rejected. At most 20 bytes are read. Runtime absence, incoherence, malformed content, stale revision or equal-revision/different-pair publication maps to the existing source-unavailable error, not a credential change, recovery URI or fallback.

Publish all three files in one immutable projected-volume generation. Use a single service-scoped revision authority across replicas. Increase the revision for every effective pair change, including intentional restoration. Example: client A at 41, client B at 42, restored A at 43. Never replay 41 as restoration. A higher revision with unchanged normalized credentials advances durable revision evidence only, with no generation change/session invalidation. Re-entry into filesystem after a stored interval requires a revision above the retained high-water mark. Revision exhaustion requires converting to stored before retiring/replacing the service through existing lifecycle controls; never wrap or reset live evidence.

## Deployment contract

Chart path `broker.thirdPartyOauth2.credentialFiles.<canonical-id>` carries the same snake-case keys through the existing ConfigMap mapping. Update `values.yaml` parameter comments/example and README, and add a render test asserting `credential_revision_file` reaches broker configuration. No new top-level chart setting or mount creation is introduced: the operator supplies the projected volume as before.

Apply migration 046 before new readers; supply coherent revision publications before replacing old replicas. Keep filesystem traffic, background refresh and sweeps off old replicas throughout cutover; resume only after all serving replicas enforce this contract. Unbaselined existing filesystem sessions are conservatively invalidated on first valid acquisition. Metadata-only listings remain independent of files.

Before rollback/DOWN, convert filesystem services to stored with explicit credentials, remove filesystem traffic and drain authorization contexts for their 10-minute TTL. Preserve ADR 041 rollback compatibility prerequisites. Do not use schema rollback to erase revision evidence while continuing filesystem traffic.

**Rationale**: A monotonic revision independently rejects stale mounts and delayed acquisitions. **Main alternative**: stop/drain all replicas on every publication and verify all mounts before resuming; it sacrifices availability and cannot independently reject a stale cold replica. The added required key is a backward-incompatible configuration change and remains Proposed.
