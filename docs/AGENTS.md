# Documentation (`docs/`)

Read this file before you add or change text in `docs/`, in the descriptions of `api/*/openapi.yaml`, or in `ARCHITECTURE.md`. Apply these rules to the text that you add or change. Use the `simple-english` skill in pragmatic mode.

`docs/` is the source of the public site. `assets/docusaurus/sidebars.js` sets the reading order. `assets/docusaurus/docusaurus.config.js` lists the maintainer files that the site excludes.

## Readers

| Path | Reader | Content |
|---|---|---|
| `introduction/`, `concepts/` | Evaluator, new user | What the broker is and how it works |
| `get-started/` | New user | A first working setup |
| `guides/`, `deployment/` | Operator, integrator | How to do one task |
| `operations/` | Operator | Upgrade and maintenance procedures |
| `reference/`, `api/`, `configuration.md` | Integrator, operator | Exact contracts and settings |

Write for the reader of that path. The reader cannot see the pull request, the specification, or the review.

## Content Rules

1. Describe the behavior after the change merges. Do not write implementation progress, test status, approval status, or release status. Put them in the pull request or the phase report.
2. Do not refer to pull requests, tasks, review findings, or specification IDs. Examples: "this PR", `feature 055`, `FR-012`, `US1.4`, `API-ORDER`, `F3`.
3. Describe what the reader observes, not how the code does it. Write "sorted by byte value", not "sorted in Go string byte order".
4. Put each fact on one page. Link to that page from other pages. An ADR holds the decision, `ARCHITECTURE.md` holds the design, and a guide holds the task.
5. Introduce each example value before you use it. Do not copy fixture names from a specification.
6. Make each command run as written, except for placeholders that the text defines. Do not use `...` in a command.
7. Write an upgrade procedure for a rolling deployment. The chart's migration Job runs before the upgrade, while old replicas still serve. If a step needs downtime, say why.
8. Put consumer steps for a breaking API change in the guide for that resource. Put operator upgrade steps in `operations/`.
9. Add each new public page to `assets/docusaurus/sidebars.js`.
