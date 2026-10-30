const { createHash } = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const phases = ["clarify", "plan", "tasks", "analyze", "remediate", "reanalyze", "implement"];
const commands = ["specify", "clarify", "plan", "implement"];
const reviewCommands = ["clarify", "plan"];
const documents = ["spec.md", "plan.md", "tasks.md"];
const marker = "## Spec Kit result";
const directory = "/tmp/gh-aw/speckit";
const approvalKinds = { api: "API", adr: "ADR", decision: "Decision" };
const approvalMarker = /^<!-- speckit-approval: (\{.*\}) -->$/m;
const statusMarker = "<!-- speckit-status -->";
const commitNotice = /^Commit pushed: \[`([0-9a-f]{7,40})`\]\((https:\/\/github\.com\/\S+?\/commit\/[0-9a-f]{40})\)/;
const blocking = ["CRITICAL", "HIGH"];
const pipeline = ["route", "specify", "clarify", "clarified", "plan", "tasks", "analyze", "remediate", "reanalyze",
  "implement", "verify", "approvals"];
const labels = {
  clarify: "Clarify", plan: "Plan", tasks: "Tasks", analyze: "Analyze", remediate: "Remediate",
  reanalyze: "Reanalyze", implement: "Implement",
};
const running = { clarify: "Clarifying the specification", plan: "Planning", implement: "Implementing" };

function artifactHashes(files) {
  return Object.fromEntries(Object.entries(files).map(([name, content]) => [
    name, createHash("sha256").update(name.endsWith("/tasks.md")
      ? content.replace(/^(\s*-\s+)\[[xX ]\](\s+T\d+\b)/gm, "$1[ ]$2")
      : content).digest("hex"),
  ]));
}

function featurePath(value) {
  if (typeof value !== "string" || !/^specs\/[a-zA-Z0-9][a-zA-Z0-9-]*$/.test(value)) {
    throw new Error("Invalid Spec Kit feature directory");
  }
  return value;
}

function validateResult(result) {
  featurePath(result.feature);
  if (!phases.includes(result.phase) || !["complete", "waiting", "blocked"].includes(result.status) ||
      !/^\d+\/\d+$/.test(result.run) || !result.files || typeof result.files !== "object") {
    throw new Error("Invalid Spec Kit result");
  }
  for (const [name, hash] of Object.entries(result.files)) {
    if (!documents.some((doc) => name === `${result.feature}/${doc}`) || !/^[a-f0-9]{64}$/.test(hash)) {
      throw new Error("Invalid result file hash");
    }
  }
  if (!Array.isArray(result.findings) || result.findings.length > 50) throw new Error("Invalid findings");
  for (const finding of result.findings) {
    if (!["CRITICAL", "HIGH", "MEDIUM", "LOW"].includes(finding.severity) ||
        !["summary", "location", "recommendation"].every((key) => typeof finding[key] === "string" && finding[key].trim())) {
      throw new Error("Invalid analysis finding");
    }
  }
  if (!Array.isArray(result.questions) || result.questions.length > 5 ||
      result.questions.some((question) => typeof question !== "string" || !question.trim()) ||
      (result.status === "waiting" && !result.questions.length) ||
      (result.phase === "clarify" && result.status === "complete" && result.questions.length)) {
    throw new Error("Invalid clarification questions");
  }
  if (result.answer_id !== undefined && (!Number.isSafeInteger(result.answer_id) || result.answer_id < 0)) {
    throw new Error("Invalid clarification answer ID");
  }
  if (result.approvals !== undefined) {
    if (!["analyze", "reanalyze"].includes(result.phase) || !Array.isArray(result.approvals) ||
        result.approvals.length > 25) {
      throw new Error("Invalid approval items");
    }
    for (const item of result.approvals) {
      if (!Object.hasOwn(approvalKinds, item?.kind) || typeof item.title !== "string" || !item.title.trim() ||
          item.title.length > 300 || /[\r\n]/.test(item.title)) {
        throw new Error("Invalid approval item");
      }
      approvalLocation(item.location);
    }
  }
  if (result.next_step !== undefined &&
      (typeof result.next_step !== "string" || !result.next_step.trim() || result.next_step.length > 1500)) {
    throw new Error("Invalid next step");
  }
  if (result.summary !== undefined && (typeof result.summary !== "string" || !result.summary.trim() ||
      result.summary.length > 200 || /[\r\n\u2028\u2029]/.test(result.summary))) {
    throw new Error("Invalid summary");
  }
  return result;
}

// A maintainer reviews the first specification before any planning. Once a plan exists, a completed
// clarification refreshes the plan, tasks, and analysis without another command.
function continuesToPlan(result) {
  return result.phase === "clarify" && result.status === "complete" && Boolean(result.files[`${result.feature}/plan.md`]);
}

function approvalLocation(location) {
  const match = typeof location === "string" &&
    location.match(/^([A-Za-z0-9][A-Za-z0-9._/-]*):([1-9]\d*)(?:-([1-9]\d*))?$/);
  const start = match && Number(match[2]);
  const end = match && Number(match[3] || match[2]);
  if (!match || end < start || match[1].split("/").some((part) => [".", "..", ""].includes(part))) {
    throw new Error(`Invalid approval item location: ${String(location).slice(0, 200)}`);
  }
  return { path: match[1], start, end };
}

function actionsBot(user) {
  return user?.login === "github-actions[bot]" && user?.type === "Bot";
}

function parseCommand(body) {
  const match = typeof body === "string" && body.match(/^\/speckit-([a-z]+)(?=\s|$)([\s\S]*)$/);
  return match ? { phase: match[1], argument: match[2].trim() } : null;
}

function parseResult(comment) {
  const body = comment.body || "";
  const issue = comment.issue_url?.match(/^https:\/\/api\.github\.com\/repos\/([^/]+)\/([^/]+)\/issues\/\d+$/);
  const sections = body.split(marker);
  if (!actionsBot(comment.user) || !issue || body.split("<!-- gh-aw-workflow-call-id:").length !== 2 ||
      sections.length > 2) return null;
  const footer = body.match(/^<!-- gh-aw-workflow-call-id: ([^/\s]+)\/([^/\s]+)\/(speckit-(?:phase|plan|analyze|implement)) -->$/m);
  if (!footer || footer[1] !== issue[1] || footer[2] !== issue[2]) return null;
  const blocks = [...sections[sections.length - 1].matchAll(/^```json\n([\s\S]*?)\n```$/gm)];
  if (blocks.length !== 1) return null;
  let result;
  try {
    result = validateResult(JSON.parse(blocks[0][1]));
  } catch {
    return null;
  }
  const workflow = ["clarify", "tasks", "remediate"].includes(result.phase) ? "speckit-phase" :
    result.phase === "reanalyze" ? "speckit-analyze" : `speckit-${result.phase}`;
  return footer[3] === workflow ? result : null;
}

function requireUnchanged(result, required, hashes) {
  for (const name of required) {
    const file = `${result.feature}/${name}`;
    if (!hashes[file] || result.files[file] !== hashes[file]) throw new Error(`${file} changed after ${result.phase}`);
  }
}

function requireResult(result, required, hashes, { clean = false, run } = {}) {
  if (!result) throw new Error("Required Spec Kit result is missing (clarification or previous phase)");
  if (result.status !== "complete") throw new Error(`Spec Kit ${result.phase} is ${result.status}`);
  if (run && result.run.split("/")[0] !== run.split("/")[0]) throw new Error("Previous phase result belongs to a different run");
  requireUnchanged(result, required, hashes);
  if (clean && result.findings.some((finding) => ["CRITICAL", "HIGH"].includes(finding.severity))) {
    throw new Error("Unresolved CRITICAL/HIGH findings block implementation");
  }
}

// Implementation ticks and adds tasks, so tasks.md is only held to the reanalysis until it starts.
function reviewedDocuments(entries, reanalysis) {
  const started = entries.some((entry) => entry.result.phase === "implement" && entry.comment.id > reanalysis.comment.id);
  return started ? ["spec.md", "plan.md"] : documents;
}

// Later phases (plan, remediate) may revise the specification and record its hash,
// so the specification is compared with the latest result rather than the clarification.
function requireSpecification(entries, hashes) {
  const clarified = latest(entries, ["clarify"])?.result;
  requireResult(clarified, [], hashes);
  requireUnchanged(latest(entries, phases).result, ["spec.md"], hashes);
  return clarified;
}

async function maintainer(github, repo, login) {
  const { data } = await github.rest.repos.getCollaboratorPermissionLevel({ ...repo, username: login });
  return ["admin", "maintain", "write"].includes(data.permission);
}

async function maintainerCommand(github, repo, id, number, phase) {
  const { data } = await github.rest.issues.getComment({ ...repo, comment_id: id });
  const command = parseCommand(data.body);
  if (!data.issue_url?.endsWith(`/issues/${number}`) || data.user?.type !== "User" ||
      command?.phase !== phase || !await maintainer(github, repo, data.user.login)) {
    throw new Error("Comment is not an eligible maintainer command");
  }
  return { id: data.id, author: data.user.login, argument: command.argument };
}

async function reviewCommand(github, repo, id, pr, phase) {
  const { data: review } = await github.rest.pulls.getReview({ ...repo, pull_number: pr, review_id: id });
  const command = parseCommand(review.body);
  if (!review.pull_request_url?.endsWith(`/pulls/${pr}`) || review.user?.type !== "User" ||
      command?.phase !== phase || !await maintainer(github, repo, review.user.login)) {
    throw new Error("Review is not an eligible maintainer review command");
  }
  const all = await github.paginate(github.rest.pulls.listReviewComments, { ...repo, pull_number: pr, per_page: 100 });
  const submitted = Date.parse(review.submitted_at);
  const comments = all.filter((entry) => entry.pull_request_review_id === review.id).sort((a, b) => a.id - b.id);
  return {
    id: review.id, author: review.user.login, argument: command.argument,
    comments: comments.map((entry) => {
      const root = entry.in_reply_to_id;
      const context = root ? all.filter((other) => other.pull_request_review_id !== review.id && other.id < entry.id &&
        (other.id === root || other.in_reply_to_id === root)).sort((a, b) => a.id - b.id) : [];
      if (context.some((other) => Date.parse(other.updated_at) > submitted)) {
        throw new Error("Review thread context changed after the review was submitted");
      }
      return {
        path: entry.path, line: entry.line ?? entry.original_line,
        start_line: entry.start_line ?? entry.original_start_line ?? null, body: entry.body,
        context: context.map((other) => ({ author: other.user?.login, body: other.body })),
      };
    }),
  };
}

function managedPR(pr, repo, state = "open") {
  const relatedPrefix = `Related issue: https://github.com/${repo.owner}/${repo.repo}/issues/`;
  const related = (pr.body || "").split(/\r?\n/).find((line) => line.startsWith(relatedPrefix));
  const issue = pr.body?.match(/<!-- speckit-issue: (\d+) -->/) ||
    related?.slice(relatedPrefix.length).match(/^(\d+)$/);
  return pr.state === state && pr.draft && issue && actionsBot(pr.user) &&
    pr.head.repo?.full_name === `${repo.owner}/${repo.repo}` &&
    pr.base.repo?.full_name === `${repo.owner}/${repo.repo}` &&
    pr.head.ref === `speckit/issue-${issue[1]}` ? Number(issue[1]) : 0;
}

async function readBlob(github, repo, ref, name, optional = false) {
  let data;
  try {
    ({ data } = await github.rest.repos.getContent({ ...repo, ref, path: name }));
  } catch (error) {
    if (optional && error.status === 404) return null;
    throw error;
  }
  if (data.type !== "file" || data.encoding !== "base64" || data.target) {
    throw new Error(`Expected a regular file: ${name}`);
  }
  return { text: Buffer.from(data.content, "base64").toString("utf8"), sha: data.sha };
}

async function readFile(github, repo, ref, name, optional = false) {
  return (await readBlob(github, repo, ref, name, optional))?.text ?? null;
}

// The feature directory's tree SHA covers every pre-implementation artifact; files outside it are
// pinned individually, so an approval never outlives a change to what the maintainer reviewed.
async function fingerprint(github, repo, ref, feature, external) {
  const { data: entries } = await github.rest.repos.getContent({ ...repo, ref, path: path.posix.dirname(feature) });
  const directory = Array.isArray(entries) && entries.find((entry) => entry.path === feature && entry.type === "dir");
  if (!directory?.sha) throw new Error(`Feature directory is missing: ${feature}`);
  const files = {};
  for (const name of [...new Set(external)].sort()) files[name] = (await readBlob(github, repo, ref, name)).sha;
  return { tree: directory.sha, files };
}

async function artifacts(github, repo, pr) {
  const body = pr.body || "";
  const markers = body.split("<!-- speckit-feature:").length - 1;
  if (markers > 1) throw new Error("Multiple Spec Kit feature markers");
  let selected;
  if (markers) {
    selected = body.match(/<!-- speckit-feature: ([^\r\n]*?) -->/)?.[1] ?? "";
  } else {
    const prefix = `Specification: https://github.com/${repo.owner}/${repo.repo}/blob/${pr.head.ref}/`;
    const links = body.split(/\r?\n/).filter((line) => line.startsWith(prefix));
    if (links.length > 1) throw new Error("Multiple Spec Kit specification links");
    if (links.length) {
      const target = links[0].slice(prefix.length);
      if (!target.endsWith("/spec.md")) throw new Error("Invalid Spec Kit specification link");
      selected = target.slice(0, -"/spec.md".length);
    }
  }
  if (selected === undefined) {
    const state = JSON.parse(await readFile(github, repo, pr.head.sha, ".specify/feature.json"));
    selected = state.feature_directory;
  }
  const feature = featurePath(selected);
  const files = {};
  for (const name of documents) {
    const content = await readFile(github, repo, pr.head.sha, `${feature}/${name}`, name !== "spec.md");
    if (content !== null) files[`${feature}/${name}`] = content;
  }
  return { feature, files, hashes: artifactHashes(files) };
}

async function reports(github, repo, number) {
  const comments = await github.paginate(github.rest.issues.listComments, { ...repo, issue_number: number, per_page: 100 });
  return comments.map((comment) => ({ comment, result: parseResult(comment) })).filter((entry) => entry.result);
}

function latest(entries, allowed) {
  return entries.filter(({ result }) => allowed.includes(result.phase)).sort((a, b) => b.comment.id - a.comment.id)[0];
}

async function routeIssue(github, repo, issue, comment) {
  if (issue.state !== "open") return { action: "noop" };
  const branch = `speckit/issue-${issue.number}`;
  const existing = await github.paginate(github.rest.pulls.list, {
    ...repo, state: "all", head: `${repo.owner}:${branch}`, per_page: 100,
  });
  if (existing.length) {
    const managed = existing.find((pr) => managedPR(pr, repo) === issue.number);
    if (managed) {
      if (latest(await reports(github, repo, managed.number), ["clarify"])) return { action: "noop" };
      return { action: "clarify", issue: issue.number, pr: managed.number, comment: 0 };
    }
    if (existing.some((pr) => pr.merged_at ||
        managedPR(pr, repo, "closed") !== issue.number)) return { action: "noop" };
  }
  try {
    await github.rest.repos.getBranch({ ...repo, branch });
    return { action: "noop" };
  } catch (error) {
    if (error.status !== 404) throw error;
  }
  return { action: "specify", issue: issue.number, pr: 0, comment };
}

async function route({ github, context }) {
  const { payload, repo, eventName } = context;
  const label = payload.label?.name;
  const isIssue = eventName === "issues" && payload.action === "labeled" &&
    label === "speckit:specify" && !payload.issue?.pull_request;
  const isLabel = eventName === "pull_request_target" && payload.action === "labeled" &&
    ["speckit:plan-execution", "speckit:implement"].includes(label);
  const isReview = eventName === "pull_request_review" && payload.action === "submitted";
  const source = isReview ? payload.review :
    eventName === "issue_comment" && payload.action === "created" ? payload.comment : null;
  const command = source?.user?.type === "User" && source.user.login === context.actor && parseCommand(source.body);
  if ((!isIssue && !isLabel && !command) ||
      !await maintainer(github, repo, context.actor)) return { action: "noop" };
  if (isIssue) return routeIssue(github, repo, payload.issue, 0);
  if (command) {
    if (!commands.includes(command.phase)) throw new Error(`Unsupported Spec Kit command: /speckit-${command.phase}`);
    if (isReview && !reviewCommands.includes(command.phase)) {
      throw new Error(`/speckit-${command.phase} runs in a pull request comment, not a review`);
    }
    const onPR = isReview || Boolean(payload.issue.pull_request);
    if (command.phase === "specify") {
      if (onPR) throw new Error("/speckit-specify runs on an issue, not a pull request");
      if (command.argument) throw new Error("/speckit-specify takes no argument; edit the issue instead");
      return routeIssue(github, repo, payload.issue, payload.comment.id);
    }
    if (!onPR) throw new Error(`/speckit-${command.phase} runs on a Spec Kit pull request`);
  }
  const number = command && !isReview ? payload.issue.number : payload.pull_request.number;
  const { data: pr } = await github.rest.pulls.get({ ...repo, pull_number: number });
  const issue = managedPR(pr, repo);
  if (!issue) return { action: "noop" };
  const entries = await reports(github, repo, number);
  const comment = command && !isReview ? payload.comment.id : 0;
  const routed = (action) => ({ action, issue, pr: number, comment, ...(isReview ? { review: payload.review.id } : {}) });
  if (command?.phase === "clarify") {
    const pending = latest(entries, ["clarify"]);
    if (!isReview && pending?.result.status === "waiting") {
      if (!command.argument) throw new Error("Answer the pending questions with /speckit-clarify followed by your answers");
      if (comment <= (pending.result.answer_id || pending.comment.id)) return { action: "noop" };
    }
    return routed("clarify");
  }
  const action = command?.phase ?? (label === "speckit:implement" ? "implement" : "plan");
  const current = await artifacts(github, repo, pr);
  requireSpecification(entries, current.hashes);
  if (action === "implement") {
    const analysis = latest(entries, ["analyze", "reanalyze"]);
    if (analysis?.result.phase !== "reanalyze") throw new Error("Independent reanalysis is required before implementation");
    requireResult(analysis.result, reviewedDocuments(entries, analysis), current.hashes, { clean: true });
    await approval(github, repo, pr, current.feature, analysis.comment, entries);
  }
  return routed(action);
}

async function prepare({ github, context, inputs }) {
  if (!await maintainer(github, context.repo, context.actor)) throw new Error("Maintainer permission is required");
  const run = `${context.runId}/${process.env.GITHUB_RUN_ATTEMPT}`;
  const data = { phase: inputs.phase, run, repository: `${context.repo.owner}/${context.repo.repo}` };
  if (inputs.phase === "specify") {
    const command = Number(inputs.comment)
      ? await maintainerCommand(github, context.repo, Number(inputs.comment), Number(inputs.issue), "specify") : null;
    if (command?.argument) throw new Error("/speckit-specify takes no argument; edit the issue instead");
    const { data: issue } = await github.rest.issues.get({ ...context.repo, issue_number: Number(inputs.issue) });
    if (issue.pull_request || issue.state !== "open" ||
        (!command && !issue.labels.some((label) => label.name === "speckit:specify"))) {
      throw new Error("Issue is not eligible for specification");
    }
    Object.assign(data, { issue: issue.number, title: issue.title, body: issue.body || "", branch: `speckit/issue-${issue.number}` });
  } else {
    if (!phases.includes(inputs.phase)) throw new Error("Unsupported Spec Kit phase");
    const { data: pr } = await github.rest.pulls.get({ ...context.repo, pull_number: Number(inputs.pr) });
    if (!managedPR(pr, context.repo)) throw new Error("Only managed, same-repository draft PRs are eligible");
    const current = await artifacts(github, context.repo, pr);
    const entries = await reports(github, context.repo, pr.number);
    const previous = {
      tasks: ["plan"], analyze: ["tasks"],
      remediate: ["analyze"], reanalyze: ["remediate"], implement: ["analyze", "reanalyze"],
    }[inputs.phase];
    if (inputs.phase === "plan") {
      data.previous = requireSpecification(entries, current.hashes);
    } else if (previous) {
      const upstream = latest(entries, previous);
      data.previous = upstream?.result;
      if (inputs.phase === "implement" && data.previous?.phase !== "reanalyze") {
        throw new Error("Independent reanalysis is required before implementation");
      }
      const required = inputs.phase === "tasks" ? ["spec.md", "plan.md"] :
        inputs.phase === "implement" ? reviewedDocuments(entries, upstream) : documents;
      requireResult(data.previous, required, current.hashes, {
        clean: inputs.phase === "implement",
        run: ["tasks", "analyze", "remediate", "reanalyze"].includes(inputs.phase) ? run : undefined,
      });
      if (inputs.phase === "implement") {
        data.approval = await approval(github, context.repo, pr, current.feature, upstream.comment, entries);
      }
    }
    Object.assign(data, { pr: pr.number, head: pr.head.sha, branch: pr.head.ref, feature: current.feature });
    if (Number(inputs.comment) && Number(inputs.review)) throw new Error("A phase accepts one maintainer command");
    if (Number(inputs.review)) {
      if (!reviewCommands.includes(inputs.phase)) throw new Error("Only clarify and plan accept a review command");
      data.command = await reviewCommand(github, context.repo, Number(inputs.review), pr.number, inputs.phase);
      const pending = inputs.phase === "clarify" ? latest(entries, ["clarify"]) : undefined;
      if (pending?.result.status === "waiting") data.pending_questions = pending.result.questions;
    } else if (Number(inputs.comment)) {
      if (!commands.includes(inputs.phase)) throw new Error("Only command entry phases accept a maintainer command");
      const command = await maintainerCommand(github, context.repo, Number(inputs.comment), pr.number, inputs.phase);
      const pending = inputs.phase === "clarify" ? latest(entries, ["clarify"]) : undefined;
      if (pending?.result.status === "waiting") {
        if (!command.argument || command.id <= (pending.result.answer_id || pending.comment.id)) {
          throw new Error("Comment is not an eligible maintainer clarification answer");
        }
        Object.assign(data, { answer: { id: command.id, body: command.argument, author: command.author }, previous: pending.result });
      } else {
        data.command = command;
      }
    }
  }
  fs.mkdirSync(directory, { recursive: true });
  fs.writeFileSync(`${directory}/context.json`, JSON.stringify(data, null, 2));
  return data;
}

async function publishedResult(github, context, comment, phase) {
  if (!Number.isSafeInteger(Number(comment)) || Number(comment) <= 0) throw new Error("Worker did not publish a result comment");
  const { data: posted } = await github.rest.issues.getComment({ ...context.repo, comment_id: Number(comment) });
  const result = parseResult(posted);
  if (!result || result.phase !== phase || result.run.split("/")[0] !== String(context.runId)) {
    throw new Error("Worker result provenance does not match this phase and run");
  }
  const number = Number(posted.issue_url.split("/").pop());
  const { data: pr } = await github.rest.pulls.get({ ...context.repo, pull_number: number });
  if (!managedPR(pr, context.repo)) throw new Error("Worker result is not on a managed draft PR");
  return { posted, result, pr, current: await artifacts(github, context.repo, pr) };
}

async function verify({ github, context, comment, phase }) {
  const { result, current } = await publishedResult(github, context, comment, phase);
  if (phase === "clarify" && result.status === "waiting") {
    for (const name of ["spec.md"]) {
      if (result.files[`${result.feature}/${name}`] !== current.hashes[`${result.feature}/${name}`]) {
        throw new Error("Specification changed while questions were generated");
      }
    }
    return result;
  }
  // Blocking reanalysis findings are not a verification failure: the approvals job withdraws the approval
  // comment, the report lists the findings, and routing to implementation still requires a clean reanalysis.
  requireResult(result, phase === "clarify" ? ["spec.md"] : documents, current.hashes);
  if (phase === "implement" && /^\s*-\s+\[ \]\s+T\d+/m.test(current.files[`${current.feature}/tasks.md`])) {
    throw new Error("Implementation left unfinished tasks");
  }
  return result;
}

function quoted(text) {
  return text.replace(/@/g, "@\u200b").replace(/</g, "&lt;");
}

// Agent output reaches the PR through gh-aw safe outputs, which always append this footer. Only the
// trusted jobs below post without it, so an agent can never author an approval or status comment.
async function botComments(github, repo, number, matches) {
  const comments = await github.paginate(github.rest.issues.listComments, { ...repo, issue_number: number, per_page: 100 });
  return comments.filter((entry) => actionsBot(entry.user) && matches(entry.body || "") &&
    !(entry.body || "").includes("<!-- gh-aw-agentic-workflow:"));
}

async function publishApprovals({ github, context, comment }) {
  const { repo } = context;
  const { posted, result, pr, current } = await publishedResult(github, context, comment, "reanalyze");
  requireResult(result, documents, current.hashes);
  for (const stale of await botComments(github, repo, pr.number, (body) => approvalMarker.test(body))) {
    await github.rest.issues.deleteComment({ ...repo, comment_id: stale.id });
  }
  if (result.findings.some((finding) => blocking.includes(finding.severity))) return null;
  const items = (result.approvals || []).map((item) => ({ ...item, ...approvalLocation(item.location) }));
  const lengths = {};
  for (const item of items) {
    if (![`${current.feature}/`, "adrs/", "api/"].some((prefix) => item.path.startsWith(prefix))) {
      throw new Error(`Approval item must point into ${current.feature}/, adrs/, or api/: ${item.path}`);
    }
    lengths[item.path] ??= (await readBlob(github, repo, pr.head.sha, item.path)).text.split("\n").length;
    if (item.end > lengths[item.path]) throw new Error(`Approval item location is past the end of ${item.path}`);
  }
  const state = {
    reanalysis: posted.id, head: pr.head.sha, items: items.length,
    ...await fingerprint(github, repo, pr.head.sha, current.feature,
      items.map((item) => item.path).filter((name) => !name.startsWith(`${current.feature}/`))),
  };
  const commit = `\`${pr.head.sha.slice(0, 7)}\``;
  const lines = items.length ? [
    `React with 👍 to approve all ${items.length} item${items.length === 1 ? "" : "s"} below for commit ${commit}. ` +
      "Then comment `/speckit-implement`.",
    "",
    "| Item | Type | Decision | Details |",
    "| --- | --- | --- | --- |",
    ...items.map((item, index) => {
      const range = item.end === item.start ? `${item.start}` : `${item.start}-${item.end}`;
      const anchor = item.end === item.start ? `L${item.start}` : `L${item.start}-L${item.end}`;
      const url = `https://github.com/${repo.owner}/${repo.repo}/blob/${pr.head.sha}/${item.path}#${anchor}`;
      const title = quoted(item.title).replace(/[\\|]/g, "\\$&");
      return `| A${index + 1} | ${approvalKinds[item.kind]} | ${title} | [\`${item.path}:${range}\`](${url}) |`;
    }),
    "",
    "To change a decision instead, comment `/speckit-plan` with the change. " +
      "The pipeline updates every affected artifact and replaces this comment.",
  ] : [
    `No API change, ADR, or consequential decision needs approval for commit ${commit}. ` +
      "Comment `/speckit-implement` to start implementation.",
  ];
  const body = ["## Spec Kit approval", "", ...lines, "", `<!-- speckit-approval: ${JSON.stringify(state)} -->`, ""];
  const { data } = await github.rest.issues.createComment({ ...repo, issue_number: pr.number, body: body.join("\n") });
  return data;
}

async function approval(github, repo, pr, feature, reanalysis, entries) {
  const [published] = (await botComments(github, repo, pr.number, (body) => approvalMarker.test(body)))
    .sort((a, b) => b.id - a.id);
  if (!published) throw new Error("No approval comment exists yet. Comment `/speckit-plan` to publish one");
  let state;
  try {
    state = JSON.parse(published.body.match(approvalMarker)[1]);
  } catch {
    throw new Error(`The approval comment ${published.html_url} is malformed. Comment \`/speckit-plan\` to replace it`);
  }
  if (state.reanalysis !== reanalysis.id) {
    throw new Error(`The approval comment ${published.html_url} belongs to an earlier reanalysis. ` +
      "Comment `/speckit-plan` to replace it");
  }
  // Implementation ticks tasks and moves ADR drafts, so the reviewed state is only enforced until it starts.
  const started = entries.some((entry) => entry.result.phase === "implement" && entry.comment.id > published.id);
  const files = state.files && typeof state.files === "object" ? state.files : {};
  const current = started ? state : await fingerprint(github, repo, pr.head.sha, feature, Object.keys(files));
  if (current.tree !== state.tree || JSON.stringify(current.files) !== JSON.stringify(files)) {
    throw new Error(`Feature artifacts changed after the approval comment ${published.html_url}. ` +
      "Comment `/speckit-plan` to refresh it");
  }
  const record = { url: published.html_url, items: state.items, approvers: [] };
  if (!state.items) return record;
  const since = Date.parse(published.updated_at);
  const reactions = await github.paginate(github.rest.reactions.listForIssueComment,
    { ...repo, comment_id: published.id, per_page: 100 });
  for (const reaction of reactions) {
    if (!["+1", "-1"].includes(reaction.content) || reaction.user?.type !== "User" ||
        !(Date.parse(reaction.created_at) >= since) || !await maintainer(github, repo, reaction.user.login)) continue;
    if (reaction.content === "-1") {
      throw new Error(`Maintainer \`${reaction.user.login}\` reacted 👎 to ${published.html_url}. ` +
        "Comment `/speckit-plan` with the requested change");
    }
    record.approvers.push({ login: reaction.user.login, at: reaction.created_at });
  }
  if (!record.approvers.length) {
    throw new Error(`Approval pending: a maintainer must react with 👍 to ${published.html_url}`);
  }
  return record;
}

function runURL(repo, id) {
  return `https://github.com/${repo.owner}/${repo.repo}/actions/runs/${id}`;
}

function runOf(result) {
  return result.run.split("/")[0];
}

function cell(text) {
  return quoted(text).replace(/[\\|]/g, "\\$&");
}

function outcome(result) {
  if (result.status === "blocked") return "⛔";
  if (result.status === "waiting") return "⏸";
  if (["analyze", "reanalyze"].includes(result.phase) &&
      result.findings.some((finding) => blocking.includes(finding.severity))) return "⚠️";
  return "✅";
}

function severities(findings) {
  const counts = ["CRITICAL", "HIGH", "MEDIUM", "LOW"]
    .map((severity) => [severity, findings.filter((finding) => finding.severity === severity).length])
    .filter(([, count]) => count);
  return counts.length ? counts.map(([severity, count]) => `${count} ${severity}`).join(" · ") : "no findings";
}

// A rerun of an earlier phase supersedes every later result, so only results newer than their predecessor count.
function current(entries) {
  const stages = {};
  let floor = 0;
  for (const phase of phases) {
    const entry = latest(entries, [phase]);
    if (entry && entry.comment.id > floor) {
      stages[phase] = entry;
      floor = entry.comment.id;
    }
  }
  return stages;
}

function progress(stages, { active, approve }) {
  const reached = active ? phases.indexOf(active) : -1;
  const steps = [["Specify", "✅"]];
  for (const phase of phases) {
    if (phase === "implement") steps.push(["Approve", approve || "○"]);
    const icon = phase === active ? "⏳" : reached >= 0 && phases.indexOf(phase) > reached ? "○" :
      stages[phase] ? outcome(stages[phase].result) : "○";
    steps.push([labels[phase], icon]);
  }
  const focus = active ? labels[active] : approve && approve !== "○" && !stages.implement ? "Approve" :
    labels[phases.filter((phase) => stages[phase]).at(-1)];
  return steps.map(([name, icon]) => name === focus ? `**${name} ${icon}**` : `${name} ${icon}`).join(" › ");
}

// gh-aw appends this footer to every agent comment, including its "Commit pushed" notices.
function agentComments(comments, repo, run) {
  const escape = (text) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const footer = new RegExp(`^> Generated by \\[[^\\]\\n]+\\]\\(${escape(runURL(repo, run))}\\)`, "m");
  return comments.filter((entry) => actionsBot(entry.user) && footer.test(entry.body || "") &&
    !(entry.body || "").includes(statusMarker) && !approvalMarker.test(entry.body || ""));
}

function runTable(entries, comments, repo, run) {
  const prefix = `https://github.com/${repo.owner}/${repo.repo}/commit/`;
  const commits = agentComments(comments, repo, run).map((entry) => {
    const match = (entry.body || "").match(commitNotice);
    return match?.[2].startsWith(prefix) ? { id: entry.id, sha: match[1], url: match[2] } : null;
  }).filter(Boolean);
  const own = entries.filter((entry) => runOf(entry.result) === run).sort((a, b) => a.comment.id - b.comment.id);
  if (!own.length) return [];
  let previous = 0;
  const rows = own.map(({ comment, result }) => {
    const pushed = commits.filter((entry) => entry.id > previous && entry.id < comment.id).at(-1);
    previous = comment.id;
    const state = ["analyze", "reanalyze"].includes(result.phase) ?
      `${outcome(result)} ${severities(result.findings)}` : `${outcome(result)} ${result.status}`;
    return `| ${labels[result.phase]} | ${state} | ${result.summary ? cell(result.summary) : "—"} | ` +
      `${pushed ? `[\`${pushed.sha.slice(0, 7)}\`](${pushed.url})` : "—"} | [report](${comment.html_url}) |`;
  });
  return ["### This run", "", "| Phase | Result | Summary | Commit | Details |", "| --- | --- | --- | --- | --- |", ...rows];
}

function earlierRuns(entries, repo, run) {
  const runs = new Map();
  for (const entry of [...entries].sort((a, b) => a.comment.id - b.comment.id)) {
    if (runOf(entry.result) !== run) runs.set(runOf(entry.result), [...(runs.get(runOf(entry.result)) || []), entry]);
  }
  const lines = [...runs].reverse().slice(0, 10).map(([id, list]) => `- [Run ${id}](${runURL(repo, id)}): ` +
    list.map(({ comment, result }) => `[${labels[result.phase]} ${outcome(result)}](${comment.html_url})`).join(" · "));
  return lines.length ? ["<details><summary>Earlier runs</summary>", "", ...lines, "", "</details>"] : [];
}

async function pullRequestState(github, repo, number) {
  let pr;
  try {
    ({ data: pr } = await github.rest.pulls.get({ ...repo, pull_number: number }));
  } catch (error) {
    if (error.status === 404) return null;
    throw error;
  }
  if (!managedPR(pr, repo)) return null;
  const comments = await github.paginate(github.rest.issues.listComments, { ...repo, issue_number: number, per_page: 100 });
  const entries = comments.map((comment) => ({ comment, result: parseResult(comment) })).filter((entry) => entry.result);
  return { pr, comments, entries, stages: current(entries) };
}

async function publishStatus(github, repo, number, title, sections) {
  for (const stale of await botComments(github, repo, number, (body) => body.includes(statusMarker))) {
    await github.rest.issues.deleteComment({ ...repo, comment_id: stale.id });
  }
  const body = [`## Spec Kit · ${title}`, "", ...sections.flatMap((section) => section.length ? [...section, ""] : []),
    statusMarker, ""];
  const { data } = await github.rest.issues.createComment({ ...repo, issue_number: number, body: body.join("\n") });
  return data;
}

// The status comment replaces itself on every run, so the PR always ends with one summary of the phase and the next step.
async function announce({ github, context, needs }) {
  const { repo } = context;
  const created = Number(needs.specify?.outputs?.created_pr_number);
  const number = created || Number(needs.route?.outputs?.pr);
  const action = created ? "clarify" : needs.route?.outputs?.action;
  if (!running[action] || !number || !await maintainer(github, repo, context.actor)) return null;
  const state = await pullRequestState(github, repo, number);
  const approve = action === "implement" ? "✅" : "○";
  // Runs queue per triggering issue or PR number, so only a run started on this PR holds back its later commands.
  const queued = [context.payload.pull_request?.number, context.payload.issue?.number].includes(number);
  return publishStatus(github, repo, number, `⏳ ${running[action]}`, [
    state ? [progress(state.stages, { active: action, approve })] : [], [
    `Started by \`${context.actor}\`. Follow the [workflow run](${runURL(repo, context.runId)}).` +
      (queued ? " Commands posted now wait until this run finishes." : ""),
  ]]);
}

async function readiness(github, repo, state, stage) {
  try {
    const { feature } = await artifacts(github, repo, state.pr);
    const record = await approval(github, repo, state.pr, feature, stage.comment, state.entries);
    const approvers = record.approvers.map((entry) => `\`${entry.login}\``).join(", ");
    return {
      icon: "✅", title: "✅ Ready to implement",
      lines: [`${record.items ? `Approved by ${approvers}. ` : "Nothing needs sign-off. "}` +
        "Comment `/speckit-implement` to start implementation."],
    };
  } catch (error) {
    return {
      icon: "🖐", title: "🖐 Waiting for sign-off",
      lines: [quoted(error.message), "", "Then comment `/speckit-implement`."],
    };
  }
}

async function report({ github, context, needs }) {
  const { repo } = context;
  const route = needs.route || {};
  if (route.result === "success" && route.outputs?.action === "noop") return null;
  if (!await maintainer(github, repo, context.actor)) return null;
  const number = Number(needs.specify?.outputs?.created_pr_number) || Number(route.outputs?.pr) ||
    context.payload.pull_request?.number || context.payload.issue?.number;
  if (!number) return null;
  const run = String(context.runId);
  const state = await pullRequestState(github, repo, number);
  const entries = state?.entries || [];
  const [last] = entries.filter((entry) => runOf(entry.result) === run).sort((a, b) => b.comment.id - a.comment.id);
  const jobs = Object.entries(needs).filter(([name]) => pipeline.includes(name))
    .sort(([a], [b]) => pipeline.indexOf(a) - pipeline.indexOf(b));
  const error = jobs.find(([, job]) => job.result === "failure" && job.outputs?.error);
  const failed = jobs.find(([, job]) => job.result === "failure");
  const stages = state?.stages || {};
  const tip = stages[phases.filter((phase) => stages[phase]).at(-1)];
  // A failed job without its own result must not be explained by a blocker from an earlier run.
  const stopped = [last, failed ? null : tip].find((entry) => entry?.result.status === "blocked");
  const reanalysis = stages.reanalyze && !stages.implement ? stages.reanalyze : null;
  const findings = reanalysis ? reanalysis.result.findings.filter((finding) => blocking.includes(finding.severity)) : [];
  const details = (entry) => `Details: [${entry.result.phase} report](${entry.comment.html_url})`;
  let title, lines, approve;
  if (error) {
    title = `⛔ Stopped in \`${error[0]}\``;
    lines = [quoted(error[1].outputs.error)];
  } else if (stopped) {
    title = `⛔ ${labels[stopped.result.phase]} is blocked`;
    lines = [
      quoted(stopped.result.next_step || "Read the phase report."), "",
      stopped.result.phase === "implement" ? "Then comment `/speckit-implement` to resume." :
        "Then comment `/speckit-plan <decision>`, or `/speckit-clarify <change>` to change the specification.", "",
      details(stopped),
    ];
  } else if (failed) {
    title = `⛔ \`${failed[0]}\` failed`;
    lines = ["The job failed before publishing a result. Check the workflow run, then repeat the command."];
  } else if (findings.length) {
    title = `⚠️ ${findings.length} blocking finding${findings.length === 1 ? "" : "s"}`;
    lines = [
      "**Implementation is blocked:** reanalysis still reports these findings.", "",
      ...findings.slice(0, 10).map((finding) =>
        `- **${finding.severity}** \`${quoted(finding.location)}\`: ${quoted(finding.summary)}`),
      "", "Comment `/speckit-plan` to remediate them. Add guidance after the command to steer the fix.", "",
      details(reanalysis),
    ];
  } else if (stages.implement?.result.status === "complete") {
    title = "✅ Implementation complete";
    approve = "✅";
    lines = ["Review the changes, then mark the pull request ready for review.", "", details(stages.implement)];
  } else if (reanalysis) {
    ({ title, lines, icon: approve } = await readiness(github, repo, state, reanalysis));
  } else if (stages.clarify?.result.status === "waiting") {
    title = "⏸ Waiting for your answers";
    lines = ["Answer the questions with `/speckit-clarify <answers>`.", "", details(stages.clarify)];
  } else if (stages.clarify && !stages.plan) {
    title = "👀 Specification ready for review";
    lines = [
      `Review \`${stages.clarify.result.feature}/spec.md\`, then comment \`/speckit-plan [guidance]\` to plan, ` +
        "or `/speckit-clarify <change>` to revise the specification.", "", details(stages.clarify),
    ];
  } else {
    title = "Waiting for a command";
    lines = ["Comment `/speckit-plan [guidance]` to continue the pipeline."];
  }
  const data = await publishStatus(github, repo, number, title, [
    state ? [progress(stages, { approve })] : [],
    ["### Next step", "", ...lines],
    runTable(entries, state?.comments || [], repo, run),
    earlierRuns(entries, repo, run),
    [`[Workflow run](${runURL(repo, run)})`],
  ]);
  // The status comment now summarizes this run, so its agent comments are folded but stay readable and parseable.
  const unfolded = [];
  for (const entry of agentComments(state?.comments || [], repo, run)) {
    try {
      await github.graphql(
        "mutation($id: ID!) { minimizeComment(input: {subjectId: $id, classifier: OUTDATED}) { clientMutationId } }",
        { id: entry.node_id },
      );
    } catch (error) {
      unfolded.push(`${entry.html_url}: ${error.message}`);
    }
  }
  if (unfolded.length) throw new Error(`Status published, but these agent comments stay unfolded: ${unfolded.join("; ")}`);
  return data;
}

function localFile(name) {
  let current = process.cwd();
  for (const component of name.split("/")) {
    current = path.join(current, component);
    if (fs.lstatSync(current).isSymbolicLink()) throw new Error(`Symlink is not an artifact: ${name}`);
  }
  return fs.readFileSync(current, "utf8");
}

function writeResult() {
  const context = JSON.parse(fs.readFileSync(`${directory}/context.json`, "utf8"));
  const draft = JSON.parse(fs.readFileSync(`${directory}/result.json`, "utf8"));
  const feature = featurePath(JSON.parse(localFile(".specify/feature.json")).feature_directory);
  if (feature !== context.feature) throw new Error("Worker changed the active feature");
  const files = {};
  for (const name of documents) {
    const file = `${feature}/${name}`;
    if (fs.existsSync(file)) files[file] = localFile(file);
  }
  if (!files[`${feature}/spec.md`]) throw new Error("Feature specification is missing");
  if (context.phase === "implement" && draft.status === "complete") {
    if (/^\s*-\s+\[ \]\s+T\d+/m.test(files[`${feature}/tasks.md`] || "") ||
        !draft.checks?.includes("just check") ||
        !draft.checks.some((command) => /^just (test[\w-]*|web-test|cdk-test|verify)(\s|$)/.test(command))) {
      throw new Error("Implementation requires completed tasks, just check, and matching test suites");
    }
  }
  if (draft.status === "blocked" && (typeof draft.next_step !== "string" || !draft.next_step.trim())) {
    throw new Error("A blocked result must state the maintainer's next step");
  }
  // The status comment shows the next step verbatim, so it must stay a short list a maintainer can act on.
  const steps = draft.status === "blocked" ? draft.next_step.trim().split("\n") : [];
  if (steps.length > 5 || steps.some((step) => !/^\d+\. \S/.test(step) || step.length > 300)) {
    throw new Error("The next step must be a numbered list of at most 5 actions, one line of at most 300 characters each");
  }
  const analysis = ["analyze", "reanalyze"].includes(context.phase);
  if (analysis && !Array.isArray(draft.approvals)) {
    throw new Error("An analysis must list its approval items, even when there are none");
  }
  const approvals = analysis ? draft.approvals.map((item) => ({
    kind: item?.kind, title: item?.title, location: item?.location,
  })) : [];
  for (const item of approvals) {
    const { path: name, end } = approvalLocation(item.location);
    if (!fs.existsSync(name) || localFile(name).split("\n").length < end) {
      throw new Error(`Approval item location does not exist: ${item.location}`);
    }
  }
  if (typeof draft.summary !== "string" || !draft.summary.trim()) throw new Error("A one-line phase summary is required");
  const result = validateResult({
    phase: context.phase, run: context.run, feature, files: artifactHashes(files),
    status: draft.status, findings: draft.findings, questions: draft.questions,
    answer_id: context.answer?.id || context.previous?.answer_id || 0,
    summary: draft.summary.trim(),
    ...(analysis ? { approvals } : {}),
    ...(draft.status === "blocked" ? { next_step: draft.next_step } : {}),
  });
  if (typeof draft.report !== "string" || !draft.report.trim()) throw new Error("A phase report is required");
  if ([draft.report, result.summary].some((text) => text.includes(marker))) {
    throw new Error("A phase report must not contain a result marker");
  }
  // The status comment carries the outcome; the full report and its result stay folded for reference.
  fs.writeFileSync(`${directory}/comment.md`,
    [
      `**Spec Kit ${context.phase}: ${result.status}.** ${result.summary}`, "",
      `<details><summary>Full ${context.phase} report</summary>`, "", draft.report, "",
      marker, "```json", JSON.stringify(result, null, 2), "```", "", "</details>", "",
    ].join("\n"));
}

function selectFeature() {
  const context = JSON.parse(fs.readFileSync(`${directory}/context.json`, "utf8"));
  const feature = featurePath(context.feature);
  localFile(`${feature}/spec.md`);
  const { O_WRONLY, O_CREAT, O_NOFOLLOW } = fs.constants;
  if (!O_NOFOLLOW) throw new Error("Symlink-safe feature pointer writes are unsupported");
  if (fs.lstatSync(".specify").isSymbolicLink()) throw new Error("Feature pointer must not be a symlink");
  let pointer;
  try {
    pointer = fs.openSync(".specify/feature.json", O_WRONLY | O_CREAT | O_NOFOLLOW);
  } catch (error) {
    if (error.code === "ELOOP") throw new Error("Feature pointer must not be a symlink");
    throw error;
  }
  try {
    if (!fs.fstatSync(pointer).isFile()) throw new Error("Feature pointer must be a regular file");
    fs.ftruncateSync(pointer);
    fs.writeFileSync(pointer, `${JSON.stringify({ feature_directory: feature })}\n`);
  } finally {
    fs.closeSync(pointer);
  }
}

module.exports = {
  artifactHashes, parseCommand, parseResult, route, requireResult, prepare, verify, publishApprovals, announce, report,
  continuesToPlan,
};

if (require.main === module) {
  if (process.argv[2] === "select-feature") {
    selectFeature();
  } else if (process.argv[2] === "result") {
    writeResult();
  } else {
    throw new Error("Expected result or select-feature command");
  }
}
