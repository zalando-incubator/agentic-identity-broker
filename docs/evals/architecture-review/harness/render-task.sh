#!/usr/bin/env bash
# Prints the task for one blind run of a review dimension. The dimension section is
# extracted from the workflow into the clone, so the sub-agent never opens the main
# checkout.
# Usage: render-task.sh <dimension-name> <clone-dir> <out-file>
set -euo pipefail
name=$1 clone=$2 out=$3
workflow="$(git rev-parse --show-toplevel)/.github/workflows/architecture-review.md"
mkdir -p "$clone/.git/probe-tmp" "$(dirname "$out")"
awk -v start="## agent: \`$name\`" '
  $0 == start { on = 1; print; next }
  on && /^## agent: / { exit }
  on { print }
' "$workflow" > "$clone/.git/agent-definition.md"
[ -s "$clone/.git/agent-definition.md" ] || { echo "dimension $name not found in $workflow" >&2; exit 1; }
cat <<MSG
You are a sub-agent of an automated architecture review. Your agent definition
is the file \`$clone/.git/agent-definition.md\`. Read it in full first and
follow it exactly.

Task from the lead reviewer:
- Brief: \`$clone/brief.md\`
- Head and base SHAs are in the brief (\`pr-head\` is checked out, \`base\` is a local branch)
- Maintainer focus: none
- Follow your own dimension instructions.

Sandbox rules for this run: work only inside \`$clone\` (use it as the working
directory for every command). Do not read any other checkout or directory on
this machine, do not use the network, GitHub, or \`gh\`, and do not modify
tracked files. Temporary files go under \`$clone/.git/probe-tmp/\`.

When you finish, write your complete reply (both parts) to \`$out\` and return
it as your final message.
MSG
