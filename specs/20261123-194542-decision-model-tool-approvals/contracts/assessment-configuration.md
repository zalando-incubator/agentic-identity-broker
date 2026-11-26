# Contract: Configuration and Question Set

**Feature**: [spec.md](../spec.md) | **Decisions**: [research.md](../research.md) D-17, D-18

**Approval status**: Draft. The stakeholder must approve this contract before implementation
(API-006).

## 1. ExtProc configuration keys

The keys are in the ExtProc configuration schema (`internal/extproc/config/config.go`,
`adrs/011-extproc-standalone-binary.md`). The Helm chart does not deploy ExtProc and does not
change.

| Key | Type | Default | Validation |
|---|---|---|---|
| `authorization.assessment.mode` | string | `disabled` | `disabled`, `shadow`, `guidance`, or `auto_approve` (FR-009). Assessment is enabled if the mode is not `disabled`. An enabled mode requires `authorization.enabled` and `tool_approvals.enabled`. |
| `authorization.assessment.endpoint` | URL | none | Required while assessment is enabled. HTTPS, or HTTP if `oauth2.tls.allow_http` is true. That development setting does not examine the host. With it, the context and the credential can go to each configured host without transport protection. |
| `authorization.assessment.model` | string | none | Required while assessment is enabled. |
| `authorization.assessment.credential_file` | path | none | Optional. The file has a maximum of 4 KiB. |
| `authorization.assessment.request_timeout` | duration | `500ms` | More than 0, maximum `2s`. |
| `authorization.assessment.max_context_bytes` | integer | `16384` | 1,024 to 262,144. |
| `authorization.assessment.max_listing_bytes` | integer | `1048576` | 1,024 to 8,388,608. For a larger `tools/list` response, ExtProc removes the stored definitions of that server (D-02). |
| `authorization.assessment.organization.name` | string | none | Required while assessment is enabled. Maximum 200 characters. |
| `authorization.assessment.organization.description` | string | none | Required while assessment is enabled. Maximum 1,000 characters. |
| `authorization.assessment.organization.internal_domains` | list of strings | none | Required while assessment is enabled. 1 to 50 entries. Each entry is a lowercase DNS name. |
| `authorization.assessment.questions` | list of questions | the default question set (section 2) | 1 to 16 entries. A list in the configuration replaces the full default list. |

The schema has no key for an individual target server or tool (FR-016). The keys use the existing
loader of `internal/extproc/config/`: the YAML file of `EXTPROC_CONFIG_PATH`, the `EXTPROC_`
environment variables for single values, the defaults in `loader.go`, and the rules in
`validate.go`. As for each other key, the loader ignores a key that the schema does not have.

Path values obey the existing rule for `authorization.policy.config_file`: no `..` segment, and
the path must be a file that exists at startup. A directory is not permitted, because each value
names one file.

The documentation and the examples change in these files: `docs/configuration.md`,
`examples/config/extproc-tool-assessment.yaml`, `examples/config/README.md`, `configs/config.extproc.docker.yaml`, and the configuration table in
`internal/extproc/AGENTS.md`.

## 2. Question set

The key `authorization.assessment.questions` is a list. Each entry has the fields of
[data-model.md](../data-model.md): `name`, `type`, `instructions`, and `yes_when` with `no_when`,
or `levels`.

The default value of the key is the default question set of [spec.md](../spec.md), Journey 1,
step 2. It is a part of the defaults in `internal/extproc/config/`. The example file
`examples/config/extproc-tool-assessment.yaml` shows the same questions in full, and a test makes
sure that the two are equal. A list in the configuration replaces the full default list.

A change to a default question changes the answers of each deployment that uses the default
set. The release notes must state such a change, because the thresholds of a policy can then be
wrong (SR-010).

ExtProc rejects the configuration at startup in these cases:

- A question without a required field
- A question name that is not unique
- A placeholder in a question text, a criterion, a level label, or a level description
- A text that is longer than its limit in [data-model.md](../data-model.md)
- A question set with no question or with more than 16 questions
- A `predicate` question without `yes_when` or `no_when`, or with `levels`
- A `score` question with fewer than 2 levels or more than 10 levels, or with `yes_when` or `no_when`.

The error message names the question or the key.
