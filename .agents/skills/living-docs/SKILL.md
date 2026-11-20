---
name: living-docs
description: "Write, extend, or repair a living-documentation page under docs/specification/. Use when asked for living documentation, specification by example pages, or a capability page; when `just living-docs` reports stale, failed, unbound, or not-run examples; or when a change alters behavior that a page under docs/specification/ describes."
alwaysApply: false
---

# Living documentation

A page under `docs/specification/` describes one capability as it behaves today, in free prose with
concrete examples. `just living-docs` checks every example against the acceptance suite. You write
and maintain the prose. You never decide that an example is correct: only recorded evidence does.

## How the check works

- A test is bound to an example with an identifier `<page-slug>.<topic>.<case>`:
  - in the Ginkgo suites `tests/e2e/`, `tests/e2e/extproc/`, and `tests/e2e/frontend/`, add
    `example("<id>")` as a decorator of the `It`, next to any `Label(...)`;
  - in a plain Go test, such as those under `tests/integration/`, call
    `evidence.ForTest(t, "<id>")` first in the test (package
    `tools/livingdoc/evidence`). Call it in the function that makes the assertions and
    that skips when a tool is missing. A parent test does not count as skipped when its
    subtests skip, so a binding on such a parent reports a false pass.
- With `LIVING_DOCS_EVIDENCE_DIR` set, each bound test records its outcome to that directory. A
  scenario of the backend suite `tests/e2e/` also records every HTTP exchange that it makes through
  a `bootstrap.TestServer` method or through `helpers.HTTPClient()`. Requests made with another
  HTTP client are not recorded.
- A page cites an example in two ways. An inline marker in code font, `example:<id>`, says the
  preceding claim is proven by that test. A fenced `http` block with `example="<id>"` in its
  info string shows one request and its response.
- The documentation site renders a marker as a badge that links to the test, and an example
  block as a request and response card (`assets/docusaurus/src/remark/living-docs.mjs`). The badge
  shows the verdict from `test-results/living-docs/status.json` when `just living-docs` ran before
  the site build. Put a marker after the punctuation of the sentence it proves, not in parentheses.
- `tools/livingdoc` gives each citation a verdict:

| Verdict | Meaning | What to do |
|---|---|---|
| passed | A test with this example passed. A shown exchange equals a recorded one. | Nothing |
| stale | The shown exchange equals no recorded one. The report has a diff. | Decide which side is right. See "Repair". |
| failed | A test with this example failed. | Fix the product or the test. Do not edit the page to hide it. |
| unbound | No test carries this example. | Bind an existing test, or remove the claim. |
| not run | The test was filtered out or skipped, usually because it needs Docker, Helm, or a browser. | Run `just living-docs -strict -browser` where those are available. |

### What an exchange block compares

The request line, the headers that the block shows, the status code, and both bodies.

- JSON compares by value; key order and layout are free.
- Timestamps compare by shape. UUIDs compare by identity pattern: the same UUID must appear in
  the same places, but its value is free.
- An ellipsis character `…` in a line matches any text at that place. Use it for a value that
  changes on each run and is not a UUID or timestamp: a token, a code, a signature, a port.
  Example: `"access_token": "…"` or `Location: https://agent.example.com/callback?code=…&state=af0`.
  The rest of the line must still match.
- A JSON object with the member `"…": "…"` shows only some members; the others are not compared.
  Without that member, the object must match completely. Arrays always compare completely.
- A form body (`a=1&b=2`) compares by parameter. Write one parameter on each line, in any order.

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
&client_id=550e8400-e29b-41d4-a716-446655440000
&client_secret=…

HTTP/1.1 200 OK

{
  "access_token": "…",
  "token_type": "Bearer",
  "…": "…"
}
```

### Screens

A page can show a screen with a maintained screenshot from `tests/e2e/screenshots/`. The browser
suite produces these files. Use a relative image link, and put a marker for the scenario that
takes the screenshot next to it:

```markdown
![Consent screen with one mandatory permission set](../../tests/e2e/screenshots/consent_page_loaded.png)
```

### Check one page

`just living-docs` checks every page. To check one page while you write it, use your own
evidence directory so that parallel work does not collide:

```bash
EV=$(mktemp -d)
LIVING_DOCS_EVIDENCE_DIR=$EV ginkgo --succinct --procs=2 --label-filter='/^example:<page-slug>\./ && !docker' ./tests/e2e/
go run ./tools/livingdoc -evidence $EV docs/specification/<page-slug>.md
```

Use `./tests/e2e/extproc/` for the ExtProc suite. For plain Go tests, run
`LIVING_DOCS_EVIDENCE_DIR=$EV go test <package> -run '<TestName>'`. The recorded exchanges are the
JSON files in `$EV`.

## Authority of sources

| Source | Authority |
|---|---|
| Acceptance scenarios and recorded evidence | What the broker does |
| `specs/*/spec.md`, accepted ADRs | Intent, reasons, and words that stakeholders agreed |
| `ARCHITECTURE.md` glossary | Vocabulary. Use its terms and no synonyms. |
| `api/*/openapi.yaml` | Contract |

Specs are deltas. Read every spec that touches the capability in directory order; a later spec
overrides an earlier one. When sources disagree, report the disagreement to the user. Do not
average it away in prose.

## Write a page

1. Find the capability in `specs/capability-map.md`. It lists the specs to read and the parts of
   them that later specs replaced. If the capability is new, name it as a job of a person or
   system, not as an endpoint or a feature number, and add it to the map.
   Read the specs, ADRs, contracts, and scenarios for it.
2. Select key examples. A page needs the few examples that let a reader predict the rest. It is
   not a list of every scenario. Leave edge-case matrices in the tests.
3. For each example, find the one test that proves it and bind it. Do not write a new test only
   to feed a page; a missing scenario is a gap under Constitution Principle XIII and belongs in a
   spec. Do not change what a test does.
4. Record evidence as described in "Check one page" and read the recorded exchanges.
5. Write the page for a reader who knows the domain but not the code: cast, situation, action,
   observable result, and the reason for the rule. Give state that HTTP does not show, such as
   seeded data, in prose.
6. Copy exchanges from the evidence. Replace generated UUIDs and timestamps with stable synthetic
   values. Keep equal UUIDs equal. Show only headers that matter to the reader.
7. Put an inline marker after each claim that a scenario proves. A sentence without a marker or
   a block is commentary; do not state untested behavior as fact.
8. Check the page until every citation is passed or, where Docker, Helm, or a browser is
   missing, not run.

Follow `.agents/skills/simple-english/SKILL.md` for the prose. Use synthetic identities only.
Never put a credential in a page; the recorder redacts `Authorization` and cookie headers, but
bodies are recorded as sent.

## Repair

For a stale exchange, read the diff and find the change that caused it.

- The behavior changed on purpose (a spec or ADR says so): update the block and every sentence
  that depends on it.
- The behavior changed by accident: the page found a regression. Report it. Do not edit the page.
- Only the scenario changed and no longer performs the exchange: bind the example to the scenario
  that does, or restore the exchange.

When a pull request changes behavior that a page describes, update the page in the same pull
request. CI runs the check with Docker, Helm, and a browser as the job "Living
specification"; its job summary shows the report with the diff of each stale exchange.

## Review a page

List each factual sentence that has neither a marker nor an exchange block and that a reader
would take as a guarantee. Offer to bind it or to soften it. This review is advice; it does not
replace the check.
