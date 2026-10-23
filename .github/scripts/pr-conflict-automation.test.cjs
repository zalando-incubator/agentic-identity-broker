const assert = require("node:assert/strict");
const fs = require("node:fs");
const { test } = require("node:test");
const { selectConflicts, prepare } = require("./pr-conflict-automation.cjs");

const owner = "Example";
const repo = "Broker";
const sha = "a".repeat(40);
const baseSha = "b".repeat(40);
const contextPath = "/tmp/gh-aw/pr-conflicts/context.json";

function pr(number = 42, overrides = {}) {
  return {
    number, state: "OPEN", mergeable: "CONFLICTING",
    labels: { nodes: [{ name: "auto-resolve-conflicts" }], pageInfo: { hasNextPage: false } },
    baseRefName: "main", baseRefOid: baseSha, baseRef: { name: "main" },
    headRefName: `feature/${number}`, headRefOid: sha, headRef: { name: `feature/${number}` },
    baseRepository: { nameWithOwner: "example/broker" },
    headRepository: { nameWithOwner: "EXAMPLE/BROKER" },
    ...overrides,
  };
}

function labeled(actor = { __typename: "User", login: "maintainer" }) {
  return { __typename: "LabeledEvent", label: { name: "auto-resolve-conflicts" }, actor };
}

function unlabeled(name = "auto-resolve-conflicts") {
  return { __typename: "UnlabeledEvent", label: { name } };
}

function fixture(eventName = "pull_request_target", options = {}) {
  const prs = options.prs || { 42: pr() };
  const requests = [];
  const github = {
    graphql: async (query, args) => {
      requests.push({ query, args });
      if (options.graphqlError) throw options.graphqlError;
      if (query.includes("query ConflictScan(")) {
        const { nodes, pageInfo } = options.pages?.[args.cursor || "first"] || {
          nodes: Object.values(prs).map(({ number }) => ({ number })),
          pageInfo: { hasNextPage: false, endCursor: null },
        };
        return { repository: { pullRequests: { nodes, pageInfo } } };
      }
      if (query.includes("query ConflictPR(")) {
        return { repository: { pullRequest: prs[args.number] || null } };
      }
      if (query.includes("query ConflictLabelHistory(")) {
        const page = options.historyPages?.[args.cursor || "last"] || {
          nodes: options.history ?? [labeled()],
          pageInfo: { hasPreviousPage: false, startCursor: null },
        };
        return { repository: { pullRequest: { timelineItems: page } } };
      }
      throw new Error("Unexpected GraphQL query");
    },
    rest: { repos: { getCollaboratorPermissionLevel: async ({ username }) => {
      requests.push({ username });
      if (options.permissionError) throw options.permissionError;
      return { data: { permission: options.permission || "write" } };
    } } },
  };
  const context = {
    eventName,
    repo: { owner, repo },
    payload: eventName === "pull_request_target"
      ? { action: "labeled", label: { name: "auto-resolve-conflicts" }, pull_request: { number: 42 } }
      : eventName === "push" ? { ref: "refs/heads/main" } : {},
  };
  return { github, context, requests, prs };
}

function restError(status) {
  const error = new Error(`GitHub returned ${status}`);
  error.status = status;
  return error;
}

test("authorized human label on a conflicted root PR selects its exact head and base", async () => {
  const f = fixture();
  assert.deepEqual(await selectConflicts(f), [{ number: 42, head: sha, base: "main" }]);
  assert.deepEqual(f.requests.filter(({ username }) => username).map(({ username }) => username), ["maintainer"]);
});

test("scheduled scans select stacked PRs while main pushes never select stacks", async () => {
  const stack = pr(43, { baseRefName: "feature/parent", baseRef: { name: "feature/parent" } });
  const scheduled = fixture("schedule", { prs: { 43: stack } });
  assert.deepEqual(await selectConflicts(scheduled), [{ number: 43, head: sha, base: "feature/parent" }]);
  assert.doesNotMatch(scheduled.requests[0].query, /baseRefName: "main"/);
  const push = fixture("push", { prs: { 43: stack } });
  assert.deepEqual(await selectConflicts(push), []);
  assert.match(push.requests[0].query, /baseRefName: "main"/);
});

test("unrelated events and labels do not read PR data", async () => {
  for (const payload of [
    { action: "labeled", label: { name: "other" }, pull_request: { number: 42 } },
    { action: "unlabeled", label: { name: "auto-resolve-conflicts" }, pull_request: { number: 42 } },
  ]) {
    const f = fixture();
    f.context.payload = payload;
    assert.deepEqual(await selectConflicts(f), []);
    assert.equal(f.requests.length, 0);
  }
  const f = fixture("issues");
  assert.deepEqual(await selectConflicts(f), []);
  assert.equal(f.requests.length, 0);
});

test("live root PR is checked on synchronize/reopen/edit; feature-base PR is not", async () => {
  for (const action of ["synchronize", "reopened", "edited"]) {
    const f = fixture();
    f.context.payload = { action, pull_request: { number: 42 } };
    assert.deepEqual(await selectConflicts(f), [{ number: 42, head: sha, base: "main" }]);
    f.prs[42].baseRefName = "feature/parent";
    assert.deepEqual(await selectConflicts(f), []);
  }
});

test("ineligible PRs cannot trigger a worker", async () => {
  const overrides = [
    { mergeable: "MERGEABLE" }, { mergeable: "UNKNOWN" }, { state: "CLOSED" },
    { labels: { nodes: [], pageInfo: { hasNextPage: false } } },
    { headRepository: { nameWithOwner: "outsider/broker" } },
    { baseRepository: { nameWithOwner: "outsider/broker" } },
    { baseRef: null }, { baseRefName: null }, { baseRefOid: null },
    { headRef: null }, { headRefOid: null },
  ];
  for (const change of overrides) {
    const f = fixture("schedule", { prs: { 42: pr(42, change) } });
    assert.deepEqual(await selectConflicts(f), [], JSON.stringify(change));
  }
});

test("label provenance requires the latest application by a currently authorized human", async () => {
  for (const options of [
    { permission: "triage" }, { permission: "read" },
    { history: [labeled({ __typename: "Bot", login: "dependabot" })] },
    { history: [labeled(null)] }, { history: [] },
    { history: [labeled(), unlabeled()] },
    { permissionError: restError(404) },
  ]) {
    const f = fixture("schedule", options);
    assert.deepEqual(await selectConflicts(f), [], Object.keys(options).join(","));
  }
  const f = fixture("schedule", {
    history: [labeled({ __typename: "User", login: "old-maintainer" }), unlabeled(), labeled({ __typename: "User", login: "new-maintainer" })],
  });
  assert.deepEqual(await selectConflicts(f), [{ number: 42, head: sha, base: "main" }]);
  assert.deepEqual(f.requests.filter(({ username }) => username).map(({ username }) => username), ["new-maintainer"]);
});

test("scans paginate PRs and walk backwards through label history", async () => {
  const f = fixture("schedule", {
    prs: { 43: pr(43, { baseRefName: "feature/parent", baseRef: { name: "feature/parent" } }) },
    pages: {
      first: { nodes: [], pageInfo: { hasNextPage: true, endCursor: "more" } },
      more: { nodes: [{ number: 43 }, { number: 43 }], pageInfo: { hasNextPage: false } },
    },
    historyPages: {
      last: { nodes: [unlabeled("other")], pageInfo: { hasPreviousPage: true, startCursor: "older" } },
      older: { nodes: [labeled(), unlabeled("other")], pageInfo: { hasPreviousPage: false } },
    },
  });
  assert.deepEqual(await selectConflicts(f), [{ number: 43, head: sha, base: "feature/parent" }]);
  assert.equal(f.requests.filter(({ username }) => username).length, 1);
});

test("select sorts numbers and rejects more than 256 eligible PRs", async () => {
  const f = fixture("schedule", { prs: { 43: pr(43), 42: pr(42) } });
  assert.deepEqual((await selectConflicts(f)).map(({ number }) => number), [42, 43]);
  const many = Object.fromEntries(Array.from({ length: 257 }, (_, index) => [index + 1, pr(index + 1)]));
  await assert.rejects(selectConflicts(fixture("schedule", { prs: many })), /256/);
});

test("unexpected API failures propagate rather than becoming empty routes", async () => {
  await assert.rejects(selectConflicts(fixture("schedule", { graphqlError: new Error("GraphQL down") })), /GraphQL down/);
  await assert.rejects(selectConflicts(fixture("schedule", { permissionError: restError(403) })), (error) => error.status === 403);
});

test("prepare writes only verified refs and SHAs to the sandbox context", async (t) => {
  t.after(() => fs.rmSync(contextPath, { force: true }));
  const f = fixture("schedule");
  const inputs = { pr: 42, head: sha, base: "main" };
  const expected = {
    eligible: true, pr: 42, head_ref: "feature/42", head_sha: sha,
    base_ref: "main", base_sha: baseSha,
  };
  assert.deepEqual(await prepare({ ...f, inputs }), expected);
  assert.deepEqual(JSON.parse(fs.readFileSync(contextPath, "utf8")), expected);
});

test("prepare independently rechecks label, provenance, head, base, and conflict before AI runs", async (t) => {
  t.after(() => fs.rmSync(contextPath, { force: true }));
  const inputs = { pr: 42, head: sha, base: "main" };
  const changes = [
    (f) => { f.prs[42].headRefOid = "c".repeat(40); },
    (f) => { f.prs[42].baseRefName = "feature/parent"; },
    (f) => { f.prs[42].labels.nodes = []; },
    (f) => { f.prs[42].mergeable = "MERGEABLE"; },
    (f) => { f.prs[42].baseRef = null; },
    (f) => { f.prs[42].state = "CLOSED"; },
  ];
  for (const change of changes) {
    const f = fixture("schedule");
    change(f);
    const result = await prepare({ ...f, inputs });
    assert.equal(result.eligible, false);
    assert.ok(result.reason, "ineligible context must explain why");
    assert.deepEqual(JSON.parse(fs.readFileSync(contextPath, "utf8")), result);
  }
  for (const options of [
    { permission: "triage" },
    { history: [labeled({ __typename: "Bot", login: "automation" })] },
  ]) {
    const result = await prepare({ ...fixture("schedule", options), inputs });
    assert.equal(result.eligible, false);
  }
});
