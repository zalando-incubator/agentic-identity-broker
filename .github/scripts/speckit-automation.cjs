const { createHash } = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");

const phases = ["clarify", "plan", "tasks", "analyze", "remediate", "reanalyze", "implement"];
const documents = ["spec.md", "plan.md", "tasks.md"];
const marker = "## Spec Kit result";
const directory = "/tmp/gh-aw/speckit";

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
  return result;
}

function actionsBot(user) {
  return user?.login === "github-actions[bot]" && user?.type === "Bot";
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

function requireResult(result, required, hashes, { clean = false, run } = {}) {
  if (!result) throw new Error("Required Spec Kit result is missing (clarification or previous phase)");
  if (result.status !== "complete") throw new Error(`Spec Kit ${result.phase} is ${result.status}`);
  if (run && result.run.split("/")[0] !== run.split("/")[0]) throw new Error("Previous phase result belongs to a different run");
  for (const name of required) {
    const file = `${result.feature}/${name}`;
    if (!hashes[file] || result.files[file] !== hashes[file]) throw new Error(`${file} changed after ${result.phase}`);
  }
  if (clean && result.findings.some((finding) => ["CRITICAL", "HIGH"].includes(finding.severity))) {
    throw new Error("Unresolved CRITICAL/HIGH findings block implementation");
  }
}

async function maintainer(github, repo, login) {
  const { data } = await github.rest.repos.getCollaboratorPermissionLevel({ ...repo, username: login });
  return ["admin", "maintain", "write"].includes(data.permission);
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

async function readFile(github, repo, ref, name, optional = false) {
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
  return Buffer.from(data.content, "base64").toString("utf8");
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

async function route({ github, context }) {
  const { payload, repo, eventName } = context;
  const label = payload.label?.name;
  const isIssue = eventName === "issues" && payload.action === "labeled" &&
    label === "speckit:specify" && !payload.issue?.pull_request;
  const isLabel = eventName === "pull_request_target" && payload.action === "labeled" &&
    ["speckit:plan-execution", "speckit:implement"].includes(label);
  const isAnswer = eventName === "issue_comment" && payload.action === "created" &&
    payload.issue?.pull_request && payload.comment?.user?.type === "User";
  if ((!isIssue && !isLabel && !isAnswer) ||
      !await maintainer(github, repo, context.actor)) return { action: "noop" };
  if (isIssue) {
    if (payload.issue.state !== "open") return { action: "noop" };
    const branch = `speckit/issue-${payload.issue.number}`;
    const existing = await github.paginate(github.rest.pulls.list, {
      ...repo, state: "all", head: `${repo.owner}:${branch}`, per_page: 100,
    });
    if (existing.length) {
      const managed = existing.find((pr) => managedPR(pr, repo) === payload.issue.number);
      if (managed) {
        if (latest(await reports(github, repo, managed.number), ["clarify"])) return { action: "noop" };
        return { action: "clarify", issue: payload.issue.number, pr: managed.number, comment: 0 };
      }
      if (existing.some((pr) => pr.merged_at ||
          managedPR(pr, repo, "closed") !== payload.issue.number)) return { action: "noop" };
    }
    try {
      await github.rest.repos.getBranch({ ...repo, branch });
      return { action: "noop" };
    } catch (error) {
      if (error.status !== 404) throw error;
    }
    return { action: "specify", issue: payload.issue.number, pr: 0, comment: 0 };
  }
  const number = isAnswer ? payload.issue.number : payload.pull_request.number;
  const { data: pr } = await github.rest.pulls.get({ ...repo, pull_number: number });
  const issue = managedPR(pr, repo);
  if (!issue) return { action: "noop" };
  const entries = await reports(github, repo, number);
  if (isAnswer) {
    const pending = latest(entries, ["clarify"]);
    if (!pending || pending.result.status !== "waiting" ||
        payload.comment.id <= (pending.result.answer_id || pending.comment.id) ||
        !payload.comment.body?.trim() || payload.comment.user.login !== context.actor) return { action: "noop" };
    return { action: "clarify", issue, pr: number, comment: payload.comment.id };
  }
  const current = await artifacts(github, repo, pr);
  requireResult(latest(entries, ["clarify"])?.result, ["spec.md"], current.hashes);
  if (label === "speckit:implement") {
    const analysis = latest(entries, ["analyze", "reanalyze"])?.result;
    if (analysis?.phase !== "reanalyze") throw new Error("Independent reanalysis is required before implementation");
    requireResult(analysis, documents, current.hashes, { clean: true });
  }
  return { action: label === "speckit:implement" ? "implement" : "plan", issue, pr: number, comment: 0 };
}

async function prepare({ github, context, inputs }) {
  if (!await maintainer(github, context.repo, context.actor)) throw new Error("Maintainer permission is required");
  const run = `${context.runId}/${process.env.GITHUB_RUN_ATTEMPT}`;
  const data = { phase: inputs.phase, run, repository: `${context.repo.owner}/${context.repo.repo}` };
  if (inputs.phase === "specify") {
    const { data: issue } = await github.rest.issues.get({ ...context.repo, issue_number: Number(inputs.issue) });
    if (issue.pull_request || issue.state !== "open" ||
        !issue.labels.some((label) => label.name === "speckit:specify")) throw new Error("Issue is not eligible for specification");
    Object.assign(data, { issue: issue.number, title: issue.title, body: issue.body || "", branch: `speckit/issue-${issue.number}` });
  } else {
    if (!phases.includes(inputs.phase)) throw new Error("Unsupported Spec Kit phase");
    const { data: pr } = await github.rest.pulls.get({ ...context.repo, pull_number: Number(inputs.pr) });
    if (!managedPR(pr, context.repo)) throw new Error("Only managed, same-repository draft PRs are eligible");
    const current = await artifacts(github, context.repo, pr);
    const entries = await reports(github, context.repo, pr.number);
    const previous = {
      plan: ["clarify"], tasks: ["plan"], analyze: ["tasks"],
      remediate: ["analyze"], reanalyze: ["remediate"], implement: ["analyze", "reanalyze"],
    }[inputs.phase];
    if (previous) {
      data.previous = latest(entries, previous)?.result;
      if (inputs.phase === "implement" && data.previous?.phase !== "reanalyze") {
        throw new Error("Independent reanalysis is required before implementation");
      }
      const required = inputs.phase === "plan" ? ["spec.md"] : inputs.phase === "tasks" ? ["spec.md", "plan.md"] : documents;
      requireResult(data.previous, required, current.hashes, {
        clean: inputs.phase === "implement",
        run: ["tasks", "analyze", "remediate", "reanalyze"].includes(inputs.phase) ? run : undefined,
      });
    }
    Object.assign(data, { pr: pr.number, head: pr.head.sha, branch: pr.head.ref, feature: current.feature });
    if (Number(inputs.comment)) {
      if (inputs.phase !== "clarify") throw new Error("Only clarification accepts comment answers");
      const { data: answer } = await github.rest.issues.getComment({ ...context.repo, comment_id: Number(inputs.comment) });
      const pending = latest(entries, ["clarify"]);
      if (!answer.issue_url.endsWith(`/issues/${pr.number}`) || answer.user.type !== "User" ||
          !await maintainer(github, context.repo, answer.user.login) ||
          !pending || pending.result.status !== "waiting" ||
          answer.id <= (pending.result.answer_id || pending.comment.id)) {
        throw new Error("Comment is not an eligible maintainer clarification answer");
      }
      Object.assign(data, { answer: { id: answer.id, body: answer.body, author: answer.user.login }, previous: pending.result });
    }
  }
  fs.mkdirSync(directory, { recursive: true });
  fs.writeFileSync(`${directory}/context.json`, JSON.stringify(data, null, 2));
  return data;
}

async function verify({ github, context, comment, phase }) {
  if (!Number.isSafeInteger(Number(comment)) || Number(comment) <= 0) throw new Error("Worker did not publish a result comment");
  const { data: posted } = await github.rest.issues.getComment({ ...context.repo, comment_id: Number(comment) });
  const result = parseResult(posted);
  if (!result || result.phase !== phase || result.run.split("/")[0] !== String(context.runId)) {
    throw new Error("Worker result provenance does not match this phase and run");
  }
  const number = Number(posted.issue_url.split("/").pop());
  const { data: pr } = await github.rest.pulls.get({ ...context.repo, pull_number: number });
  if (!managedPR(pr, context.repo)) throw new Error("Worker result is not on a managed draft PR");
  const current = await artifacts(github, context.repo, pr);
  if (phase === "clarify" && result.status === "waiting") {
    for (const name of ["spec.md"]) {
      if (result.files[`${result.feature}/${name}`] !== current.hashes[`${result.feature}/${name}`]) {
        throw new Error("Specification changed while questions were generated");
      }
    }
    return result;
  }
  requireResult(result, phase === "clarify" ? ["spec.md"] : documents, current.hashes, { clean: phase === "reanalyze" });
  if (phase === "implement" && /^\s*-\s+\[ \]\s+T\d+/m.test(current.files[`${current.feature}/tasks.md`])) {
    throw new Error("Implementation left unfinished tasks");
  }
  return result;
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
  const result = validateResult({
    phase: context.phase, run: context.run, feature, files: artifactHashes(files),
    status: draft.status, findings: draft.findings, questions: draft.questions,
    answer_id: context.answer?.id || context.previous?.answer_id || 0,
  });
  if (typeof draft.report !== "string" || !draft.report.trim()) throw new Error("A phase report is required");
  if (draft.report.includes(marker)) throw new Error("A phase report must not contain a result marker");
  fs.writeFileSync(`${directory}/comment.md`,
    `${draft.report}\n\n${marker}\n\`\`\`json\n${JSON.stringify(result, null, 2)}\n\`\`\`\n`);
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

module.exports = { artifactHashes, parseResult, route, requireResult, prepare, verify };

if (require.main === module) {
  if (process.argv[2] === "select-feature") {
    selectFeature();
  } else if (process.argv[2] === "result") {
    writeResult();
  } else {
    throw new Error("Expected result or select-feature command");
  }
}
