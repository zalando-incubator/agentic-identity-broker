# Contract: ExtProc Configuration — Authorization Section

**Branch**: `20260314-120100-extproc-opa-authorization`
**Date**: 2026-03-14

## Configuration Schema

The authorization configuration is added as a new top-level section in the ExtProc YAML configuration, alongside the existing `grpc`, `oauth2`, `cache`, and `log` sections.

### YAML Schema

```yaml
# Authorization configuration (optional — disabled by default)
authorization:
  # Enable or disable OPA authorization
  enabled: false  # boolean, default: false

  policy:
    # Filesystem path to a local Rego policy file or directory
    # Mutually exclusive with config_file
    path: ""  # string, optional

    # Filesystem path to an OPA configuration file (for bundles, discovery, etc.)
    # Mutually exclusive with path
    config_file: ""  # string, optional

    # OPA policy package name (dotted notation)
    package: "aib.extproc.authz"  # string, default: "aib.extproc.authz"

    # OPA decision document name within the package
    decision: "result"  # string, default: "result"

  # Decision when the policy result is undefined.
  default_decision: "deny"  # deny only; retained for compatibility

  # Maximum time for a single OPA evaluation
  evaluation_timeout: 100ms  # duration, default: 100ms

  # Maximum request body size to buffer for OPA evaluation (bytes)
  max_body_size: 1048576  # integer, default: 1048576 (1 MiB)
```

### Environment Variable Mapping

| YAML Path | Environment Variable | Type |
|-----------|---------------------|------|
| `authorization.enabled` | `EXTPROC_AUTHORIZATION_ENABLED` | boolean |
| `authorization.policy.path` | `EXTPROC_AUTHORIZATION_POLICY_PATH` | string |
| `authorization.policy.config_file` | `EXTPROC_AUTHORIZATION_POLICY_CONFIG_FILE` | string |
| `authorization.policy.package` | `EXTPROC_AUTHORIZATION_POLICY_PACKAGE` | string |
| `authorization.policy.decision` | `EXTPROC_AUTHORIZATION_POLICY_DECISION` | string |
| `authorization.default_decision` | `EXTPROC_AUTHORIZATION_DEFAULT_DECISION` | string |
| `authorization.evaluation_timeout` | `EXTPROC_AUTHORIZATION_EVALUATION_TIMEOUT` | duration |
| `authorization.max_body_size` | `EXTPROC_AUTHORIZATION_MAX_BODY_SIZE` | integer |

### Validation Rules

| Rule | Condition | Error Message |
|------|-----------|---------------|
| Disabled with policy source | `enabled: false` and `path` or `config_file` set | `authorization.policy.path or authorization.policy.config_file is set but authorization.enabled is false` |
| Mutual exclusivity | Both `path` and `config_file` set | `authorization.policy: only one of path or config_file may be specified` |
| Required source | `enabled: true` but neither `path` nor `config_file` set | `authorization.policy: one of path or config_file must be specified when authorization is enabled` |
| Path existence | `path` set but file/directory does not exist | `authorization.policy.path: file or directory not found: {path}` |
| Path traversal | `path` contains `../` | `authorization.policy.path: path traversal (../) is not allowed` |
| Config file existence | `config_file` set but file does not exist | `authorization.policy.config_file: file not found: {path}` |
| Default decision fail-closed | `default_decision != "deny"` | `authorization.default_decision must be deny; got "{value}"` |
| Timeout positive | `evaluation_timeout` ≤ 0 | `authorization.evaluation_timeout: must be a positive duration` |
| Body size positive | `max_body_size` ≤ 0 | `authorization.max_body_size: must be a positive integer` |
| Package non-empty | `enabled: true` and `package` is empty | `authorization.policy.package: must not be empty when authorization is enabled` |
| Decision non-empty | `enabled: true` and `decision` is empty | `authorization.policy.decision: must not be empty when authorization is enabled` |

Relative paths resolve against the ExtProc process working directory. `authorization.policy.path` may point to a file or directory; `authorization.policy.config_file` must point to a file.

### Example Configurations

**Local Rego file** (quick setup):

```yaml
authorization:
  enabled: true
  policy:
    path: "/etc/extproc/policy.rego"
    package: "aib.extproc.authz"
    decision: "result"
  default_decision: "deny"
  evaluation_timeout: 100ms
  max_body_size: 1048576
```

**OPA bundle server** (production):

```yaml
authorization:
  enabled: true
  policy:
    config_file: "/etc/extproc/opa-config.yaml"
    package: "aib.extproc.authz"
    decision: "result"
  default_decision: "deny"
  evaluation_timeout: 100ms
  max_body_size: 1048576
```

**Disabled** (default / backward-compatible):

```yaml
authorization:
  enabled: false
```

---

## CLI Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--authorization.enabled` | | `false` | Enable OPA authorization |
| `--authorization.policy.path` | | `""` | Path to local Rego policy file |
| `--authorization.policy.config_file` | | `""` | Path to OPA config file |
| `--authorization.policy.package` | | `aib.extproc.authz` | OPA package name |
| `--authorization.policy.decision` | | `result` | OPA decision document name |
| `--authorization.default_decision` | | `deny` | Default decision when no rule matches |
| `--authorization.evaluation_timeout` | | `100ms` | OPA evaluation timeout |
| `--authorization.max_body_size` | | `1048576` | Max request body size for OPA (bytes) |
