const fs = require("node:fs");

const label = "auto-resolve-conflicts";
const directory = "/tmp/gh-aw/pr-conflicts";
const shaPattern = /^[a-f0-9]{40}$/i;

const prQuery = `query ConflictPR($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      number state mergeable headRefName headRefOid baseRefName baseRefOid
      headRef { name }
      baseRef { name }
      headRepository { nameWithOwner }
      baseRepository { nameWithOwner }
      labels(first: 100) { nodes { name } pageInfo { hasNextPage } }
    }
  }
}`;

const historyQuery = `query ConflictLabelHistory($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      timelineItems(itemTypes: [LABELED_EVENT, UNLABELED_EVENT], last: 100, before: $cursor) {
        nodes {
          __typename
          ... on LabeledEvent { label { name } actor { __typename login } }
          ... on UnlabeledEvent { label { name } }
        }
        pageInfo { hasPreviousPage startCursor }
      }
    }
  }
}`;

function scanQuery(mainOnly) {
  return `query ConflictScan($owner: String!, $repo: String!, $cursor: String) {
    repository(owner: $owner, name: $repo) {
      pullRequests(states: OPEN, ${mainOnly ? 'baseRefName: "main", ' : ""}labels: ["auto-resolve-conflicts"], first: 100, after: $cursor) {
        nodes { number }
        pageInfo { hasNextPage endCursor }
      }
    }
  }`;
}

async function labelerAuthorized(github, repo, number) {
  let cursor = null;
  while (true) {
    const response = await github.graphql(historyQuery, { ...repo, number, cursor });
    const history = response.repository?.pullRequest?.timelineItems;
    if (!history?.nodes || !history.pageInfo) throw new Error(`Missing label history for PR #${number}`);
    for (let index = history.nodes.length - 1; index >= 0; index--) {
      const event = history.nodes[index];
      if (event.label?.name !== label) continue;
      if (event.__typename !== "LabeledEvent" || event.actor?.__typename !== "User" || !event.actor.login) {
        return false;
      }
      try {
        const { data } = await github.rest.repos.getCollaboratorPermissionLevel({ ...repo, username: event.actor.login });
        return ["admin", "maintain", "write"].includes(data.permission);
      } catch (error) {
        if (error.status === 404) return false;
        throw error;
      }
    }
    if (!history.pageInfo.hasPreviousPage) return false;
    if (!history.pageInfo.startCursor || history.pageInfo.startCursor === cursor) {
      throw new Error(`Invalid label history cursor for PR #${number}`);
    }
    cursor = history.pageInfo.startCursor;
  }
}

async function eligibility(github, repo, number) {
  const response = await github.graphql(prQuery, { ...repo, number });
  const pr = response.repository?.pullRequest;
  if (!pr || pr.state !== "OPEN") return { reason: "PR is not open" };
  if (!pr.labels?.nodes || pr.labels.pageInfo?.hasNextPage) {
    throw new Error(`Incomplete labels for PR #${number}`);
  }
  if (!pr.labels.nodes.some(({ name }) => name === label)) return { reason: "Opt-in label was removed" };
  const repository = `${repo.owner}/${repo.repo}`.toLowerCase();
  if (pr.headRepository?.nameWithOwner?.toLowerCase() !== repository ||
      pr.baseRepository?.nameWithOwner?.toLowerCase() !== repository) {
    return { reason: "PR head or base is outside this repository" };
  }
  if (!pr.baseRef || !pr.baseRefName || pr.baseRef.name !== pr.baseRefName ||
      !shaPattern.test(pr.baseRefOid || "")) return { reason: "PR base ref is unavailable" };
  if (!pr.headRef || !pr.headRefName || pr.headRef.name !== pr.headRefName ||
      !shaPattern.test(pr.headRefOid || "")) return { reason: "PR head ref is unavailable" };
  if (pr.mergeable !== "CONFLICTING") return { reason: `PR conflict status is ${pr.mergeable}` };
  if (!await labelerAuthorized(github, repo, number)) return { reason: "Opt-in label lacks authorized human provenance" };
  return { pr };
}

async function selectConflicts({ github, context }) {
  const { eventName, payload, repo } = context;
  let numbers;
  if (eventName === "pull_request_target") {
    if (!["labeled", "synchronize", "reopened", "edited"].includes(payload.action) ||
        (payload.action === "labeled" && payload.label?.name !== label) ||
        !Number.isSafeInteger(payload.pull_request?.number)) return [];
    numbers = [payload.pull_request.number];
  } else if (eventName === "push" || eventName === "schedule") {
    if (eventName === "push" && payload.ref !== "refs/heads/main") return [];
    numbers = [];
    let cursor = null;
    const query = scanQuery(eventName === "push");
    while (true) {
      const response = await github.graphql(query, { ...repo, cursor });
      const result = response.repository?.pullRequests;
      if (!result?.nodes || !result.pageInfo) throw new Error("Missing PR scan results");
      numbers.push(...result.nodes.map(({ number }) => number));
      if (!result.pageInfo.hasNextPage) break;
      if (!result.pageInfo.endCursor || result.pageInfo.endCursor === cursor) throw new Error("Invalid PR scan cursor");
      cursor = result.pageInfo.endCursor;
    }
  } else return [];

  const candidates = [];
  for (const number of new Set(numbers)) {
    if (!Number.isSafeInteger(number) || number <= 0) throw new Error("Invalid PR number in scan");
    const { pr } = await eligibility(github, repo, number);
    if (!pr || (eventName === "push" || eventName === "pull_request_target") && pr.baseRefName !== "main") continue;
    candidates.push({ number: pr.number, head: pr.headRefOid, base: pr.baseRefName });
    if (candidates.length > 256) throw new Error("More than 256 conflicted PRs qualify for the Actions matrix");
  }
  return candidates.sort((a, b) => a.number - b.number);
}

async function prepare({ github, context, inputs }) {
  const number = Number(inputs.pr);
  if (!Number.isSafeInteger(number) || number <= 0) throw new Error("Invalid selected PR number");
  const { pr, reason } = await eligibility(github, context.repo, number);
  let data;
  if (!pr) data = { eligible: false, reason };
  else if (inputs.head !== pr.headRefOid) data = { eligible: false, reason: "Selected PR head changed" };
  else if (inputs.base !== pr.baseRefName) data = { eligible: false, reason: "Selected PR base branch changed" };
  else data = {
    eligible: true, pr: number, head_ref: pr.headRefName, head_sha: pr.headRefOid,
    base_ref: pr.baseRefName, base_sha: pr.baseRefOid,
  };
  fs.mkdirSync(directory, { recursive: true });
  fs.writeFileSync(`${directory}/context.json`, JSON.stringify(data));
  return data;
}

module.exports = { selectConflicts, prepare };
