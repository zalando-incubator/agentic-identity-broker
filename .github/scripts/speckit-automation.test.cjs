const assert = require("node:assert/strict");
const fs = require("node:fs");
const { test } = require("node:test");
const {
  artifactHashes,
  parseResult,
  route,
  requireResult,
  prepare,
  verify,
} = require("./speckit-automation.cjs");

const repo = { owner: "example", repo: "broker" };
const spec = "specs/042-example";
const files = {
  [`${spec}/spec.md`]: "Feature requirements",
  [`${spec}/plan.md`]: "Technical plan",
  [`${spec}/tasks.md`]: "- [ ] T001 Implement feature",
};

function result(phase, overrides = {}) {
  return {
    phase, status: "complete", feature: spec, run: "100/1",
    files: artifactHashes(files), findings: [], questions: [],
    ...overrides,
  };
}

function comment(data, overrides = {}) {
  return {
    id: 11,
    issue_url: "https://api.github.com/repos/example/broker/issues/42",
    user: { login: "github-actions[bot]", type: "Bot" },
    body: `Report\n<!-- speckit-result -->\n\`\`\`json\n${JSON.stringify(data)}\n\`\`\`\n<!-- gh-aw-workflow-id: speckit-phase -->`,
    ...overrides,
  };
}

function fixture(eventName, payload, options = {}) {
  const calls = [];
  const pr = {
    number: 42, state: "open", draft: true,
    body: "<!-- speckit-issue: 7 -->",
    head: { ref: "speckit/issue-7", sha: "a".repeat(40), repo: { full_name: "example/broker" } },
    base: { ref: "main", repo: { full_name: "example/broker" } },
    ...options.pr,
  };
  const github = {
    rest: {
      repos: {
        getCollaboratorPermissionLevel: async (args) => {
          calls.push(args);
          return { data: { permission: options.permission || "write" } };
        },
        getContent: async ({ path }) => ({
          data: { type: "file", encoding: "base64", content: Buffer.from(
            path === ".specify/feature.json"
              ? JSON.stringify({ feature_directory: spec })
              : files[path],
          ).toString("base64") },
        }),
      },
      pulls: {
        get: async () => ({ data: pr }),
        list: async () => ({ data: options.existing || [] }),
      },
      issues: {
        listComments: async () => ({ data: options.comments || [] }),
        get: async () => ({ data: options.issue }),
        getComment: async ({ comment_id }) => ({ data: options.comments.find((entry) => entry.id === comment_id) }),
      },
    },
    paginate: async (method, args) => (await method(args)).data,
  };
  return {
    github, calls,
    context: { repo, eventName, payload, actor: "maintainer", runId: 100 },
  };
}

test("specify accepts the exact issue label and does not fetch comments", async () => {
  const f = fixture("issues", { action: "labeled", label: { name: "speckit:specify" }, issue: { number: 7, state: "open" } });
  assert.deepEqual(await route(f), { action: "specify", issue: 7, pr: 0, comment: 0 });
  assert.equal(f.calls.length, 1);
});

test("unrelated labels, read-only users, bots, and issue comments do not execute", async () => {
  const cases = [
    fixture("issues", { action: "labeled", label: { name: "other" }, issue: { number: 7 } }),
    fixture("issues", { action: "labeled", label: { name: "speckit:specify" }, issue: { number: 7 } }, { permission: "read" }),
    fixture("issue_comment", { action: "created", issue: { number: 7 }, comment: { user: { type: "User" } } }),
    fixture("issue_comment", { action: "created", issue: { number: 42, pull_request: {} }, comment: { user: { type: "Bot" } } }),
  ];
  for (const f of cases) assert.equal((await route(f)).action, "noop");
});

test("relabeling a specified issue does not open another PR", async () => {
  const f = fixture("issues", { action: "labeled", label: { name: "speckit:specify" }, issue: { number: 7, state: "open" } }, { existing: [{ number: 42 }] });
  assert.equal((await route(f)).action, "noop");
});

test("only maintainer comments posted after pending questions become answers", async () => {
  const payload = {
    action: "created", issue: { number: 42, pull_request: {} },
    comment: { id: 20, body: "1. Option A", user: { login: "maintainer", type: "User" } },
  };
  const pending = comment(result("clarify", { status: "waiting", questions: ["Which users can access the feature?"] }));
  const f = fixture("issue_comment", payload, { comments: [pending] });
  assert.deepEqual(await route(f), { action: "clarify", issue: 7, pr: 42, comment: 20 });
  const old = fixture("issue_comment", { ...payload, comment: { ...payload.comment, id: 10 } }, { comments: [pending] });
  assert.equal((await route(old)).action, "noop");
  const read = fixture("issue_comment", payload, { permission: "read", comments: [pending] });
  assert.equal((await route(read)).action, "noop");
  const complete = fixture("issue_comment", payload, { comments: [comment(result("clarify"))] });
  assert.equal((await route(complete)).action, "noop");
});

test("forks and unmanaged PRs cannot trigger privileged workers", async () => {
  const payload = { action: "labeled", label: { name: "speckit:plan-execution" }, pull_request: { number: 42 } };
  for (const pr of [
    { head: { ref: "speckit/issue-7", repo: { full_name: "outsider/broker" } } },
    { body: "Please plan this", head: { ref: "other", repo: { full_name: "example/broker" } } },
  ]) assert.equal((await route(fixture("pull_request_target", payload, { pr }))).action, "noop");
});

test("planning requires completed clarification for the current specification", async () => {
  const payload = { action: "labeled", label: { name: "speckit:plan-execution" }, pull_request: { number: 42 } };
  await assert.rejects(route(fixture("pull_request_target", payload)), /clarification/i);
  const stale = result("clarify", { files: { ...artifactHashes(files), [`${spec}/spec.md`]: "0".repeat(64) } });
  await assert.rejects(route(fixture("pull_request_target", payload, { comments: [comment(stale)] })), /changed/i);
  const ready = fixture("pull_request_target", payload, { comments: [comment(result("clarify"))] });
  assert.equal((await route(ready)).action, "plan");
});

test("implementation fails closed for missing, stale, blocked, or severe analysis", async () => {
  const hashes = artifactHashes(files);
  for (const report of [
    null,
    result("reanalyze", { status: "blocked" }),
    result("reanalyze", { findings: [{ severity: "HIGH", summary: "Missing authorization", location: "spec.md:12", recommendation: "Specify authorization" }] }),
    result("reanalyze", { files: { ...hashes, [`${spec}/tasks.md`]: "0".repeat(64) } }),
  ]) assert.throws(() => requireResult(report, ["spec.md", "plan.md", "tasks.md"], hashes, { clean: true }), /result|blocked|HIGH|changed/i);
  requireResult(result("reanalyze"), ["spec.md", "plan.md", "tasks.md"], hashes, { clean: true });
});

test("result parsing rejects forged provenance, invalid severities, and missing findings", () => {
  assert.equal(parseResult(comment(result("analyze"), { user: { login: "contributor", type: "User" } })), null);
  assert.equal(parseResult(comment(result("analyze"), { body: "<!-- speckit-result -->" })), null);
  assert.throws(() => parseResult(comment(result("analyze", { findings: [{ severity: "high" }] }))), /finding/i);
  assert.throws(() => parseResult(comment(result("analyze", { findings: null }))), /findings/i);
  assert.deepEqual(parseResult(comment(result("analyze"))), result("analyze"));
});

test("artifact hashes change when any required document changes", () => {
  const before = artifactHashes(files);
  const after = artifactHashes({ ...files, [`${spec}/spec.md`]: "Changed requirements" });
  assert.notEqual(before[`${spec}/spec.md`], after[`${spec}/spec.md`]);
  assert.equal(before[`${spec}/plan.md`], after[`${spec}/plan.md`]);
});

test("task completion markers do not invalidate the approved task definitions", () => {
  const completed = { ...files, [`${spec}/tasks.md`]: "- [X] T001 Implement feature" };
  assert.equal(artifactHashes(files)[`${spec}/tasks.md`], artifactHashes(completed)[`${spec}/tasks.md`]);
});

test("Opus plan reports retain trusted provenance", () => {
  const planned = result("plan");
  assert.deepEqual(parseResult(comment(planned, {
    body: comment(planned).body.replace("speckit-phase", "speckit-plan"),
  })), planned);
});

test("a clean initial analysis cannot bypass the independent reanalysis", async () => {
  const payload = { action: "labeled", label: { name: "speckit:implement" }, pull_request: { number: 42 } };
  const comments = [comment(result("clarify")), comment(result("analyze"), { id: 12 })];
  await assert.rejects(route(fixture("pull_request_target", payload, { comments })), /reanaly/i);
  comments.push(comment(result("reanalyze"), { id: 13 }));
  assert.equal((await route(fixture("pull_request_target", payload, { comments }))).action, "implement");
});

test("queued maintainer replies are accepted once even if the next questions were posted later", async () => {
  const payload = {
    action: "created", issue: { number: 42, pull_request: {} },
    comment: { id: 25, body: "The remaining answer is B", user: { login: "maintainer", type: "User" } },
  };
  const pending = comment(result("clarify", {
    status: "waiting", answer_id: 20, questions: ["Which policy applies?"],
  }), { id: 30 });
  assert.equal((await route(fixture("issue_comment", payload, { comments: [pending] }))).action, "clarify");
  payload.comment.id = 20;
  assert.equal((await route(fixture("issue_comment", payload, { comments: [pending] }))).action, "noop");
});

function preparationIO(t) {
  const attempt = process.env.GITHUB_RUN_ATTEMPT;
  process.env.GITHUB_RUN_ATTEMPT = "1";
  t.after(() => {
    if (attempt === undefined) delete process.env.GITHUB_RUN_ATTEMPT;
    else process.env.GITHUB_RUN_ATTEMPT = attempt;
  });
  t.mock.method(fs, "mkdirSync", () => {});
  const saved = [];
  t.mock.method(fs, "writeFileSync", (name, body) => saved.push({ name, body }));
  return saved;
}

test("specification context preserves only the issue title and body, not comments", async (t) => {
  const saved = preparationIO(t);
  const issue = { number: 7, state: "open", labels: [{ name: "speckit:specify" }], title: "Feature $(not-a-command)", body: "Original issue body" };
  const f = fixture("issues", {}, { issue });
  f.github.rest.issues.listComments = async () => assert.fail("Specification must not fetch comments");
  const data = await prepare({ ...f, inputs: { phase: "specify", issue: 7 } });
  assert.equal(data.title, issue.title);
  assert.equal(data.body, issue.body);
  assert.equal(data.branch, "speckit/issue-7");
  assert.equal(data.answer, undefined);
  assert.deepEqual(JSON.parse(saved[0].body), data);
});

test("tasks consume only a current successful plan from the same pipeline run", async (t) => {
  preparationIO(t);
  const planned = comment(result("plan"), {
    body: comment(result("plan")).body.replace("speckit-phase", "speckit-plan"),
  });
  const f = fixture("pull_request_target", {}, { comments: [planned] });
  const data = await prepare({ ...f, inputs: { phase: "tasks", pr: 42 } });
  assert.equal(data.previous.phase, "plan");
  assert.equal(data.head, "a".repeat(40));
  planned.body = comment(result("plan", { run: "99/1" })).body;
  await assert.rejects(prepare({ ...f, inputs: { phase: "tasks", pr: 42 } }), /different run/);
});

test("pipeline reruns reuse upstream results across attempts but reject other runs and stale artifacts", async (t) => {
  const saved = preparationIO(t);
  process.env.GITHUB_RUN_ATTEMPT = "2";
  for (const [phase, previous] of [
    ["tasks", "plan"], ["analyze", "tasks"], ["remediate", "analyze"], ["reanalyze", "remediate"],
  ]) {
    const upstream = comment(result(previous));
    const f = fixture("pull_request_target", {}, { comments: [upstream] });
    const data = await prepare({ ...f, inputs: { phase, pr: 42 } });
    assert.equal(data.run, "100/2");
    assert.equal(data.previous.run, "100/1");
    assert.equal(JSON.parse(saved.at(-1).body).run, "100/2");
    upstream.body = comment(result(previous, { run: "1000/1" })).body;
    await assert.rejects(prepare({ ...f, inputs: { phase, pr: 42 } }), /different run/);
    upstream.body = comment(result(previous, {
      files: { ...artifactHashes(files), [`${spec}/spec.md`]: "0".repeat(64) },
    })).body;
    await assert.rejects(prepare({ ...f, inputs: { phase, pr: 42 } }), /changed/);
  }
});

test("final verification reuses earlier attempts but rejects other runs and stale artifacts", async (t) => {
  preparationIO(t);
  process.env.GITHUB_RUN_ATTEMPT = "2";
  const posted = comment(result("reanalyze"));
  const f = fixture("pull_request_target", {}, { comments: [posted] });
  assert.equal((await verify({ ...f, comment: 11, phase: "reanalyze" })).run, "100/1");
  posted.body = comment(result("reanalyze", { run: "1000/1" })).body;
  await assert.rejects(verify({ ...f, comment: 11, phase: "reanalyze" }), /provenance/);
  posted.body = comment(result("reanalyze", {
    files: { ...artifactHashes(files), [`${spec}/spec.md`]: "0".repeat(64) },
  })).body;
  await assert.rejects(verify({ ...f, comment: 11, phase: "reanalyze" }), /changed/);
});

test("final verification rejects missing output and incomplete implementation tasks", async (t) => {
  preparationIO(t);
  const f = fixture("pull_request_target", {}, { comments: [comment(result("implement"))] });
  await assert.rejects(verify({ ...f, comment: "", phase: "implement" }), /did not publish/);
  await assert.rejects(verify({ ...f, comment: 11, phase: "implement" }), /unfinished tasks/);
});

test("final verification accepts unanswered clarification without falsely completing it", async (t) => {
  preparationIO(t);
  const pending = result("clarify", { status: "waiting", questions: ["Which policy applies?"] });
  const f = fixture("issues", {}, { comments: [comment(pending)] });
  assert.equal((await verify({ ...f, comment: 11, phase: "clarify" })).status, "waiting");
});
