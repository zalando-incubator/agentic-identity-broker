const fs = require("node:fs");

const directory = "/tmp/gh-aw/architecture-review";
const heading = "## 🏗️ Architectural Review";
// Safe outputs strip HTML comments from agent comments, so the state travels in a visible JSON block.
const marker = "## Architecture review result";
const statusMarker = "<!-- architecture-review-status -->";
const dimensions = ["ubiquitous-language", "hexagonal-compliance", "adr-compliance", "data-authority",
  "coupling-cohesion", "otel-parity", "e2e-test-quality", "maintainability"];
const severities = ["RED", "YELLOW", "GREEN"];
const recommendations = ["APPROVE", "DISCUSS", "REQUEST CHANGES"];
const sections = ["### Executive Summary", "### Top Findings", "### Concept Map", "### Hexagonal Compliance",
  "### Pattern Consistency", "### Coupling & Cohesion", "### OTel Parity", "### E2E Test Assessment",
  "### Maintainability Forecast", "### Recommendation"];
const pipeline = ["route", "review", "verify"];
const maxFindings = 7;
const maxBody = 60000;

function parseCommand(body) {
  const match = typeof body === "string" && body.match(/^\/architecture-review(?=\s|$)([\s\S]*)$/);
  return match ? { argument: match[1].trim() } : null;
}

function actionsBot(user) {
  return user?.login === "github-actions[bot]" && user?.type === "Bot";
}

async function maintainer(github, repo, login) {
  const { data } = await github.rest.repos.getCollaboratorPermissionLevel({ ...repo, username: login });
  return ["admin", "maintain", "write"].includes(data.permission);
}

// Fork heads never reach the worker: it fetches the head SHA from origin and runs PR code only in the sandbox.
function eligible(pr, repo) {
  const slug = `${repo.owner}/${repo.repo}`;
  return pr.state === "open" && !pr.merged_at && pr.head.repo?.full_name === slug && pr.base.repo?.full_name === slug;
}

function feature(pr) {
  return pr.body?.match(/<!-- speckit-feature: (specs\/[a-zA-Z0-9][a-zA-Z0-9-]*) -->/)?.[1] ?? null;
}

function quoted(text) {
  return text.replace(/@/g, "@​").replace(/</g, "&lt;");
}

function runURL(repo, id) {
  return `https://github.com/${repo.owner}/${repo.repo}/actions/runs/${id}`;
}

async function route({ github, context }) {
  const { payload, repo, eventName } = context;
  const source = eventName === "issue_comment" && payload.action === "created" ? payload.comment : null;
  const command = source?.user?.type === "User" && source.user.login === context.actor && parseCommand(source.body);
  if (!command || !await maintainer(github, repo, context.actor)) return { action: "noop" };
  if (!payload.issue?.pull_request) throw new Error("/architecture-review runs on a pull request, not an issue");
  const { data: pr } = await github.rest.pulls.get({ ...repo, pull_number: payload.issue.number });
  if (!eligible(pr, repo)) throw new Error("/architecture-review runs on an open pull request from this repository");
  return { action: "review", pr: pr.number, comment: source.id };
}

async function maintainerCommand(github, repo, id, number) {
  const { data } = await github.rest.issues.getComment({ ...repo, comment_id: id });
  const command = parseCommand(data.body);
  if (!data.issue_url?.endsWith(`/issues/${number}`) || data.user?.type !== "User" || !command ||
      !await maintainer(github, repo, data.user.login)) {
    throw new Error("Comment is not an eligible maintainer command");
  }
  return { id: data.id, author: data.user.login, argument: command.argument };
}

async function adrs(github, repo, ref) {
  let data;
  try {
    ({ data } = await github.rest.repos.getContent({ ...repo, ref, path: "adrs" }));
  } catch (error) {
    if (error.status === 404) return [];
    throw error;
  }
  if (!Array.isArray(data)) throw new Error("Expected adrs/ to be a directory");
  return data.filter((entry) => entry.type === "file" && entry.name.endsWith(".md")).map((entry) => entry.path).sort();
}

async function prepare({ github, context, inputs }) {
  const { repo } = context;
  if (!await maintainer(github, repo, context.actor)) throw new Error("Maintainer permission is required");
  const number = Number(inputs.pr);
  if (!Number.isSafeInteger(number) || number <= 0) throw new Error("A pull request number is required");
  const { data: pr } = await github.rest.pulls.get({ ...repo, pull_number: number });
  if (!eligible(pr, repo)) throw new Error("Only open, same-repository pull requests are eligible");
  const data = {
    run: `${context.runId}/${process.env.GITHUB_RUN_ATTEMPT}`, repository: `${repo.owner}/${repo.repo}`,
    pr: pr.number, head: pr.head.sha, branch: pr.head.ref, base: pr.base.ref, base_sha: pr.base.sha,
    feature: feature(pr), focus: "",
  };
  if (Number(inputs.comment)) {
    data.command = await maintainerCommand(github, repo, Number(inputs.comment), pr.number);
    data.focus = data.command.argument;
  }
  const files = await github.paginate(github.rest.pulls.listFiles, { ...repo, pull_number: pr.number, per_page: 100 });
  const changed = files.map((file) => ({
    path: file.filename, status: file.status, additions: file.additions, deletions: file.deletions,
    ...(file.previous_filename ? { previous_path: file.previous_filename } : {}),
  }));
  const isADR = (file) => /^adrs\/[^/]+\.md$/.test(file.path);
  const details = {
    number: pr.number, title: pr.title, body: pr.body || "", url: pr.html_url,
    headRefOid: pr.head.sha, headRefName: pr.head.ref, baseRefOid: pr.base.sha, baseRefName: pr.base.ref,
    feature: data.feature, changedFiles: changed,
    adrs: {
      existing: await adrs(github, repo, pr.base.sha),
      added: changed.filter((file) => isADR(file) && file.status === "added").map((file) => file.path),
      modified: changed.filter((file) => isADR(file) && file.status !== "added").map((file) => file.path),
    },
  };
  fs.mkdirSync(directory, { recursive: true });
  fs.writeFileSync(`${directory}/context.json`, JSON.stringify(data, null, 2));
  fs.writeFileSync(`${directory}/pr-details.json`, JSON.stringify(details, null, 2));
  return data;
}

function validateResult(draft, context) {
  if (!recommendations.includes(draft.recommendation)) throw new Error("Invalid recommendation");
  if (typeof draft.summary !== "string" || !draft.summary.trim() || draft.summary.length > 200 ||
      /[\r\n\u2028\u2029]/.test(draft.summary)) {
    throw new Error("A one-line summary of at most 200 characters is required");
  }
  if (!Array.isArray(draft.findings) || draft.findings.length > maxFindings) {
    throw new Error(`Findings must list at most ${maxFindings} items`);
  }
  for (const finding of draft.findings) {
    if (!severities.includes(finding?.severity) || !dimensions.includes(finding.dimension) ||
        typeof finding.title !== "string" || !finding.title.trim() || finding.title.length > 200 ||
        /[\r\n]/.test(finding.title)) {
      throw new Error("Invalid finding");
    }
  }
  const status = draft.dimensions;
  if (!status || typeof status !== "object" || Array.isArray(status) ||
      Object.keys(status).length !== dimensions.length ||
      dimensions.some((name) => !["assessed", "failed"].includes(status[name]))) {
    throw new Error("Every analysis dimension must be reported as assessed or failed");
  }
  const failed = dimensions.filter((name) => status[name] === "failed");
  const counts = Object.fromEntries(severities.map((severity) =>
    [severity, draft.findings.filter((finding) => finding.severity === severity).length]));
  if (failed.length > 1) {
    throw new Error("More than one unassessed dimension cannot be published; report the blocker instead");
  }
  if (failed.length && draft.recommendation === "APPROVE") {
    throw new Error("A review with unassessed dimensions cannot approve");
  }
  if (counts.RED && draft.recommendation !== "REQUEST CHANGES") {
    throw new Error("A 🔴 finding requires REQUEST CHANGES");
  }
  if (!counts.RED && !counts.YELLOW && draft.recommendation === "REQUEST CHANGES") {
    throw new Error("REQUEST CHANGES needs at least one 🔴 or 🟡 finding");
  }
  const body = draft.body;
  if (typeof body !== "string" || !body.startsWith(`${heading}\n`)) throw new Error("The review body must start with the review heading");
  if (body.length > maxBody) throw new Error(`The review body exceeds ${maxBody} characters`);
  for (const section of sections) {
    if (!body.includes(`\n${section}`)) throw new Error(`The review body is missing the "${section}" section`);
  }
  if (!body.includes(`**${draft.recommendation}**`)) throw new Error("The review body must state the recommendation");
  if (/<!-- (?:architecture-review|gh-aw)/.test(body) || body.includes(marker)) {
    throw new Error("The review body must not contain reserved markers");
  }
  const prefix = `https://github.com/${context.repository}/blob/`;
  for (const match of body.matchAll(/https:\/\/github\.com\/[^/\s)]+\/[^/\s)]+\/blob\/([^/\s)]+)\//g)) {
    if (!match[0].startsWith(prefix) || match[1] !== context.head) {
      throw new Error(`File links must use ${prefix}${context.head}/: ${match[0]}`);
    }
  }
  if (failed.length && !body.includes("not assessed")) {
    throw new Error("The review body must say which dimensions were not assessed");
  }
  return { failed, counts };
}

function writeResult() {
  const context = JSON.parse(fs.readFileSync(`${directory}/context.json`, "utf8"));
  const draft = JSON.parse(fs.readFileSync(`${directory}/result.json`, "utf8"));
  const { failed, counts } = validateResult(draft, context);
  const state = {
    run: context.run, pr: context.pr, head: context.head, recommendation: draft.recommendation,
    summary: draft.summary.trim(), findings: counts, failed,
  };
  fs.writeFileSync(`${directory}/comment.md`,
    [draft.body.trimEnd(), "", "<details><summary>Review state</summary>", "", marker,
      "```json", JSON.stringify(state, null, 2), "```", "", "</details>", ""].join("\n"));
}

function validateState(state) {
  if (!/^\d+\/\d+$/.test(state.run) || !/^[0-9a-f]{40}$/.test(state.head) ||
      !Number.isSafeInteger(state.pr) || state.pr <= 0 || !recommendations.includes(state.recommendation) ||
      typeof state.summary !== "string" || !state.findings || typeof state.findings !== "object" ||
      severities.some((severity) => !Number.isSafeInteger(state.findings[severity])) ||
      !Array.isArray(state.failed) || state.failed.some((name) => !dimensions.includes(name))) {
    throw new Error("Invalid architecture review state");
  }
  return state;
}

// Only the worker's safe output carries the workflow footer, so an agent comment cannot pose as a review.
function parseReview(comment) {
  const body = comment.body || "";
  const issue = comment.issue_url?.match(/^https:\/\/api\.github\.com\/repos\/([^/]+)\/([^/]+)\/issues\/(\d+)$/);
  if (!actionsBot(comment.user) || !issue || body.split("<!-- gh-aw-workflow-call-id:").length !== 2) return null;
  const footer = body.match(/^<!-- gh-aw-workflow-call-id: ([^/\s]+)\/([^/\s]+)\/architecture-review -->$/m);
  if (!footer || footer[1] !== issue[1] || footer[2] !== issue[2]) return null;
  const sections = body.split(marker);
  if (sections.length !== 2) return null;
  const blocks = [...sections[1].matchAll(/^```json\n([\s\S]*?)\n```$/gm)];
  if (blocks.length !== 1) return null;
  try {
    const state = validateState(JSON.parse(blocks[0][1]));
    return state.pr === Number(issue[3]) ? state : null;
  } catch {
    return null;
  }
}

async function verify({ github, context, comment }) {
  if (!Number.isSafeInteger(Number(comment)) || Number(comment) <= 0) throw new Error("Worker did not publish a review comment");
  const { data: posted } = await github.rest.issues.getComment({ ...context.repo, comment_id: Number(comment) });
  const review = parseReview(posted);
  if (!review || review.run.split("/")[0] !== String(context.runId)) {
    throw new Error("Review provenance does not match this run");
  }
  const { data: pr } = await github.rest.pulls.get({ ...context.repo, pull_number: review.pr });
  if (!eligible(pr, context.repo)) throw new Error("Review is not on an open, same-repository pull request");
  if (pr.head.sha !== review.head) {
    throw new Error("The pull request changed while it was reviewed. Comment `/architecture-review` again");
  }
  return review;
}

async function statusComments(github, repo, number) {
  const comments = await github.paginate(github.rest.issues.listComments, { ...repo, issue_number: number, per_page: 100 });
  return comments.filter((entry) => actionsBot(entry.user) && (entry.body || "").includes(statusMarker) &&
    !(entry.body || "").includes("<!-- gh-aw-agentic-workflow:"));
}

// The review itself is the worker's comment. This job only replaces the status comment, and only on failure.
async function report({ github, context, needs }) {
  const { repo } = context;
  const route = needs.route || {};
  if (route.result === "success" && route.outputs?.action === "noop") return null;
  if (!await maintainer(github, repo, context.actor)) return null;
  const number = Number(route.outputs?.pr) || context.payload.issue?.number;
  if (!number) return null;
  for (const stale of await statusComments(github, repo, number)) {
    await github.rest.issues.deleteComment({ ...repo, comment_id: stale.id });
  }
  const jobs = Object.entries(needs).filter(([name]) => pipeline.includes(name))
    .sort(([a], [b]) => pipeline.indexOf(a) - pipeline.indexOf(b));
  const error = jobs.find(([, job]) => job.result === "failure" && job.outputs?.error);
  const failed = jobs.find(([, job]) => job.result === "failure");
  if (!error && !failed) return null;
  const title = error ? `⛔ Stopped in \`${error[0]}\`` : `⛔ \`${failed[0]}\` failed`;
  const lines = error ? [quoted(error[1].outputs.error)] :
    ["The job failed before publishing a review. Check the workflow run, then comment `/architecture-review` again."];
  const body = [`## 🏗️ Architecture review · ${title}`, "", ...lines, "", `[Workflow run](${runURL(repo, context.runId)})`,
    "", statusMarker, ""];
  const { data } = await github.rest.issues.createComment({ ...repo, issue_number: number, body: body.join("\n") });
  return data;
}

module.exports = { parseCommand, eligible, route, prepare, validateResult, parseReview, verify, report, dimensions };

if (require.main === module) {
  if (process.argv[2] === "result") {
    writeResult();
  } else {
    throw new Error("Expected the result command");
  }
}
