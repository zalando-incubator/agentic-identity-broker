#!/usr/bin/env bash
# Renders the reviewer task for one run: copies the dimension prompt into the
# clone (so the sub-agent never opens the main checkout) and prints the task.
# Usage: render-task.sh <prompt.md> <clone-dir> <out-file>
set -euo pipefail
prompt=$1 clone=$2 out=$3
mkdir -p "$clone/.git/probe-tmp" "$(dirname "$out")"
cp "$prompt" "$clone/.git/agent-definition.md"
cat <<MSG
You are a sub-agent of an automated architecture review. Your agent definition
is the file \`$clone/.git/agent-definition.md\`. Read it in full first and
follow it exactly.

Task from the lead reviewer:
- Brief: \`$clone/brief.md\`
- Pull request: #293; head and base SHAs are in the brief (\`pr-head\` is checked out, \`base\` is a local branch)
- Maintainer focus: none
- Follow your own dimension instructions.

Sandbox rules for this run: work only inside \`$clone\` (use it as the working
directory for every command). Do not read any other checkout or directory on
this machine, do not use the network, GitHub, or \`gh\`, and do not modify
tracked files. Temporary files go under \`$clone/.git/probe-tmp/\`.

When you finish, write your complete reply (both parts) to \`$out\` and return
it as your final message.
MSG
