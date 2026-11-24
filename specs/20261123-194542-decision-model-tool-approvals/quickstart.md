# Tool Call Assessment: Validation Guide

This guide describes checks after implementation. It does not describe current capabilities. See
the [plan](plan.md), the [data model](data-model.md), and the [contracts](contracts/).

## Prerequisites

- The Go and Node versions of the manifests, `just`, and the existing test tools.
- Playwright Chromium for the browser cases.
- Docker or Podman for the agentgateway cases and the PostgreSQL cases. The agentgateway cases are in the ExtProc suite and use agentgateway `v1.7.0` (D-14).
- No account at a model provider. All automated checks use test doubles for the decision endpoint.

The verification of D-02, D-03, and D-07 is complete. The section "Verification record" in
[research.md](research.md) gives the results. Two gates stay before behavior changes: the
approved contracts and the Accepted ADR. The first implementation task changes the pinned
agentgateway release to `v1.7.0` and runs the verification cases again.

## Commands

While you implement, run the related package tests. After each Go or test change, run
`just check` before the tests.

```sh
just check
just test
just web-test
just test-integration-infra
just test-e2e-extproc
just test-e2e-docker
just test-e2e-frontend
```

Before the pull request, run `just verify`, as `CONTRIBUTING.md` requires. Record the real
results and each prerequisite that was not available. Do not record a result that you did not
see.

## Journeys

Each journey uses the values of the journeys in [spec.md](spec.md).

1. **Recommendation (Journey 1).** Start ExtProc in the `recommend` mode with the example catalog and the example policy. Send a `tools/list` exchange for `google-drive-mcp`. Send the external editor share. Make sure that the decision endpoint double received one request with the four context sections. Make sure that the request does not contain the `file_id` value or an identity. Open the approval page. Make sure that the page shows `critical`, the attention text, and four findings.
2. **No model request for other calls.** Send a call that the policy allows, a call that the policy denies, and a call with an existing permanent denial. Make sure that the double received no request.
3. **Failures (Journey 3).** Set the double to each failure: a late answer, a refusal, a missing answer, an invalid probability, an HTTP error, and a closed connection. For each failure, make sure that a pending approval exists. Make sure that the page shows the "no assessment" text.
4. **Second identical call.** Send the external editor share two times before the approval expires. Make sure that the double received one request. Make sure that the two responses contain the same approval URL.
5. **Evaluation (Journey 4).** Start `assessment evaluate` with the starter case set and the tool listing file. Make sure that the report contains the share of calls that skip a person and the false approvals with the upper bound. Make sure that the run wrote an evaluation record. Make sure that the tool double and the broker double received no request.
6. **Automatic approval (Journey 2).** Start ExtProc in the `auto_approve` mode with the evaluation record of step 5. Send the internal reader share. Make sure that the tool double received the call and the broker double received no create request. Find the record `assessment.tool_call.assessed` with the rule name. Make sure that it contains no argument value.
7. **Stops of automatic approval.** Do step 6 again with a different `model` value in the response. Do it again after a new `tools/list` exchange with a changed description. Do it again with a tool that the evaluation record does not name. Do it again with an argument that the definition does not describe. For each change, make sure that the call waits for a person.
8. **Disabled feature.** Set `authorization.assessment.enabled` to `false`. Send the external editor share. Make sure that the double received no request. Make sure that the create request and the approval page are the same as before this feature.
9. **Gateway path.** In the ExtProc suite with agentgateway `v1.7.0`, send the `tools/list` exchange and the share through agentgateway. Make sure that ExtProc learned the definition and that the client received the bytes that ExtProc returned. Make sure that the Decisions request with two `input_text` parts gets to the OpenAI-format double and to the System One-format double.
10. **Storage.** Apply the new migration up, down, and up against PostgreSQL. Make sure that existing approvals stay valid and read as "no assessment information".

## Evidence and limits

- Map each of the 28 scenario identifiers to one `It`, as the table in [plan.md](plan.md) shows.
- Keep the screenshots of the three page states under `tests/e2e/screenshots/`.
- Copy the verification record of [research.md](research.md) into the ADR. The cases in `tests/e2e/extproc/assessment_gateway_verification_test.go`, `tests/e2e/extproc/assessment_decision_endpoint_verification_test.go`, and `tests/e2e/approval_api_test.go` stay in the suites.
- The automated checks prove the behavior of the system around the model. They do not measure a model. The false-approval rate of a real model comes only from an evaluation run of the operator with real cases.
- The checks do not prove that a tool description from a server is true. This risk stays, and the documentation must state it (SR-010).
