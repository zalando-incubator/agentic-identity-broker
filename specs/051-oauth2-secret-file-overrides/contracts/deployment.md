# Contract: Helm and Target Deployment

**Propagated**: 2026-10-07 — Updated from spec.md refinement

**Updated**: 2026-10-08 — Aligned with implemented chart/runtime behavior, regenerated external manifests, and verified local-image evidence.

This contract describes implemented chart support and current external deployment inputs/generated manifests. The feature remains unreleased. CDP image placeholders remain. Local validation does not establish registry publication, live deployment, or a completed full final gate.

## Chart value

The existing third-party OAuth2 values object contains:

```yaml
broker:
  thirdPartyOauth2:
    credentialFiles: {}
```

The value maps exact canonical IDs to pair objects. Each pair requires two absolute-path strings: `client_id_file` and `client_secret_file`. The default is `{}`. Default rendering omits `third_party_oauth2.credential_files` from broker configuration.

~~`broker.thirdPartyOauth2.clientSecretFiles` renders secret-only bindings as `third_party_oauth2.client_secret_files`.~~ Superseded because filesystem mode requires both credential files. No old-name alias exists.

An empty value supplies no bindings. It never changes a registered filesystem service to stored mode. When an operation requires the missing binding, that service fails.

A configured mapping renders under the existing `third_party_oauth2` block in the broker ConfigMap. Key spelling and case remain unchanged. Both paths belong in this ConfigMap, but neither credential value belongs there.

These chart files define the implemented contract:

- `charts/agentic-identity-broker/values.yaml`, including the existing `# --` documentation style
- `charts/agentic-identity-broker/values.schema.json`, with object-valued bindings and both required string-path fields
- `charts/agentic-identity-broker/templates/configmap.yaml`
- `charts/agentic-identity-broker/README.md`.

The schema rejects invalid mapping shapes, non-object pairs, missing fields, and non-string or empty paths. Broker startup also rejects whitespace-only and relative paths. The broker owns complete canonical-ID validation, including UUID-shaped key rejection. Helm schema validation complements startup validation. Neither stage opens credential files or queries registered services.

T064 recorded semantic-red evidence from all 50 authored acceptance journeys before chart or broker behavior. Typed rendering/schema tests passed in T065 before broker production implementation. All 50 feature cases subsequently passed in individual story runs. Chart checks do not replace the full final gate.

## Existing mount inputs

No new volume or mount input exists. The current target inputs use:

```yaml
broker:
  thirdPartyOauth2:
    credentialFiles:
      zalando-platform:
        client_id_file: /meta/credentials/employee-client-id
        client_secret_file: /meta/credentials/employee-client-secret
  extraVolumes:
    - name: credentials
      secret:
        secretName: agentic-platform-credentials
  extraVolumeMounts:
    - name: credentials
      mountPath: /meta/credentials
      readOnly: true
```

**Current delivery evidence, 2026-10-08:** Test, prod, and sandbox source inputs contain the exact pair and read-only credential directory mount through existing chart inputs. The existing generator regenerated all three targets from the current local working chart. Validation preserved credential, application-configuration, and `/tmp` mounts and non-root settings. No live deployment occurred.

This fragment combines the required mapping with the existing chart mount mechanism. It is not a full broker configuration or a Kubernetes Secret definition.

Preserve the chart's existing application-configuration and `/tmp` mounts. Credential paths must not collide with those mounts. Use a read-only directory mount without `subPath` on the credential file. The existing application-configuration mount is unaffected.

The chart's current pod identity is UID/GID 1000 with `fsGroup: 1000`. Both files need read permission for that identity. Every parent directory needs traversal permission. Read-only mount mode does not itself grant those permissions.

## Target association

| Deployment element | Required value |
|---|---|
| Registered eligible service canonical ID | `zalando-platform` |
| Credential Secret | `agentic-platform-credentials` |
| Read-only mount directory | `/meta/credentials` |
| Client-ID file | `/meta/credentials/employee-client-id` |
| Client-secret file | `/meta/credentials/employee-client-secret` |
| Chart mapping | `broker.thirdPartyOauth2.credentialFiles.zalando-platform` |
| Registered service source | `credential_source: filesystem`, with neither inline credential |

~~Only the mapping selects file-backed authentication for the service.~~ Superseded because the registered service must explicitly select filesystem mode. The mapping associates its exact canonical ID with paths. Client labels, display names, Secret names, and filenames do not select a service or its source.

Register the eligible service with canonical ID `zalando-platform`. Set `credential_source` to `filesystem` through the administrative service interface. Omit both `client_id` and `client_secret`, including empty, null, and placeholder values. Keep the service's other required metadata.

The source-selector fragment is not a complete registration request:

```json
{
  "credential_source": "filesystem"
}
```

For an existing stored service, the administrative source transition removes both stored credentials atomically. Deployment values never perform that transition or create the service. Admin API `2.0.0` implements the approved source-aware contract. Registration remains a separate operator action. Generated manifests do not prove a live service transition.

~~The `employee` client ID remains stable, and this feature does not rotate or override client IDs.~~ Superseded because filesystem mode reads both credentials. A client-ID change applies to new authorizations. Existing codes and sessions with another recorded identity require a new connection.

## Deployment repository inputs and outputs

The deployment repository contains the same one-entry pair mapping in `deploy/values/{test,prod,sandbox}.yaml`. Each input uses both paths from the target-association table and the existing supplemental mount mechanism. Unrelated values remain unchanged. Separately, register or update `zalando-platform` with filesystem source and no inline credentials.

The deployment README documents the pair association, explicit source selection, plain-file format, non-root permissions, and fail-closed behavior. It also explains consistent pair publication, provider validity overlap, and the client-ID reconnect rule.

Regenerate with the existing `scripts/update-k8s-manifests.sh`. Use `CHART_REPO_DIR` to select chart source containing this feature during local validation. Select a broker image version containing the implementation.

The script writes manifests under `deploy/kubernetes/<env>/` and records the latest chart path commit in `scripts/chart_ref`. That file is an output, not an input checkout pin. Current generation used the local working chart, including uncommitted feature changes. `scripts/chart_ref` remains `ad2289121b459ea14d56efd7d227bbf71d452948`, a historical chart path revision. That revision alone does not reproduce the working chart or generated outputs. Do not edit generated manifests as the source of the binding.

A packaged-chart release is not required for local source-based generation. Production rollout still requires the corresponding feature-capable runtime image and deployment release process.

**Local image evidence, 2026-10-08:** The linux/arm64 release image `agentic-identity-broker:oauth2-credential-files-051` was built and verified with UID 1000. Its local image ID is `sha256:f128f4a9237e272b32773d87682fd8ca06b6aea2247a9b984c9634b95d9a7ab0`. This is a local tag and image ID, not a published release tag or registry digest. CDP `DEP_BROKER_VERSION` placeholders remain in the generated manifests. No registry publication or live deployment occurred.

## Rotation delivery contract

Each file contains plaintext, not JSON or a credential document. Each mounted target must be a regular file of at most 65,536 bytes. The broker removes surrounding whitespace and preserves internal characters. An empty or whitespace-only required value is unusable. Non-regular sources must fail without blocking.

The provider must publish a complete, consistent pair through fresh immutable targets. Never modify published targets or republish retired targets during acquisition. Partial pair publication lies outside the delivery contract. Ordinary projected-volume symlinks and target replacement remain supported. AIB consumes files without querying the provider. No new mount field or credential-file `subPath` is required.

Authorization initiation reopens only the client-ID path. Each code-exchange or refresh attempt acquires one pair with `ReadPair`. The adapter opens both targets before either bounded read. After both reads, it revalidates both configured paths with `os.SameFile` against their open descriptors. Both descriptors close before return. Detected target changes return `generation_changed` before provider authentication, without partial values or acquisition retries.

Publication after validation can leave an in-flight operation using that validated pair. Established-identity checks and provider validity still apply. Later acquisitions use current targets. The broker retains no descriptor, cross-operation credential cache, or previous value. File replacement requires neither restart nor registration update.

Secret rotation with the same client ID can preserve sessions. Continuity depends on old/new provider validity overlap across file delivery and in-flight authentication. This feature does not promise uninterrupted rotation. A usable but rejected pair remains a provider rejection. An unusable required source fails closed before provider authentication.

After client ID A changes to B, a new authorization uses B. The broker must not exchange A's code or refresh A's session as B. Those operations stop before provider authentication and require a new connection. Provider validity overlap does not migrate an existing code or session to another identity.

Filesystem code exchange and refresh require a recorded identity. Both eligible source modes compare recorded identities before authentication, including across source transitions. A missing identity is usable only for stored services whose internal `credential_source_transitioned` marker is false. An actual source transition atomically sets that marker to true, and later transitions never reset it.

Returning to stored cannot restore legacy missing-identity reuse. Excluded modes remain unchanged. Never infer identity from current credentials.

## Validation requirements

These maintained validation requirements define the deployment contract. Historical chart checks, completed story runs, external generator validation, and local image evidence are recorded separately. They do not establish a completed full final gate or live provider timing:

| Scenario | Required evidence |
|---|---|
| Default chart values | No rendered binding and no new credential mount. Existing filesystem service source remains unchanged, with missing-binding failure at use. |
| Non-empty mapping | Exactly both configured paths and the literal key reach embedded broker YAML and loaded configuration. |
| Invalid chart or startup input | Invalid shapes, missing pair fields, and invalid field values are rejected at the required validation stage. No file or service lookup occurs at startup. |
| Existing supplemental mount | Credential, application-configuration, and temporary-storage mounts all remain intact. |
| Non-root operation | A real non-root filesystem broker connects with a synthetic pair and refreshes after secret replacement with the same client ID. |
| Target source generation | All three environment inputs regenerate to exactly the two target paths for `zalando-platform`. Live registration separately requires filesystem source without inline placeholders. Generation does not perform registration. |
| Projection rotation | The next acquisition uses a coherent current pair. Detected changes produce `generation_changed` and zero provider requests. A later operation recovers. Publication after validated acquisition retains only that in-flight pair. |
| Client-ID change | New authorization uses B. Codes and sessions established as A stop before provider authentication and require reconnection. |

Portable E2E fixtures carry the target input fragment without production secrets or a developer-specific external path. The real external generator completed separate validation against all three environment inputs. CDP image selection and live registration/deployment remain operator delivery actions.

For runtime errors and events, see [outbound-authentication.md](./outbound-authentication.md). For runnable commands, see [quickstart.md](../quickstart.md).
