# Decision Model Research: Practices, Pitfalls, and Consequences for This Feature

**Date**: 2026-11-23

**Feature**: [spec.md](spec.md)

**Purpose**: This document records the published guidance about decision models. The sources are
vendor documentation, one report about a classifier in production, and early evaluation papers.
Each finding gives its consequence for the specification. The spec refers to the finding
identifiers (`R-01` to `R-20`).

**Limits of this research**: The research used only published material. It did not call a model.
The evaluation papers are preprints from September and October 2026 without peer review. Vendor
documentation describes the products of the vendor. The source list marks third-party guides.

## Model and contract names

The OpenAI documentation names `gpt-6-luna` as the only model for `POST /v1/decisions`. It does
not list a `gpt-6-luna-decision` identifier. Thus, the spec uses the term "the configured decision
model". It uses `gpt-6-luna` and `jev-1.14.0` only as example values.

| Aspect | OpenAI Decisions | System One (Jev) | Through agentgateway |
|---|---|---|---|
| Endpoint | `/v1/decisions` | `/v1/systemone` | `/v1/decisions` for the two models |
| Shared evidence | `input`: text, or messages with text, image, and `input_json` parts | `state`: string, object, or array | OpenAI shape |
| Questions | Array. The answer repeats the optional `name`. | Map with a name as the key. The model does not receive the name. | OpenAI shape |
| Yes/no | `predicate` → `probability` | `noul` → `noul`. Optional `criteria.true` and `criteria.false`. | `predicate` |
| One of a set | `choice` with `choices[].value` and `description` | `choice` with a `criteria` map | `choice` |
| Ordered scale | `score` with 2 to 10 `levels[]` (`label`, `description`) | `score` with ordered `criteria` | `score` |
| Scale answer | `score` (probability-weighted mean), `probabilities[]`, `confidence` | The same three values | OpenAI shape |
| No answer | A `refusal` answer for one question. The other answers stay. | Not in the schema that this research read | The client must process `refusal` |
| Model identity | `model` in the response | `model` in the response is the versioned identifier behind an alias | The virtual model name selects the upstream model |

## Findings

### A. Role of the model

**R-01 — An answer is a signal, not an authorization.** agentgateway states that a probability
can cause a challenge, a review, or a rejection. It must not grant an authority that identity,
delegation, and baseline policy did not permit. agentgateway does not change a signal into allow
or deny. The caller owns the thresholds, the fallback, and the review. TypeSafe gives the same
rule: control flow, deterministic rules, and side effects stay in code.
*Consequence*: Only the policy decides. A model answer cannot change a denial. It cannot increase
the authority of the caller.

**R-02 — Do not ask the model for the decision.** A generative judge that writes "allow" or
"block" applies a threshold of 0.5. No person configured that threshold. A typed probability
lets the operator select the threshold for each deployment.
*Consequence*: No question asks whether to approve. A question that asks for an approval verdict
is invalid configuration.

**R-03 — One answer must not be the last control before an irreversible action.** This rule is in
a third-party Jev guide, and it agrees with R-01.
*Consequence*: Automatic approval first needs a deterministic eligibility rule. Then it needs
two or more independent answers that all agree. One answer is never sufficient.

### B. Question design

**R-04 — One observable condition for each question.** TypeSafe calls the decomposition of
questions "probably the most important concept". OpenAI tells writers to use observable criteria
and to keep concerns apart. If a question has two conditions, the returned value has less
meaning.
*Consequence*: Each question examines one property of the call. Compound wording is invalid.

**R-05 — The model reads literally.** Jev "answers the question you wrote, not the one you
meant". It reads scoping words, negations, and implied conditions as written. Double negatives
and questions with two or more reasoning steps decrease accuracy. Instructions and criteria that
contradict each other also decrease accuracy.
*Consequence*: Each question states the exact condition in the positive form. A high value always
shows that the risk property is present. Each yes/no question has explicit descriptions of yes
and of no, with the boundary cases.

**R-06 — Option names and option sequence are part of the prompt.** OpenAI says that a changed
choice `value` can change the result. One audit reports that swapped yes/no names moved an AUC
from 0.815 to 0.581. The Jev documentation says that the option sequence can change the answer. A
paired evaluation found that new wording of only the instructions moved accuracy by 5 to 15
points.
*Consequence*: The name, wording, criteria, level labels, level sequence, and type of a question
are one versioned unit. Each edit makes a new version. Thresholds belong to one version.

**R-07 — Use the correct type.** A yes/no value is a probability, not a degree. An ordered scale
is correct for a degree, and a fixed set is correct for categories. If the options do not include
each possible input, add an option such as `other` or `not stated`.
*Consequence*: The basic types are yes/no questions and ordered-scale questions. The top level of
each ordered scale also includes "cannot be determined from the supplied context".

**R-08 — Questions in one request are independent.** The two vendors evaluate the questions in
parallel against shared evidence. A question that depends on an earlier answer needs a second
request.
*Consequence*: The system sends one request for each tool call. The question set contains only
independent questions. Questions do not depend on each other.

**R-09 — Keep arithmetic, counts, dates, and opaque identifiers out of questions.** Jev does not
count reliably. It does not compare dates or numbers reliably. It processes semantic text better
than numeric or encoded values. OpenAI recommends meaningful labels instead of opaque identifiers
such as a UUID.
*Consequence*: If code can compute a fact, the policy evaluates that fact. Examples are "the
recipient domain is not an organization domain" and "the call names 214 recipients". The model
judges only meaning.

### C. Context design

**R-10 — Supply the context that the question needs. Do not rely on model knowledge.** TypeSafe:
"Do not rely on knowledge stored in model weights when current information can come from your
own knowledge base." A server name such as `google-drive-mcp` has no reliable meaning for a
model. If context is missing, the model guesses. An ordered scale then shows low confidence or a
flat distribution.
*Consequence*: The context includes the function of the tool and the meaning of each supplied
argument. This information comes from the tool definition that the MCP server lists. The context
also includes a description of "the organization", which the operator writes once. If this
context is missing for a call, the system does not assess the call.

**R-11 — Supply only that context.** Accuracy decreases as unrelated content increases ("context
rot"). A large state also hides the input that caused a wrong answer.
*Consequence*: The context has a fixed set of sections and a size limit. It excludes session
history, agent messages, and unrelated tools. An oversized context is a failed assessment.

**R-12 — Use structure, and refer to fields in the questions.** Use nested data with named
fields. Refer to a field by its path in the question. One string that mixes application facts
with untrusted text is the worst shape, because the model reads injected text as context.
*Consequence*: The context is a structured document with named sections. Each question names the
sections that it examines.

**R-13 — Keep trusted facts apart from untrusted content.** agentgateway says to keep user input,
retrieved documents, and tool output apart from trusted policy state. A third-party Jev guide
adds two rules. First, question text is constant, and code never makes it from untrusted text.
Second, the application writes its own facts and does not copy them from fields that a different
party controls.
*Consequence*: Operator text is in trusted sections. The tool definition of the server is in its
own section, with the server as the named author. Tool-call arguments are in an untrusted
section. Question text never contains argument values or definition text.

**R-14 — Do not send the account that the agent gives of the call.** The auto mode classifier of
Anthropic sees only user messages and bare tool calls. Anthropic removes assistant text. Thus,
the agent cannot argue the classifier into a wrong decision. Anthropic also removes tool results,
because hostile content enters through them. With this content, the measured performance
decreased. The cost is that the classifier does not know the origin of a value.
*Consequence*: The context never contains a rationale from the agent, a call description from the
agent, or tool results.

**R-15 — Tool descriptions and annotations from a server are hints.** The MCP specification says
that annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) do not
guarantee behavior. It says that clients must not trust them unless the server is trusted. OWASP
lists two attacks. Tool poisoning hides instructions in descriptions or schemas. In a rug pull, a
definition changes after approval. OWASP recommends that clients pin the reviewed definition.
*Consequence*: The first draft of the spec excluded definitions from servers. The operator had to
describe each tool and argument. That design copies a tool registry and is not practical for
hundreds of servers. The spec uses the definition of the server (R-10) and reads it as a claim.

Four controls apply to the definition of a server:

- The definition is in a separate section, and the approver never sees it.
- A policy rule must apply to the call.
- The evaluation record must contain a fingerprint of the exact definition. A changed definition stops automatic approval for that tool.
- One question finds definition text that addresses a reviewer.

A server that describes a tool falsely, with a description that does not change, stays a
residual risk.

### D. How to read answers

**R-16 — Thresholds apply to one question and one action, and the cost of the error sets them.**
OpenAI and TypeSafe say to set thresholds from labeled examples of the application. They say to
use the cost of false positives and false negatives. TypeSafe gives riskier actions higher
thresholds. It gives the band between "yes" and "no" its own outcome in code. Its start values
(0.7 and 0.3) are for routing, not for security gates.
*Consequence*: The product has no default threshold that enables automatic approval. The
expensive error is a false "harmless" answer. Thus, the thresholds are not symmetric. A value
near zero permits the call, and each other value keeps the person in the decision.

**R-17 — Read the distribution of an ordered scale, not its mean.** The `score` is a weighted
mean. It can be between two levels that divide the answer. `confidence` measures the
concentration of the distribution. It is not the probability that the answer is correct. A
division between two adjacent levels decreases it only a small quantity. TypeSafe recommends
adding the probabilities of the levels on the important side.
*Consequence*: The policy receives the full distribution for each level. Rules for automatic
approval set a limit for the probability of the risky levels.

**R-18 — A refusal, a missing answer, and an uncertain answer are different outcomes.** OpenAI
says that a refusal is an answer type. It is different from a low probability, and it needs an
explicit path.
*Consequence*: The policy can see these states: answered, refused, missing, invalid, and timed
out. Only an answered question can satisfy a rule for automatic approval.

### E. Evaluation and change

**R-19 — Calibration is true for groups, not for one answer, and it is different for each
question.** TypeSafe says that calibration "does not guarantee that an individual answer is
correct". The preprints report these results:

- The direction of miscalibration changes between questions of the same model.
- Wrong tool calls had a mean confidence of 0.778.
- For one model, 26% of the answers with a confidence of 0.9 or more were wrong.
- Adversarially optimized context moved 45% of initially correct decisions to a confident wrong answer.
- A 5% miss target failed on held-out data in approximately half of the splits. Several hundred positive examples or a safety margin prevented this.

If the same data selects and scores a threshold, the result is correct by construction and
proves nothing. The Anthropic classifier, with full reasoning, missed 17% of real overeager
actions and blocked 0.4% of benign actions.
*Consequence*: Automatic approval needs evidence from labeled tool calls on held-out data. The
evidence must use the exact model version, question versions, and context version of production.
The evidence reports the false-approval rate with an upper bound, not a mean accuracy.

**R-20 — Each version change can move the answers.** A model alias such as `jev-latest` moves
with each release. The response names the model that answered. If an operator tuned
thresholds against one version, TypeSafe says to send the identifier of that version. Third-party guidance says to keep a
fixed adversarial set. It says to run that set again for each change to a question, a threshold,
or a model version.
*Consequence*: The system records the evaluated combination. If the answering model, a question,
or the context shape is different from that combination, automatic approval stops. The person
then decides. The audit record names all versions.

## Worked examples

The examples use the tool call of the spec: `drive.share_file` on `google-drive-mcp`. This
research wrote the examples for this feature, unless the text names a source. The research called
no model. Thus, the stated answers show the expected direction and are not measurements.

### E-01 — The text that the operator writes once

The organization description is the reference for each "outside the organization" question
(R-10). The question set is the fixed list of typed questions for each assessed call.

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
    statement: "Gives access or data to a party outside the organization"   # for the approver; not sent to the model
```

### E-02 — Question wording: incorrect and correct

| Finding | Incorrect | Cause of the problem | Correct |
|---|---|---|---|
| R-02 | "Is this call safe to approve?" | It asks for the decision. The answer has an implied threshold that no person set. | "Does the call in `call` give access or data to a party outside the organization?" |
| R-04 | "Does the call share data outside the organization or delete something?" | It has two conditions. A value of 0.6 does not show which condition is present. | Two questions: `leaves_organization` and `persistent_effect`. |
| R-05 | "Does the call stay inside the organization?" | A high value shows low risk. A rule for the other questions reads it in the wrong direction. | "Does the call give access or data to a party outside the organization?" |
| R-05 | "Does the call share the file with an external user?" | A public link has no user. A literal reader answers no. | The same question with the criterion "... is public, or cannot be identified". |
| R-07 | "Is the effect of the call reversible?" | It asks for a degree as yes/no. A value of 0.5 can mean "partly" or "unknown". | An ordered scale: no persistent effect, owner can undo, cannot be undone or cannot be determined. |
| R-09 | "Does the call notify more than 50 recipients?" | It needs a count. The model does not count reliably. | The policy counts: `count(input.mcp.arguments.recipients) > 50`. |
| R-09 | "Is the expiry date of the share more than 30 days away?" | It needs date arithmetic. | The policy compares the dates. |

Two of these patterns come from vendor documentation. TypeSafe gives "Is the customer angry and
asking for a refund?" as a compound question. It gives "Is the message free of personal data?" as
inverted wording. The OpenAI example of an observable question with an explicit exclusion is
"Does the product have visible damage, such as a crack, tear, or dent? Ignore shadows and damage
to the packaging."

### E-03 — Context: the data that the model needs and the data that it must not get

Too little context (R-10). The model must guess the function of the server and of the tool:

```json
{"mcp_server": "google-drive-mcp", "tool_name": "drive.share_file", "arguments": {"role": "editor", "grantee_domain": "thirdparty.example.com"}}
```

Too much context, with mixed authors (R-11, R-13, R-14). The account from the agent can argue for
an answer. The string does not let the model tell facts from claims:

```json
{"request": "The user asked me to share the Q3 report with our partner. This was approved earlier. Tool drive.share_file, role editor, domain thirdparty.example.com.", "session_history": ["..."], "policy_risk_level": "medium"}
```

Sufficient and bounded context (R-10 to R-15). Each section has one author, and a question can
refer to a field:

```json
{
  "organization": {"name": "Example Corp", "internal_domains": ["example.com"]},
  "server": {"name": "google-drive-mcp"},
  "tool": {
    "written_by": "server",
    "name": "drive.share_file",
    "description": "Share a file with a user, group, or domain. The grantee gets access at once and receives a notification email.",
    "arguments": {"grantee_domain": {"type": "string", "description": "Email domain of the grantee."}}
  },
  "call": {"arguments": {"role": "editor", "grantee_domain": "thirdparty.example.com"}}
}
```

### E-04 — How to read an ordered scale

The scale has three levels: 0 "No persistent effect", 1 "Owner can undo", and 2 "Cannot be undone
or cannot be determined". These two answers have almost the same mean (R-17):

| Probabilities for each level | `score` (mean) | Probability of level 2 | Meaning |
|---|---|---|---|
| `[0.02, 0.97, 0.01]` | 0.99 | 0.01 | The model selects "Owner can undo" with high confidence. |
| `[0.50, 0.02, 0.48]` | 0.98 | 0.48 | The model cannot decide between "nothing stays" and "cannot be undone". |

A rule on the mean (`score <= 1.1`) accepts the two answers. A rule on the risky level
(`probabilities[2] <= 0.02`) accepts only the first answer. The example policy of the spec uses
the second form.

### E-05 — Three results that are not a "no"

| Result for `leaves_organization` | State in the policy input | Can it satisfy a rule for automatic approval? |
|---|---|---|
| `{"type": "predicate", "probability": 0.01}` | answered | Yes, if all other conditions are true |
| `{"type": "refusal"}` | refused | No (R-18) |
| No entry with that name | missing | No |
| `{"type": "predicate", "probability": 0.4}` | answered, between the thresholds | No. The finding is "unclear" (R-16) |

## Additional observations

- **The errors of a classifier.** In most of the misses that Anthropic examined, the classifier saw the danger. It made an error about whether the consent of the user applied to the action. Thus, questions about the observable effect of a call are more reliable than questions about user intent. This feature asks only about the effect.
- **Humans are a weak baseline.** Anthropic reports that users approve 97% of prompts. In its study, users found 13.6% of the dangerous commands that it put into prompts. Fewer prompts with better content can improve the human decision. It is not necessary for the model to be perfect to improve the total outcome. But each automatic approval removes the human check for that call.
- **Narrow output limits the damage of an injection and does not prevent the injection.** Injected text can move a probability. It cannot make instructions or persuasive text. A separate question about text that addresses an assessor is a low-cost detection method.
- **Latency and cost.** TypeSafe states approximately 100 ms for most calls. OpenAI states that the Decisions API is approximately 10 times faster than its generative API. OpenAI charges $0.10 for each million input tokens and nothing for output. If an identical prefix has a minimum of 1,024 tokens, the API can cache shared input. Questions are not in that prefix. Thus, subsequent calls can use a stable, trusted context section at the start again.
- **Data handling.** OpenAI states that the Decisions API supports Zero Data Retention and regional processing for eligible customers. A local open-weights model keeps arguments in the organization. Disclosure is a deployment decision, and it must be explicit.

## Sources

Vendor and protocol documentation:

- OpenAI, [Decisions guide](https://developers.openai.com/api/docs/guides/decisions) and
  [create decision reference](https://developers.openai.com/api/reference/resources/decisions/methods/create)
- TypeSafe, [System One](https://docs.typesafe.ai/concepts/system-one),
  [How to build with TypeSafe](https://docs.typesafe.ai/concepts/how-to-build-with-system-one),
  [Noul](https://docs.typesafe.ai/primitives/noul), [Confidence](https://docs.typesafe.ai/confidence),
  [Routing by probability](https://docs.typesafe.ai/patterns/probability-routing),
  [Routing by confidence](https://docs.typesafe.ai/patterns/confidence-routing),
  [Model jaggedness](https://docs.typesafe.ai/model-jaggedness), [Models](https://docs.typesafe.ai/models),
  [System One API](https://docs.typesafe.ai/api/system-one)
- agentgateway, [Using OpenAI's Decisions API and Jev through agentgateway](https://agentgateway.dev/blog/2026-10-12-openai-decisions-api-jev-agentgateway/)
- agentgateway, [External processing (ExtProc)](https://agentgateway.dev/docs/standalone/latest/documentation/configuration/traffic-management/extproc/) and
  [Guardrail processors](https://agentgateway.dev/docs/standalone/main/documentation/mcp/guardrails/processors/), for the visibility of `tools/list` results at the gateway
- Model Context Protocol, [Tool Annotations as Risk Vocabulary](https://blog.modelcontextprotocol.io/posts/2026-03-16-tool-annotations/)
- OWASP, [MCP Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/MCP_Security_Cheat_Sheet.html)

Deployment reports:

- Anthropic, [How we built Claude Code auto mode](https://www.anthropic.com/research/claude-code-auto-mode)
- Jan Brennenstuhl, [System One for Tool Approvals](https://www.janbrennenstuhl.eu/system-one-approvals/)
- StackOne, [Building a System One Model for Prompt Injection](https://www.stackone.com/blog/system-one-model-prompt-injection/)

Preprints (without peer review):

- [TypedBench: A Benchmark for Calibration, Framing Sensitivity, and Cost in System One Decision Models](https://arxiv.org/abs/2610.11392)
- [Typed Decision Models: An Early Evidence Audit and Evaluation Checklist](https://arxiv.org/abs/2609.32160)
- [Fast Models, Slow Evidence: A Paired and Self-Audited Evaluation of System-1 Decision Models for LLM Agent Harnesses](https://arxiv.org/abs/2610.02267)
- [Confidence Is Not a Promise: A Reference Audit of Typed Decision Models](https://arxiv.org/abs/2610.36311)

Third-party guides (not compared with vendor sources):

- Reveneau, [Prompt Injection and Adversarial Input: What Jev Can and Cannot Catch](https://reveneau.com/guides/jev-in-production/prompt-injection-and-adversarial-input)
- Learn Jev, [Jev failure modes](https://learnjev.com/tutorials/failure-modes)
