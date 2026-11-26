# Decision Model Research: Practices, Pitfalls, and Consequences for This Feature

**Date**: 2026-11-23

**Feature**: [spec.md](spec.md)

**Purpose**: This document records published evidence and guidance about decision models.
Each finding gives its consequence for the specification. The specification refers to findings `R-01` to `R-20`.

**Limits of this research**: On 2026-11-24, we read the sources listed under Sources in full unless the entry says otherwise.
We used the full arXiv articles, not their abstracts alone. The preprint results apply to the models and data that each study evaluated.
The Learn Jev URL redirects to a parked domain. We did not read its guide. Its advice remains unverified.

## Model and contract names

The [OpenAI Decisions guide](https://developers.openai.com/api/docs/guides/decisions) says:

> Send requests to `POST /v1/decisions` using `gpt-6-luna`, the only currently supported model.

The [Create a decision reference](https://developers.openai.com/api/reference/resources/decisions/methods/create) says: “The model to use. Only `gpt-6-luna` is supported.”
Neither source lists `gpt-6-luna-decision`.

The TypeSafe [Models](https://docs.typesafe.ai/models#aliases) page lists `jev-1.14.0` and `jev-1.13.0` as versioned identifiers.
Both `jev-latest` and `jev-preview` point to `jev-1.14.0` on the review date. The specification uses `gpt-6-luna` and `jev-1.14.0` as examples.

The agentgateway article describes nightly builds. Version 1.7.0 first shipped native support for Decisions and System One. See [research.md D-03](research.md#d-03--the-decision-request-is-plain-json-in-the-decisions-shape-with-the-context-as-text) for the deployed contract.

| Aspect | OpenAI Decisions | System One (Jev) | Through agentgateway |
|---|---|---|---|
| Endpoint | `POST /v1/decisions` | `POST /v1/systemone` | `POST /v1/decisions` for both providers in v1.7.0 |
| Shared evidence | `input`: string or ordered items. User messages accept `input_text`, `input_image`, and `input_json` with a JSON object or array of objects in `data`. | Required `state`: string, JSON object, or JSON array. Use `assets` for images. | See [research.md D-03](research.md#d-03--the-decision-request-is-plain-json-in-the-decisions-shape-with-the-context-as-text). |
| Questions | Array. Each answer includes the question `name` or `null`. | Object keyed by question ID. The API maps answers to those IDs. Jev does not see the IDs. | OpenAI shape. |
| Yes/no | A `predicate` answer contains `probability`. | A `noul` answer contains `noul`, the probability of yes, and no separate `confidence`. Optional `criteria` has `true` and `false` descriptions. Each can be a string, object, array, or `null`. | `predicate` |
| One of a set | `choice` with string or Boolean `choices[].value` and optional `description`. | `choice` with a `criteria` object. Each key names an option. Its value is a string, object, array, or `null`. | `choice` |
| Ordered scale | `score` with 2 to 10 `levels[]`. Each entry has a `label` and an optional `description`. | `score` with an ordered `criteria` array of strings, objects, or arrays. The API schema permits 1 to 10 levels. TypeSafe recommends at least 2. | `score` |
| Scale answer | Numeric `score` (a probability-weighted mean) and `confidence`. Each `probabilities[]` entry contains `value`, `label`, and `probability`. | Numeric `score` and `confidence`. The `probabilities` and `legend` objects use zero-based level numbers as string keys. A `score` is the probability-weighted mean. TypeSafe computes `confidence` from the spread around the most probable level on a scale from 0 to 1. | OpenAI shape. |
| No answer | A question can return `refusal`. Other questions in the request can still receive answers. | The API schema lists `noul`, `choice`, `score`, and `box`. It has no per-question refusal answer. | An OpenAI upstream returns its `refusal` unchanged. If a System One upstream omits an answer, the Decisions response has no entry for that question (Docker test, D-03). The reverse direction, a System One client with an OpenAI upstream, returns an error for a `refusal` in v1.7.0 (`crates/llm/src/conversion/systemone.rs`). |
| Model identity | The response `model` names the model used. | The response `model` gives the versioned identifier that answered, even if the request uses an alias. | The request `model` names a model entry of the gateway, and the provider entry sets the upstream model. The client gets the upstream `model` value unchanged (Docker test with doubles, D-03). |

## Findings

### A. Role of the model

**R-01 — A model answer is an input to policy, not an authorization.** agentgateway says that policy can use an answer to challenge, review, or reject a request.
Policy must not grant authority beyond identity, delegation, and baseline policy. The application owns the threshold and the review path.
TypeSafe keeps control flow, deterministic rules, and side effects in code.
*Consequence*: Only policy authorizes the call. A model answer cannot override a denial or increase the authority of the caller.
*Support*: strong. We read the agentgateway Decisions article and TypeSafe's *How to build with TypeSafe* in full. Both put the policy in code.

**R-02 — Do not ask the model for an approval verdict.** StackOne describes a judge that picks the more probable of two labels, “block” and “allow.”
That rule implies a cutoff of 0.5 on the probability normalized over those two labels. It does not describe all generative judges.
A typed probability lets the operator set a threshold for the deployment.
*Consequence*: The specification treats a question that asks for an approval verdict as invalid configuration.
*Support*: strong. We read the StackOne article and the Brennenstuhl essay in full. Both separate narrow model answers from the application decision.

**R-03 — One model answer is not a sufficient control.** Reveneau recommends deterministic limits for high-stakes actions and human review if model answers disagree.
The sources do not establish that agreement between questions to one model makes an irreversible action safe.
*Consequence*: The example policy requires a deterministic eligibility rule and several distinct answers that satisfy their thresholds.
*Support*: weak. We read the Reveneau guide in full. The Learn Jev guide did not load, and the example policy defines which answers it requires.

### B. Question design

**R-04 — Ask about one observable condition per question.** TypeSafe calls question decomposition “probably the most important concept in this guide.”
OpenAI recommends observable criteria and separate questions for separate concerns. One probability for a compound question does not identify which condition applies.
*Consequence*: The specification's rules for authors require each question to examine one property of the call.
*Support*: strong. We read TypeSafe's *How to build with TypeSafe* and the OpenAI Decisions guide in full. Both recommend narrow questions.

**R-05 — Jev reads questions literally.** TypeSafe writes: “Jev answers the question you wrote, not the one you meant.”
TypeSafe warns about double negatives, multiple reasoning steps, and contradictions between `instructions` and `criteria`.
*Consequence*: Each question states the exact condition in positive form. A high value indicates the risk property the question describes. Each yes/no question defines both outcomes and their boundary cases.
*Support*: moderate. We read TypeSafe's *Model jaggedness* and *Noul* pages in full. Both give guidance for Jev question wording.

**R-06 — Option names and order can change answers.** OpenAI says that a change to a choice `value` can change the answer despite an unchanged description.
TypeSafe reports that Jev sometimes favors the first choice option. Li observed changes in 1.8% of Jev answers after reversing option order.
Sun and coauthors report that Jev's area under the receiver operating characteristic curve fell from 0.815 to 0.581.
They swapped the definitions assigned to yes and no without changing the question, state, or option names.
Li reports an accuracy range of 5 to 15 percentage points per task for both Jev and Laya across instruction variants and different case samples.
*Consequence*: The question name, wording, criteria, labels, order, and type form one version. Each edit makes a new version. Thresholds apply to one version.
*Support*: strong. We read the OpenAI Decisions guide and TypeSafe's *Model jaggedness* in full. We also read the Sun and Li preprints in full.

**R-07 — Match the question type to the property.** A yes/no question gives a probability, not a degree. An ordered scale gives a degree.
A choice distinguishes categories. OpenAI recommends an `other` choice for cases outside the listed categories.
*Consequence*: The specification uses yes/no questions and ordered scales. It includes “cannot be determined from the supplied context” in the highest-risk level.
*Support*: strong. We read the OpenAI Decisions guide and TypeSafe's *Noul* and *Score* pages in full. The highest-risk level is a project rule.

**R-08 — Questions in one request must not depend on other answers.** TypeSafe documents independent, parallel evaluation against the same `state`.
OpenAI recommends one request for independent questions and separate requests for dependent questions. OpenAI does not specify how it schedules questions internally.
*Consequence*: The specification permits at most one model request per assessed tool call. No question depends on another answer. Different questions do not guarantee independent errors.
*Support*: strong. We read TypeSafe's *State* page and the OpenAI Decisions guide in full. Both describe how to group independent questions.

**R-09 — Keep exact computations in code.** TypeSafe says that Jev does not reliably count, compare dates, or handle questions that require numeric precision.
TypeSafe also prefers meaningful text over numeric encodings. OpenAI recommends meaningful string values for Choice options instead of opaque identifiers such as UUIDs.
*Consequence*: The policy checks facts that code can compute. The policy can compare domains or count recipients. The model judges meaning.
*Support*: strong. We read TypeSafe's *Model jaggedness* and the OpenAI Decisions guide in full. Their guidance covers different parts of this finding.

### C. Context design

**R-10 — Supply the facts that the question needs.** TypeSafe writes: “Do not rely on knowledge stored in model weights when current information can come from your own knowledge base.”
A server name such as `google-drive-mcp` does not state what the tool does. Low confidence can reflect missing facts, but it does not reliably identify them.
*Consequence*: The broker includes the server's description of the tool and its arguments. The operator supplies an organization description. If required context is missing, the broker does not assess the call.
*Support*: moderate. We read TypeSafe's *How to build with TypeSafe* and *Confidence* in full. We also read *TypedBench* in full, which found confident answers on underspecified input.

**R-11 — Supply only relevant context.** TypeSafe says that unrelated content can reduce accuracy and make errors harder to diagnose.
This effect depends on the task. The *TypedBench* length test did not show an accuracy decrease for Jev on its ticket-priority task.
*Consequence*: The broker uses fixed context sections and a size limit. It excludes session history, agent messages, and unrelated tools. The broker does not send oversized context.
*Support*: moderate. We read TypeSafe's *Model jaggedness* and the *TypedBench* preprint in full. Neither establishes a universal link between input length and accuracy.

**R-12 — Use named fields in the context.** If a field reference removes ambiguity, TypeSafe recommends a path in backticks. TypeSafe recommends nested JSON.
Reveneau warns that a single string can mix application facts and untrusted text. Named fields do not enforce a trust boundary.
*Consequence*: The broker uses a structured document with named sections. Each question identifies the sections it examines.
*Support*: moderate. We read TypeSafe's *How to build with TypeSafe* and the third-party Reveneau guide in full.

**R-13 — Keep trusted facts apart from untrusted content.** agentgateway recommends keeping user input, retrieved documents, and tool results separate from trusted policy state.
OWASP recommends identifying tool results as untrusted data. Reveneau recommends fixed question text and application facts from sources that the application controls.
*Consequence*: The broker puts operator text in trusted sections and identifies the server as the author of its tool definition. The broker places call arguments in an untrusted section. The broker does not copy argument values or definition text into the questions.
*Support*: strong. We read the agentgateway Decisions article and OWASP cheat sheet in full. We read the third-party Reveneau guide in full for the fixed-question guidance.

**R-14 — Do not send the agent's account of the call.** In Anthropic's engineering report, the classifier reads user messages and executable tool calls from the transcript.
Anthropic removes assistant prose, tool-call descriptions, and tool results. The exclusions reduce exposure to agent rationalizations and hostile tool content.
The classifier variant with assistant prose and tool results performed worse on the overeagerness test set. The classifier can lose information about the source of an argument.
*Consequence*: The broker does not send an agent rationale, an agent call description, or tool results.
*Support*: moderate. We read Anthropic's *How we built Claude Code auto mode* in full. Its comparison describes Claude Code, not this broker.

**R-15 — Treat server-provided tool definitions as claims.** The MCP annotations article lists `readOnlyHint`, `destructiveHint`, `idempotentHint`, and `openWorldHint`. The article says these hints do not guarantee behavior.
Clients must treat annotations as untrusted unless they come from a trusted server. OWASP describes prompt injection in definitions and unauthorized changes after review.
OWASP recommends a cryptographic hash of the complete reviewed definition. OWASP also warns against automatic tool approvals.
*Consequence*: The broker uses the tool definition from the server as context (R-10), not as proof of behavior. The following controls are project rules.
*Support*: strong. We read the MCP tool specification, the MCP annotations article, and the OWASP cheat sheet in full. These sources do not endorse this automatic approval policy.

Four controls apply to the tool definition from a server:

- The system puts the definition in a separate section and does not send it to the broker.
- A policy rule must apply to the call.
- The evaluation record must include the tool context fingerprint, which covers the tool definition. If the definition changes, the evaluation record no longer matches.
- The example policy uses a question that checks whether the definition text addresses the reviewer.

A fingerprint does not detect false claims or a behavior change behind an unchanged definition.

### D. How to read answers

**R-16 — Set thresholds for each question and action.** OpenAI recommends labeled application examples and thresholds based on the cost of false positives and false negatives.
TypeSafe recommends a higher threshold for a riskier action and a separate outcome for the uncertain band.
TypeSafe gives 0.7 and 0.3 as starting values for Noul routing. It does not validate these values as security thresholds.
*Consequence*: The specification provides no default for automatic approval. This feature treats false approval as the more costly error. If a rule does not find enough evidence to approve, a person decides.
*Support*: strong. We read the OpenAI Decisions guide and TypeSafe's *Noul* and routing guides in full. They agree that the cost of error matters.

**R-17 — Use the score distribution for decisions about specific levels.** OpenAI and TypeSafe define `score` as a probability-weighted mean of level numbers.
This mean can hide a split between levels. For Jev, `confidence` reflects the spread around the most probable level, not the probability that the answer is correct.
For Jev, a split across adjacent levels reduces `confidence` less than a split across distant levels. TypeSafe recommends adding probabilities on the relevant side of a threshold.
*Consequence*: The broker gives policy the full distribution. Rules for automatic approval limit the probability for risky levels.
*Support*: strong. We read the OpenAI Decisions guide and TypeSafe's *Confidence* and *Routing by confidence* pages in full. The confidence formula describes Jev only.

**R-18 — A refusal, a missing answer, and an uncertain answer are different.** OpenAI defines a `refusal` answer with `type` and `name`.
Other questions in the same request can still receive answers. TypeSafe does not define a refusal answer in the System One API schema.
*Consequence*: Policy distinguishes answered, refused, missing, and invalid questions. A timeout is an assessment status. Only a complete assessment can support automatic approval.
*Support*: strong. We read OpenAI's *Create a decision* and the TypeSafe *System One* API reference in full. Both define the answer types.

### E. Evaluation and change

**R-19 — Calibration describes groups of answers, not individual answers.** TypeSafe states that calibration “does not guarantee that an individual answer is correct.”
The preprints report the following results. These results are not measurements of this feature:

- *TypedBench* found that the same model can have overconfident answers on some questions and underconfident answers on others.
- Wu and Lim report a mean confidence of 0.778 for incorrect Jev function selections on the Berkeley Function Calling Leaderboard.
- In Li's evaluation of Laya 0.3.21, 26% of answers with confidence of at least 0.9 were wrong. Li defines confidence as the largest answer probability.
- In Xu's test with optimized context, `jev-1.13.0` assigned at least 0.7 probability to a fixed wrong option in 229 of 508 initially correct decisions (45.1%).
- Li reports that held-out miss rates exceeded the 5% target in about half of random splits. Li recommends several hundred positive examples or a conservative margin.
- Brom reports that thresholds from point estimates often missed their accuracy targets. Bounds improved results under the study assumptions but did not always transfer to new data.

Tang and Zheng recommend separate data to select and evaluate thresholds. Evaluation on the selection data does not establish the miss rate on new data.
In a separate study, Anthropic's full two-stage pipeline missed 17% of 52 real overeager actions. It blocked 0.4% of benign actions in real traffic.
*Consequence*: Automatic approval needs labeled, held-out tool calls that represent production. The evaluation uses the production versions of the model, questions, and context. The evaluation reports an upper bound on the false-approval rate, not mean accuracy alone.
*Support*: strong. We read TypeSafe's *System One* and Anthropic's engineering report in full. We read the Sharma, Wu and Lim, Li, Xu, Brom, and Tang and Zheng preprints in full (weak support for individual measurements).

**R-20 — Version changes can change model answers.** TypeSafe says that `jev-latest` tracks stable releases and that the response identifies the version that answered.
If thresholds depend on a version, TypeSafe recommends a versioned identifier. Reveneau recommends rerunning a fixed adversarial set after changes to questions, thresholds, or models.
*Consequence*: The broker records the evaluated model identifier and fingerprints. If the evaluated combination no longer matches, the broker stops automatic approval. A person then decides.
*Support*: strong. We read TypeSafe's *Models* specification for alias behavior in full. We also read the third-party Reveneau guide in full for its regression-test advice.

## Worked examples

These examples use `drive.share_file` on `google-drive-mcp`, as in the Brennenstuhl essay and the specification.
The configuration, payloads, and answer values here illustrate this feature. The answer values are not measurements.

### E-01 — The organization values and one default question

The organization description defines “outside the organization” for these questions (R-10). The operator writes it once in the service configuration. The product supplies the question, and the same question set applies to each assessed call.

```yaml
organization:
  name: "Example Corp"
  description: "Employees of Example Corp. All other parties are outside, including partners, customers, and personal accounts."
  internal_domains: ["example.com"]

questions:
  - name: leaves_organization        # R-06: the name is an identifier; the meaning is in the text
    type: predicate                  # R-07: a yes/no property, so a probability of yes
    instructions: "Does the call in `call` give access or data to a party outside the organization in `organization`?"   # R-04: one condition; R-12: sections named
    yes_when: "A recipient, grantee, or destination is not in `organization.internal_domains`, is public, or cannot be identified."   # R-05: boundary cases
    no_when: "Every recipient, grantee, and destination is in `organization.internal_domains`, or the call has none."
```

### E-02 — Question wording: incorrect and correct

| Finding | Incorrect | Cause of the problem | Correct |
|---|---|---|---|
| R-02 | "Is this call safe to approve?" | It asks for an approval verdict instead of a property. | "Does the call in `call` give access or data to a party outside the organization?" |
| R-04 | "Does the call share data outside the organization or delete something?" | It has two conditions. A value of 0.6 does not show which condition is present. | Two questions: `leaves_organization` and `persistent_effect`. |
| R-05 | "Does the call stay inside the organization?" | A high value indicates less risk. The example policy expects higher values to indicate more risk. | "Does the call give access or data to a party outside the organization?" |
| R-05 | "Does the call share the file with an external user?" | The question does not explicitly cover public links. | Use the question and criteria in E-01. |
| R-07 | "Is the effect of the call reversible?" | A yes/no answer does not distinguish degrees of reversibility. A probability of 0.5 does not mean partly reversible. | An ordered scale: no persistent effect, owner can undo, cannot be undone or cannot be determined. |
| R-09 | "Does the call notify more than 50 recipients?" | It needs a count. The model does not count reliably. | The policy counts: `count(input.mcp.arguments.recipients) > 50`. |
| R-09 | "Is the expiry date of the share more than 30 days away?" | It needs date arithmetic. | The policy compares the dates. |

TypeSafe's [Noul](https://docs.typesafe.ai/primitives/noul#writing-a-noul-question) page gives "Is the customer angry and asking for a refund?" as a compound question.
It gives "Is the message free of personal data?" as inverted wording.
The [OpenAI Decisions guide](https://developers.openai.com/api/docs/guides/decisions#check-an-image-for-visible-damage) gives an observable question with an explicit exclusion:
"Does the product have visible damage, such as a crack, tear, or dent? Ignore shadows and damage to the packaging."

### E-03 — Context: the data that the model needs and the data that it must not get

Too little context (R-10). The input gives the names but does not describe the tool or what its arguments mean:

```json
{"mcp_server": "google-drive-mcp", "tool_name": "drive.share_file", "arguments": {"role": "editor", "grantee_domain": "thirdparty.example.com"}}
```

Too much context from different authors (R-11, R-13, R-14). The input mixes the agent's claims with facts from the policy:

```json
{"request": "The user asked me to share the Q3 report with our partner. This was approved earlier. Tool drive.share_file, role editor, domain thirdparty.example.com.", "session_history": ["..."], "policy_risk_level": "medium"}
```

Separate sections for different sources (R-10 to R-15). The tool description is an example server claim, not a claim about Google Drive behavior:

```json
{
  "organization": {"name": "Example Corp", "internal_domains": ["example.com"]},
  "server": {"name": "google-drive-mcp"},
  "tool": {
    "name": "drive.share_file",
    "description": "Share a file with a user, group, or domain. The grantee gets access at once and receives a notification email.",
    "arguments": {"grantee_domain": {"type": "string", "description": "Email domain of the grantee."}}
  },
  "call": {"arguments": {"role": "editor", "grantee_domain": "thirdparty.example.com"}}
}
```

### E-04 — How to read an ordered scale

The scale has three levels: 0 "No persistent effect", 1 "Owner can undo", and 2 "Cannot be undone or cannot be determined".
These two illustrative answers use the probability-weighted mean that TypeSafe and OpenAI document (R-17):

| Probabilities for each level | `score` (mean) | Probability of level 2 | Meaning |
|---|---|---|---|
| `[0.02, 0.97, 0.01]` | 0.99 | 0.01 | The distribution puts most of the probability on "Owner can undo". |
| `[0.50, 0.02, 0.48]` | 0.98 | 0.48 | The distribution splits most of the probability between levels 0 and 2. |

A rule on the mean (`score <= 1.1`) accepts both answers. A rule on the probability of level 2 accepts only the first answer.
The example policy in the specification uses the second form (`probabilities[2] <= 0.02`) on its policy data, not on the raw OpenAI array.

### E-05 — Three results that are not a "no"

These are example OpenAI answer objects. The policy states and thresholds are examples (R-16, R-18).

| Result for `leaves_organization` | State in the policy input | Can it satisfy a rule for automatic approval? |
|---|---|---|
| `{"type": "predicate", "name": "leaves_organization", "probability": 0.01}` | answered | Yes, if all other conditions are true |
| `{"type": "refusal", "name": "leaves_organization"}` | refused | No (R-18) |
| No entry with that name | missing | No |
| `{"type": "predicate", "name": "leaves_organization", "probability": 0.4}` | answered, between the thresholds | No. The value is in the uncertain band (R-16) |

## Additional observations

- **Consent errors.** Anthropic reports that its classifier recognized the danger in most misses that it examined. The classifier misread whether the user's consent covered that action. This feature asks about the effect, not the user's intent. Anthropic did not compare the accuracy of those two question types.
- **Manual review.** Anthropic's rollout article reports that Claude Code users approve 97% of permission prompts. Its engineering report gives 93% and does not explain the difference. In the rollout article's controlled study, paid testers blocked 143 of 1,053 substituted dangerous commands (13.6%). This study does not establish the accuracy of review in this feature. Automatic approval removes manual review for that call.
- **Constrained answers.** StackOne and TypeSafe report that untrusted text can change probabilities. Typed answers prevent the model from adding free-form instructions or a rationale. StackOne asks whether tool results address the agent. This feature adapts that idea to tool descriptions. The sources do not establish the cost or effectiveness of that question for this feature.
- **Latency and cost.** TypeSafe reports about 100 ms for most calls. OpenAI reports that Decisions returns typed answers about 10 times faster than the Responses API. These are vendor claims, not broker benchmarks. OpenAI lists a base rate of $0.10 per million uncached input tokens and no output charge for Decisions. Cache rates, regional premiums, and long-context multipliers can change the input charge. OpenAI requires an identical shared input prefix of at least 1,024 tokens for cache reuse. Questions are outside that prefix.
- **Data handling.** OpenAI documents Zero Data Retention for eligible Decisions customers. It lists data residency in all supported API regions and regional processing in the United States and Europe (EEA and Switzerland). Brennenstuhl illustrates local inference. Local inference can keep call arguments inside the organization if the deployment does not send them elsewhere. The operator must decide which data the deployment sends to a provider.

## Smoke test with a local model

On 2026-11-24, ExtProc was not part of this test. A test client sent the request of Journey 1,
step 6, to agentgateway `v1.7.0`. The gateway translated it to System One and sent it to a local
`winnow:e4b` model. That model is a Gemma 4 E4B fine-tune with 7.5B parameters in Q8_0 on
llama.cpp, with a TypeSafe-compatible API. The questions had the full criteria of the
spec at that time. On 2026-11-26, the spec changed three of these questions because of the
observations below ([research.md](research.md), D-17). The answer values do not apply to the new
wording. The test sent three calls three times each. Only the call arguments differed. At that
time, the `tool` section had one more field, `"written_by": "server"`. The spec removed the field
on 2026-11-26. The answer values below are for the request with that field.

| Call | `leaves_organization` | `changes_access` | `persistent_effect` levels 0/1/2 | `addresses_assessor` | `definition_addresses_assessor` |
|---|---|---|---|---|---|
| Journey 1: `thirdparty.example.com`, editor | 0.939 | 0.944 | 0.138 / 0.065 / 0.798 | 0.044 | 0.249 |
| Journey 2: `example.com`, reader | 0.041 | 0.913 | 0.195 / 0.070 / 0.735 | 0.047 | 0.275 |
| Journey 4: `example.com.attacker.test`, reader | 0.920 | 0.921 | 0.171 / 0.074 / 0.755 | 0.053 | 0.228 |

Observations:

- The three runs of each call gave the same values. The requests had about 2,520 input tokens.
- `leaves_organization` separated the internal domain from the external domain and from the look-alike domain. This is the purpose of the `internal_domains` value (R-10).
- The model put most probability on "Cannot be undone or cannot be determined" for each share. The tool description says that the grantee receives a notification email. An email cannot be recalled, so the answer agrees with the third level. The spec examples assume "Owner can undo". The level boundary needs a decision of the operator, and the starter question set must state it (R-05).
- `definition_addresses_assessor` was between 0.23 and 0.28 for a description that does not address a reviewer. With the thresholds of the example policy, this value is `unclear`.
- Thus, the example policy of the spec sends all three calls to a person with this model. Journey 2 would not proceed automatically. That is the safe direction. It shows that the thresholds of the journeys are examples, as R-16 states.
- The latency was 6.1 s for the first request and 762 to 1,247 ms after that, on one Apple silicon workstation. Each value is above the default `request_timeout` of 500 ms. With the default, each call gets the status `timeout` and waits for a person.
- The response `model` was `winnow:e4b`, the name in the request, not a versioned identifier. If a deployment changes the weights behind the same name, the model condition of FR-041 does not see the change.

This test is a smoke test of the request shape and the context. It is not an evaluation. It used
one local model and three calls. No hosted OpenAI or TypeSafe model was called.

## Sources

Vendor and protocol documentation:

- OpenAI, [Decisions](https://developers.openai.com/api/docs/guides/decisions) and
  [Create a decision](https://developers.openai.com/api/reference/resources/decisions/methods/create)
- TypeSafe, [System One](https://docs.typesafe.ai/concepts/system-one),
  [How to build with TypeSafe](https://docs.typesafe.ai/concepts/how-to-build-with-system-one),
  [State](https://docs.typesafe.ai/concepts/state), [Score](https://docs.typesafe.ai/primitives/score),
  [Noul](https://docs.typesafe.ai/primitives/noul), [Confidence](https://docs.typesafe.ai/confidence),
  [Routing by probability](https://docs.typesafe.ai/patterns/probability-routing),
  [Routing by confidence](https://docs.typesafe.ai/patterns/confidence-routing),
  [Model jaggedness](https://docs.typesafe.ai/model-jaggedness), [Models](https://docs.typesafe.ai/models),
  [System One (API reference)](https://docs.typesafe.ai/api/system-one)
- agentgateway, [Using OpenAI's Decisions API and Jev through agentgateway](https://agentgateway.dev/blog/2026-10-12-openai-decisions-api-jev-agentgateway/)
- agentgateway, [External processing (ExtProc)](https://agentgateway.dev/docs/standalone/latest/documentation/configuration/traffic-management/extproc/) and
  [Guardrail processors](https://agentgateway.dev/docs/standalone/main/documentation/mcp/guardrails/processors/)
- Model Context Protocol, [Tools](https://modelcontextprotocol.io/specification/2026-07-28/server/tools) and
  [Tool Annotations as Risk Vocabulary: What Hints Can and Can't Do](https://blog.modelcontextprotocol.io/posts/2026-03-16-tool-annotations/)
- OWASP, [MCP (Model Context Protocol) Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/MCP_Security_Cheat_Sheet.html)

Deployment reports and design essays:

- Anthropic, [How we built Claude Code auto mode: a safer way to skip permissions](https://www.anthropic.com/research/claude-code-auto-mode)
- Anthropic, [Auto mode is now the default in Claude Code for Pro, Max, Ultra, and Team plans](https://claude.com/resources/articles/auto-mode-default-in-claude-code)
- Jan Brennenstuhl, [System One for Tool Approvals](https://www.janbrennenstuhl.eu/system-one-approvals/)
- StackOne, [Building a System One Model for Prompt Injection](https://www.stackone.com/blog/system-one-model-prompt-injection/)

Preprints (without peer review):

- Sharma and coauthors, [TypedBench: A Benchmark for Calibration, Framing Sensitivity, and Cost in System One Decision Models](https://arxiv.org/html/2610.11392v1)
- Tang and Zheng, [Typed Decision Models: An Early Evidence Audit and Evaluation Checklist](https://arxiv.org/html/2609.32160v1)
- Li, [Fast Models, Slow Evidence: A Paired and Self-Audited Evaluation of System-1 Decision Models for LLM Agent Harnesses](https://arxiv.org/html/2610.02267v1)
- Brom, [Confidence Is Not a Promise: A Reference Audit of Typed Decision Models](https://arxiv.org/html/2610.36311v1)
- Sun and coauthors, [Type-Safe Is Not Error-Free: Typed Decision Models Follow the Option Name, Not the Definition Bound to It](https://arxiv.org/html/2609.26758v2)
- Wu and Lim, [REFLEX with Jev for Efficient Selective Control in LLM Agents](https://arxiv.org/html/2609.26532v1)
- Xu, [JevOut: Natural Context Can Flip Decision Models](https://arxiv.org/html/2609.30243v1)

Third-party guides:

- Reveneau, [Prompt injection and adversarial input: what Jev can and cannot catch](https://reveneau.com/guides/jev-in-production/prompt-injection-and-adversarial-input)
- Learn Jev, [Jev failure modes](https://learnjev.com/tutorials/failure-modes) (unverified title and content: parked domain)
