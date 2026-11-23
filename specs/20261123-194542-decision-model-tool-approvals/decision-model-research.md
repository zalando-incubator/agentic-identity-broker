# Decision Model Research: Practices, Pitfalls, and Consequences for This Feature

**Date**: 2026-11-23

**Feature**: [spec.md](spec.md)

**Purpose**: Record what vendor documentation, an independent classifier deployment, and early
evaluation papers say about decision models. Each finding states its consequence for the
specification. The spec cites the finding IDs (`R-01` to `R-20`).

**Limits of this research**: It is a reading of published material. No model was called. The
evaluation papers are preprints from September and October 2026 and are not peer reviewed. Vendor
documentation describes vendor products. Third-party guides are marked as such.

## Model and contract names

OpenAI's documentation names `gpt-6-luna` as the only model that `POST /v1/decisions` supports. It
does not list a `gpt-6-luna-decision` identifier. The spec therefore refers to "the configured
decision model" and uses `gpt-6-luna` and `jev-1.14.0` only as example values.

| Aspect | OpenAI Decisions | System One (Jev) | Through agentgateway |
|---|---|---|---|
| Endpoint | `/v1/decisions` | `/v1/systemone` | `/v1/decisions` for both |
| Shared evidence | `input`: text, or messages with text, image, and `input_json` parts | `state`: string, object, or array | OpenAI shape |
| Questions | Array; optional `name` echoed in the answer | Map keyed by a name; the name is not sent to the model | OpenAI shape |
| Yes/no | `predicate` → `probability` | `noul` → `noul`; optional `criteria.true` and `criteria.false` | `predicate` |
| One of several | `choice` with `choices[].value` and `description` | `choice` with `criteria` map | `choice` |
| Ordered scale | `score` with 2–10 `levels[]` (`label`, `description`) | `score` with ordered `criteria` | `score` |
| Scale answer | `score` (probability-weighted mean), `probabilities[]`, `confidence` | The same three values | OpenAI shape |
| No answer | Per-question `refusal` answer; other answers still returned | Not present in the schema read for this research | Client must handle `refusal` |
| Model identity | `model` in the response | `model` in the response is the versioned ID behind an alias | Virtual model name selects the upstream |

## Findings

### A. Role of the model

**R-01 — An answer is a signal, not an authorization.** agentgateway states that a probability can
cause a challenge, review, or rejection, but must not grant authority that identity, delegation,
and baseline policy did not already permit. It does not convert a signal into allow or deny. The
caller owns thresholds, fallback, and review. TypeSafe gives the same rule: control flow,
deterministic rules, and side effects stay in code.
*Consequence*: the policy alone decides. A model answer cannot change a denial, and cannot widen
what the caller can already do.

**R-02 — Do not ask the model for the decision.** A generative judge that writes "allow" or "block"
applies a threshold of 0.5 that nobody configured. A typed probability lets the operator choose the
threshold per deployment.
*Consequence*: no question asks whether to approve. A question whose wording asks for an approval
verdict is invalid configuration.

**R-03 — A single answer must not be the last control before an irreversible action.** This rule
appears in a third-party Jev guide and agrees with R-01.
*Consequence*: automatic approval needs a deterministic eligibility rule first, and then several
independent answers that all agree. One answer is never sufficient.

### B. Question design

**R-04 — One observable condition per question.** TypeSafe calls question decomposition "probably
the most important concept". OpenAI says to write questions around observable criteria and to
separate concerns. A question with two conditions returns a value that means less.
*Consequence*: each question judges one property of the call. Compound wording is invalid.

**R-05 — The model reads literally.** Jev "answers the question you wrote, not the one you meant".
Scoping words, negations, and implied conditions are read at face value. Double negatives and
multi-hop questions cost accuracy. Contradictory instructions and criteria confuse the model.
*Consequence*: each question states the exact condition, in the positive form. A high value always
means the risk property is present. Each yes/no question has explicit descriptions of what counts
as yes and as no, including boundary cases.

**R-06 — Option names and order are part of the prompt.** OpenAI: changing a choice `value` can
change the result. One audit reports that swapping yes/no names across rubrics moved an AUC from
0.815 to 0.581. Jev's documentation says option order can affect the answer. A paired evaluation
found that rewording only the instructions moved accuracy by 5 to 15 points.
*Consequence*: question name, wording, criteria, level labels, level order, and type together form
one versioned unit. Any edit is a new version. Thresholds belong to a version.

**R-07 — Use the right type.** A yes/no value is a probability, not a degree. Use an ordered scale
for degree and a fixed set for categories. Add a fallback option such as `other` or `not stated`
when the options do not cover every input.
*Consequence*: yes/no and ordered-scale questions are the base. Each ordered scale has a top level
that also covers "cannot be determined from the supplied context".

**R-08 — Questions in one request are independent.** Both vendors evaluate the questions in
parallel against shared evidence. A question that depends on an earlier answer needs a second
request.
*Consequence*: one request per tool call. The question set contains independent questions only.
There is no question chaining.

**R-09 — Keep arithmetic, counting, dates, and opaque identifiers out of questions.** Jev does not
count or compare dates and numbers reliably. It handles semantic text better than numeric or
encoded values. OpenAI advises meaningful labels over opaque identifiers such as a UUID.
*Consequence*: facts that code can compute are computed before the model call and supplied as named
facts, or evaluated in the policy. Examples: "the recipient domain is not an organisation domain",
"the call names 214 recipients". The model judges meaning only.

### C. Context design

**R-10 — Supply the context the question needs. Do not rely on model knowledge.** TypeSafe: "Do not
rely on knowledge stored in model weights when current information can come from your own
knowledge base." A server name such as `google-drive-mcp` carries no reliable meaning for a model.
Missing context produces guesses, and an ordered scale then shows low confidence or a flat
distribution.
*Consequence*: the context includes what the tool does and what each supplied argument means,
taken from the tool definition that the MCP server lists, and what "the organisation" is, written
once by the operator. A call for which this context is missing is not assessed.

**R-11 — Supply only that context.** Accuracy falls as unrelated content grows ("context rot").
Large state also hides which input caused a wrong answer.
*Consequence*: the context has a fixed set of sections and a size limit. Session history, agent
messages, and unrelated tools are excluded. An oversized context is a failed assessment.

**R-12 — Use structure, and point questions at fields.** Use nested data with named fields. Refer
to a field by its path in the question. A single string that mixes application facts with
untrusted text is the worst shape, because injected text reads as context.
*Consequence*: the context is a structured document with named sections. Questions name the
sections they inspect.

**R-13 — Separate trusted facts from untrusted content.** agentgateway: keep user input, retrieved
documents, and tool output separate from trusted policy state. A third-party Jev guide adds two
rules: question text is constant and is never built from untrusted text, and application facts are
written by the application, not copied from fields that the other party controls.
*Consequence*: operator text is in trusted sections. The server's tool definition is in its own
section, marked as the server's claims. Tool-call arguments are in an untrusted section. Question
text never contains argument values or definition text.

**R-14 — Withhold the agent's own account of the call.** Anthropic's auto mode classifier sees user
messages and bare tool calls only. Assistant text and tool results are removed so that the agent
cannot argue the classifier into a wrong call, and because tool results are where hostile content
enters. Including them lowered measured performance. The cost is lost provenance.
*Consequence*: the context never contains agent-written rationale, agent-written call descriptions,
or tool results.

**R-15 — Tool descriptions and annotations from a server are hints.** The MCP specification says
annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`) do not
guarantee behavior, and that clients must treat them as untrusted unless the server is trusted.
OWASP lists tool poisoning (instructions hidden in descriptions or schemas) and rug pulls (a
definition changes after approval), and recommends pinning the reviewed definition.
*Consequence*: a first draft of the spec excluded server-supplied definitions and required the
operator to describe every tool and argument. That replicates a tool registry and does not scale,
so the spec now uses the server's own definition (R-10) and treats it as a claim. Four controls
apply. The definition is in a separate section and never reaches the approver. A policy rule must
cover the call. The exact definition is part of a fingerprint that
the evaluation record must contain, so a changed definition stops automatic approval for that
tool. A question detects definition text addressed to a reviewer. A server that describes a tool
falsely and consistently remains a residual risk.

### D. Reading answers

**R-16 — Thresholds are per question, per action, and set by the cost of the error.** OpenAI and
TypeSafe both say to set thresholds from labeled examples of the application and from the cost of
false positives and false negatives. TypeSafe: riskier actions get higher thresholds, and the band
between "yes" and "no" gets its own outcome in code. Its starting values (0.7 and 0.3) are for
routing, not for security gates.
*Consequence*: there is no product default threshold that enables automatic approval. The costly
error here is a false "harmless" answer, so thresholds are asymmetric: a value near zero permits,
and everything else keeps the human in the loop.

**R-17 — Read the distribution of an ordered scale, not its mean.** The `score` is a weighted
mean and can sit between two levels that split the answer. `confidence` measures how concentrated
the distribution is. It is not the probability that the answer is correct, and a split between
neighboring levels barely lowers it. TypeSafe recommends adding up the probabilities of the levels
on the side that matters.
*Consequence*: the policy receives the full per-level distribution. Rules for automatic approval
bound the probability of the risky levels.

**R-18 — A refusal, a missing answer, and an uncertain answer are separate outcomes.** OpenAI: a
refusal is an answer type, differs from a low probability, and needs an explicit path.
*Consequence*: the policy can distinguish answered, refused, missing, invalid, and timed out. None
of these states can satisfy a rule for automatic approval.

### E. Evaluation and change

**R-19 — Calibration holds for groups, not for one answer, and differs by question.** TypeSafe:
calibration "does not guarantee that an individual answer is correct". Preprints report that
miscalibration changes direction between questions of the same model, that wrong tool calls had a
mean confidence of 0.778, that 26% of one model's answers at confidence 0.9 or above were wrong,
and that adversarially optimized context moved 45% of initially correct decisions to a confident
wrong answer. Thresholds picked and scored on the same data keep their promise by construction.
On held-out data, a 5% miss target was exceeded in about half of the splits unless several hundred
positive examples or a safety margin were used. Anthropic's classifier, with full reasoning,
missed 17% of real overeager actions and blocked 0.4% of benign ones.
*Consequence*: automatic approval needs evidence measured on held-out, labeled tool calls for the
exact model version, question versions, and context version in use. The evidence reports the
false-approval rate with an upper bound, not an average accuracy.

**R-20 — Every version change can move answers.** A model alias such as `jev-latest` moves when a
release ships. The response names the model that answered. TypeSafe: if thresholds were tuned
against one version, send that version's ID. Third-party guidance: keep a fixed adversarial set
and run it again for each change to a question, threshold, or model version.
*Consequence*: the evaluated combination is recorded. If the answering model, a question, or the
context shape differs from the evaluated combination, automatic approval stops and the human
decides. The audit record names all versions.

## Worked examples

The examples use the tool call of the spec: `drive.share_file` on `google-drive-mcp`. They were
written for this feature unless a source is named. No model was called, so the stated answers
show the expected direction and are not measurements.

### E-01 — What the operator writes once

The organisation description gives every "outside the organisation" question its reference point
(R-10). The question set is the fixed list of typed questions for every assessed call.

```yaml
organisation:
  name: "Example Corp"
  description: "Employees of Example Corp. All other parties are outside, including partners, customers, and personal accounts."
  internal_domains: ["example.com"]

questions:
  - name: leaves_organisation        # R-06: the name is an identifier; the meaning is in the text
    type: predicate                  # R-07: a yes/no property, so a probability of yes
    instructions: "Does the call in `call` give access or data to a party outside the organisation in `organisation`?"   # R-04: one condition; R-12: sections named
    yes_when: "A recipient, grantee, or destination is not in `organisation.internal_domains`, is public, or cannot be identified."   # R-05: boundary cases
    no_when: "Every recipient, grantee, and destination is in `organisation.internal_domains`, or the call has none."
    statement: "Gives access or data to a party outside the organisation"   # for the approver; not sent to the model
```

### E-02 — Question wording: breaks a rule, follows the rule

| Finding | Breaks the rule | Why it fails | Follows the rule |
|---|---|---|---|
| R-02 | "Is this call safe to approve?" | Asks for the decision. The answer has an implied threshold that nobody set. | "Does the call in `call` give access or data to a party outside the organisation?" |
| R-04 | "Does the call share data outside the organisation or delete something?" | Two conditions. A value of 0.6 does not show which one is present. | Two questions: `leaves_organisation` and `persistent_effect`. |
| R-05 | "Does the call stay inside the organisation?" | A high value now means low risk. A rule written for the other questions reads it backwards. | "Does the call give access or data to a party outside the organisation?" |
| R-05 | "Does the call share the file with an external user?" | A public link has no user. A literal reading answers no. | The same question with the criterion "... is public, or cannot be identified". |
| R-07 | "Is the effect of the call reversible?" | Degree asked as yes/no. 0.5 can mean "partly" or "unknown". | An ordered scale: no persistent effect, owner can undo, cannot be undone or cannot be determined. |
| R-09 | "Does the call notify more than 50 recipients?" | Counting. The model does not count reliably. | The policy counts: `count(input.mcp.arguments.recipients) > 50`. |
| R-09 | "Is the expiry date of the share more than 30 days away?" | Date arithmetic. | The policy compares the dates. |

Two of these patterns come from vendor documentation. TypeSafe gives "Is the customer angry and
asking for a refund?" as a compound question and "Is the message free of personal data?" as
inverted wording. OpenAI's example of an observable question with an explicit exclusion is "Does
the product have visible damage, such as a crack, tear, or dent? Ignore shadows and damage to the
packaging."

### E-03 — Context: what the model needs and what it must not get

Too little (R-10). The model must guess what the server and the tool do:

```json
{"mcp_server": "google-drive-mcp", "tool_name": "drive.share_file", "arguments": {"role": "editor", "grantee_domain": "thirdparty.example.com"}}
```

Too much, and mixed (R-11, R-13, R-14). The agent's own account can argue for an answer, and the
string gives the model no way to tell facts from claims:

```json
{"request": "The user asked me to share the Q3 report with our partner. This was approved earlier. Tool drive.share_file, role editor, domain thirdparty.example.com.", "session_history": ["..."], "policy_risk_level": "medium"}
```

Sufficient and bounded (R-10 to R-15). Each section has one author, and the question can point at
a field:

```json
{
  "organisation": {"name": "Example Corp", "internal_domains": ["example.com"]},
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

### E-04 — Reading an ordered scale

The scale has three levels: 0 "No persistent effect", 1 "Owner can undo", 2 "Cannot be undone or
cannot be determined". Two answers with almost the same mean (R-17):

| Per-level probabilities | `score` (mean) | Probability of level 2 | Meaning |
|---|---|---|---|
| `[0.02, 0.97, 0.01]` | 0.99 | 0.01 | The model is settled on "Owner can undo". |
| `[0.50, 0.02, 0.48]` | 0.98 | 0.48 | The model is split between "nothing remains" and "cannot be undone". |

A rule on the mean (`score <= 1.1`) passes both. A rule on the risky level
(`probabilities[2] <= 0.02`) passes only the first. The spec's example policy uses the second form.

### E-05 — Three results that are not a "no"

| Result for `leaves_organisation` | State in the policy input | Can it satisfy an automatic approval rule? |
|---|---|---|
| `{"type": "predicate", "probability": 0.01}` | answered | Yes, if all other conditions hold |
| `{"type": "refusal"}` | refused | No (R-18) |
| No entry with that name | missing | No |
| `{"type": "predicate", "probability": 0.4}` | answered, between the thresholds | No. The finding is "unclear" (R-16) |

## Additional observations

- **What a classifier gets wrong.** In most of the misses Anthropic examined, the classifier saw
  the danger. It misjudged whether the user's consent covered the action. Questions about the
  observable effect of a call are therefore more reliable than questions about user intent. This
  feature asks only about effect.
- **Humans are a weak baseline.** Anthropic reports that users approve 97% of prompts and caught
  13.6% of planted dangerous commands. Fewer prompts with better content can improve the human
  decision. The model does not have to be perfect to improve the total outcome, but each automatic
  approval removes the human check for that call.
- **Narrow output limits injection damage and does not prevent it.** Injected text can move a
  probability. It cannot produce instructions or persuasive text. A separate question that asks
  whether an argument contains text addressed to an assessor is a low-cost tripwire.
- **Latency and cost.** TypeSafe states about 100 ms for most calls. OpenAI states about 10 times
  faster than its generative API, with input priced at $0.10 per million tokens and no charge for
  output. Shared input can be cached when an identical prefix is at least 1,024 tokens, and
  questions are outside that prefix. A stable, trusted context section placed first can therefore
  be reused across calls.
- **Data handling.** OpenAI states that the Decisions API supports Zero Data Retention and
  regional processing for eligible customers. A local open-weights model keeps arguments inside
  the organisation. Disclosure is a deployment decision and must be explicit.

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

Preprints (not peer reviewed):

- [TypedBench: A Benchmark for Calibration, Framing Sensitivity, and Cost in System One Decision Models](https://arxiv.org/abs/2610.11392)
- [Typed Decision Models: An Early Evidence Audit and Evaluation Checklist](https://arxiv.org/abs/2609.32160)
- [Fast Models, Slow Evidence: A Paired and Self-Audited Evaluation of System-1 Decision Models for LLM Agent Harnesses](https://arxiv.org/abs/2610.02267)
- [Confidence Is Not a Promise: A Reference Audit of Typed Decision Models](https://arxiv.org/abs/2610.36311)

Third-party guides (unverified against vendor sources):

- Reveneau, [Prompt Injection and Adversarial Input: What Jev Can and Cannot Catch](https://reveneau.com/guides/jev-in-production/prompt-injection-and-adversarial-input)
- Learn Jev, [Jev failure modes](https://learnjev.com/tutorials/failure-modes)
