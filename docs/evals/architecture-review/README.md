# Architecture review evaluations

- [`RESULTS.md`](RESULTS.md): the evaluation of the `behavioral-probe` dimension against the PR #293 human review findings. It covers scores, the changelog, overlap with existing dimensions and the recommendation.
- [`behavioral-probe-dimension.md`](behavioral-probe-dimension.md): the final dimension section, identical to the ninth agent in `.github/workflows/architecture-review.md`.
- `prompts/`: the evaluated prompt versions (`v1`, `v2`, `v2-noext` ablation, `v3`).
- `harness/`: `make-brief.sh` builds the orchestrator-style brief for an isolated clone. `render-task.sh` stages the prompt and prints the sub-agent task. `task-template.md` documents the task message.
- `runs/`: the raw output of every blind run (`<version>/<commit>-r<n>.md`), the judge notes, and `cost.tsv` (version, commit, run, sub-agent tokens, tool calls, seconds).
