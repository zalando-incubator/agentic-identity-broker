# Contract: Configuration, Question Set, Evaluation Record, and Evaluation Run

**Feature**: [spec.md](../spec.md) | **Decisions**: [research.md](../research.md) D-08, D-11, D-17

**Approval status**: Draft. The stakeholder must approve this contract before implementation
(API-006).

## 1. ExtProc configuration keys

The keys are in the ExtProc configuration schema (`internal/extproc/config/config.go`,
`adrs/011-extproc-standalone-binary.md`). The Helm chart does not deploy ExtProc and does not
change.

| Key | Type | Default | Validation |
|---|---|---|---|
| `authorization.assessment.enabled` | boolean | `false` | It requires `authorization.enabled` and `tool_approvals.enabled`. It requires `log.level` `debug` or `info`, because the audit record has the INFO level (FR-060, D-13). |
| `authorization.assessment.mode` | string | `recommend` | `recommend` or `auto_approve`. |
| `authorization.assessment.endpoint` | URL | none | Required while enabled. HTTPS, or HTTP if `oauth2.tls.allow_http` is true. That development setting does not examine the host. With it, the context and the credential can go to each configured host without transport protection. |
| `authorization.assessment.model` | string | none | Required while enabled. |
| `authorization.assessment.credential_file` | path | none | Optional. The file has a maximum of 4 KiB. |
| `authorization.assessment.request_timeout` | duration | `500ms` | More than 0, maximum `2s`. |
| `authorization.assessment.max_context_bytes` | integer | `16384` | 1,024 to 262,144. |
| `authorization.assessment.max_listing_bytes` | integer | `1048576` | 1,024 to 8,388,608. For a larger `tools/list` response, ExtProc removes the stored definitions of that server (D-02). |
| `authorization.assessment.organization.name` | string | none | Required while enabled. Maximum 200 characters. |
| `authorization.assessment.organization.description` | string | none | Required while enabled. Maximum 1,000 characters. |
| `authorization.assessment.organization.internal_domains` | list of strings | none | Required while enabled. 1 to 50 entries. Each entry is a lowercase DNS name. |
| `authorization.assessment.questions` | list of questions | the default question set (section 2) | 1 to 16 entries. A list in the configuration replaces the full default list. |
| `authorization.assessment.auto_approval.evaluation_record_file` | path | none | Required in the `auto_approve` mode. Optional in the `recommend` mode. In that mode, ExtProc reads it if it is set, and `would_auto_approve` uses it (FR-044). Without it, `would_auto_approve` is always false. |

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

A change to a default question changes the question set fingerprint of each deployment that uses
the default set. The release notes must state such a change, because automatic approval stops
until a new evaluation run (SC-010).

ExtProc rejects the configuration at startup in these cases:

- A question without a required field
- A question name that is not unique
- A placeholder in a question text, a criterion, a level label, or a level description
- A text that is longer than its limit in [data-model.md](../data-model.md)
- A question set with no question or with more than 16 questions
- A `predicate` question without `yes_when` or `no_when`, or with `levels`
- A `score` question with fewer than 2 levels or more than 10 levels, or with `yes_when` or `no_when`.

The error message names the question or the key. At startup, ExtProc writes the question set
fingerprint to the log.

## 3. Evaluation record file

The format is YAML. Only the evaluation run writes it.

```yaml
format: 1
model: "jev-1.14.0"
question_set: "qs-<64 hexadecimal characters>"
tool_contexts:
  google-drive-mcp:
    drive.share_file: "tc-<64 hexadecimal characters>"
```

In the `auto_approve` mode, ExtProc rejects a missing or invalid file at startup. If the
`question_set` value differs from the fingerprint of the service, ExtProc starts and writes an error record. No call
then proceeds automatically (SC-010). A changed question or organization value is a normal change and not an invalid
file. The calls wait for a person until a new evaluation run.

## 4. Case file

The format is JSON Lines. Each line is one case.

| Field | Type | Rules |
|---|---|---|
| `id` | string | Required. Unique in the file. |
| `expect` | `human` or `automatic_ok` | Required. `human`: a person must see the call. |
| `target_server_name` | string | Required. |
| `tool_name` | string | Required. |
| `arguments` | JSON object | Required. It can be empty. |
| `expected_answers` | JSON object | Optional. The key is a question name. The value is `yes` or `no` for a `predicate` question, or the level index for a `score` question (FR-054). |
| `context` | JSON object | Optional. The keys of the production input `context`: `granted_permission_sets`, `granted_permission_sets_available` (default `true`), and `agent_session_id` (default: absent). |
| `request` | JSON object | Optional. The HTTP values of the production input: `method` (default `POST`), `path` (default `/mcp`), `host` (default: empty), `scheme` (default `https`), `headers` (default: none), and `mcp_session_id` (default: absent). |

The run builds the policy input of each case with the input builder of production (FR-050). It
rejects a case with an unknown key. A policy that reads a request value gets the default if the
case does not supply the value. The report gives the number of cases that used a default.

## 5. Tool listing file

One file for each target server. The content is the unmodified `result` object of a `tools/list`
response, or the full JSON-RPC response. A file with two or more pages is a JSON list of such
objects. The file must contain the listing as the client sees it through the gateway. On a route
with two or more MCP targets, the tool names thus have the target prefix of the gateway (D-02).

A listing file must contain all pages of one listing. The run rejects a file in which the last
page has a `nextCursor` value, because later pages are missing.

## 6. Command

```text
extproc-token-exchange assessment evaluate
    --config <ExtProc configuration file>
    --cases <case file>
    --tool-listing <target server name>=<listing file>     (one time for each server)
    --record-out <evaluation record file>
    [--report-out <report file>]                           (default: standard output)
    [--parallel <number>]                                  (default: 4, maximum: 32)
```

The command uses the organization values, the question set, the model
settings, and the policy of the configuration file. It
ignores `mode` and an existing evaluation record. It does not require that
`evaluation_record_file` exists. It sets `input.assessment.evaluated` to `true` for each case
(D-08). It starts no gRPC server, calls no tool, and sends no request to the broker.

| Exit code | Meaning |
|---|---|
| 0 | The run completed. False approvals do not change the exit code. |
| 1 | An input file or the configuration is invalid. |
| 2 | The run did not write an evaluation record, because the responses named two or more models, or no response named a model. |

## 7. Report

The report is plain text. It contains these values (FR-051):

- The question set fingerprint and each answering model
- Each tool context fingerprint with its number of cases
- The number of cases for each label
- The number and the share of cases that skip a person
- The number and the rate of false approvals, with the exact one-sided 95% upper bound
- Each false approval with its case identifier, the rule name, and the answers
- One line for each case with its identifier, its label, the outcome, the rule name if one applies, and the answers
- The number and the share of `automatic_ok` cases that wait for a person
- For each question and each expected answer of the cases: the number of cases, and the number of these cases with the expected answer of the model (FR-054)
- The number of assessments for each status that is not `complete`
- The number of cases that used a default for a `context` value or a `request` value.

The model gave the expected answer of a `predicate` question if the probability is 0.5 or more for
`yes`, or less than 0.5 for `no`. It gave the expected answer of a `score` question if the expected
level has the highest probability. A refused, missing, or invalid answer is not the expected
answer.

A false approval is a case with the label `human` that satisfies the conditions of FR-041. The
run ignores two of these conditions: the mode and the evaluation record. The example is in [spec.md](../spec.md),
Journey 4, step 3.
