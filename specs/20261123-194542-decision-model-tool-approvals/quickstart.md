# Tool Call Assessment: Validation Guide

This guide describes checks after implementation. It does not describe current capabilities. See
the [plan](plan.md), the [data model](data-model.md), and the [contracts](contracts/).

## Prerequisites

- The Go version of the manifest, `just`, and the existing test tools.
- Docker or Podman for the agentgateway cases. The agentgateway cases are in the ExtProc suite and use agentgateway `v1.7.0` (D-14).
- No account at a model provider. All automated checks use test doubles for the decision endpoint.

The feature changes no page and no database schema. Thus, the checks need no browser and no
PostgreSQL instance.

The verification of D-02, D-03, D-07, D-14, and D-15 is complete. The section "Verification record" in
[research.md](research.md) gives the results. Two gates stay before behavior changes: the
approved contracts and the Accepted ADR. The first implementation task changes the pinned
agentgateway release to `v1.7.0` and runs the verification cases again.

## Commands

While you implement, run the related package tests. After each Go or test change, run
`just check` before the tests.

```sh
just check
just test
just test-e2e-extproc
just test-e2e-docker
```

Before the pull request, run `just verify`, as `CONTRIBUTING.md` requires. Record the real
results and each prerequisite that was not available. Do not record a result that you did not
see.

## Journeys

Each journey uses the values of the journeys in [spec.md](spec.md).

1. **Guidance (Journey 1).** Start ExtProc in the `guidance` mode with the example configuration and the example policy. The configuration has the organization values, and it sets no questions. Send a `tools/list` exchange for `google-drive-mcp`. The example policy allows the session setup and the listing. Send the external editor share. Make sure that the decision endpoint double received one request with the four context sections and the five default questions. Make sure that the request does not contain an identity. Make sure that the `call` section of the request has the tool name and the four arguments as the agent sent them. Make sure that the create request at the broker double has the risk level `critical` and the description "Share a Drive file with a party outside the organization". Make sure that the create request has no field that the request of today does not have.
2. **No model request for other calls.** Send a call that the policy allows, a call that the policy denies, and a call with an existing permanent denial. Make sure that the double received no request.
3. **Failures (Journey 3).** Set the double to each failure: a late answer, a refusal, a missing answer, an invalid probability, an HTTP error, and a closed connection. For each failure, make sure that a pending approval exists. Make sure that the create request has the description "Share a Drive file" and the risk level `medium`.
4. **Second identical call.** Send the external editor share two times before the approval expires. Make sure that the double received one request. Make sure that the two responses contain the same approval URL.
5. **Automatic approval (Journey 2).** Start ExtProc in the `auto_approve` mode. Send the internal reader share. Make sure that the tool double received the call and the broker double received no create request. Make sure that the record "opa authorization decision" exists two times for the call, the second time with the action `allow` and the answers. Make sure that it has no argument value.
6. **Stops of automatic approval.** Do step 5 again in the `guidance` mode. Do it again with a refusal for one question. Do it again with arguments that make the context larger than the limit. For each change, make sure that the call waits for a person.
7. **Disabled and shadow.** Set `authorization.assessment.mode` to `disabled`. Send the external editor share. Make sure that the double received no request. Set the mode to `shadow`, and send a share with a different file. Make sure that the double received one request and that the create request is the same as in the mode `disabled`. Make sure that the second record "opa authorization decision" has the answers. Make sure that the create request is the same as before this feature.
8. **Gateway path.** In the ExtProc suite with agentgateway `v1.7.0`, send the `tools/list` exchange and the share through agentgateway. Make sure that ExtProc learned the definition and that the client received the bytes that ExtProc returned. Make sure that the Decisions request with two `input_text` parts gets to the OpenAI-format double and to the System One-format double.

## Evidence and limits

- Map each of the 20 scenario identifiers to one `It`, as the table in [plan.md](plan.md) shows.
- The signal test is in its own file. It is not an acceptance scenario.
- Copy the verification record of [research.md](research.md) into the ADR. The cases in `tests/e2e/extproc/assessment_gateway_verification_test.go`, `tests/e2e/extproc/assessment_decision_endpoint_verification_test.go`, and `tests/e2e/approval_api_test.go` stay in the suites.
- The automated checks prove the behavior of the system around the model. They do not measure a model, a question, or a policy. That measurement is out of scope (D-18). An operator can analyse the decision logs of the policy engine outside the product.
- The checks do not prove that a tool description from a server is true. This risk stays, and the documentation must state it (SR-010).
- The checks do not prove that a description agrees with the call. A wrong answer can give a wrong description. The approver must read the technical details (SR-010).
