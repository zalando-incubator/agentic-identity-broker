# Reviewer task template

Each eval run starts a fresh sub-agent (the Claude Code `Agent` tool,
`general-purpose`, with no shared context) whose whole message is the output of
`render-task.sh <prompt.md> <clone> <out-file>`. The script copies the dimension
section into `<clone>/.git/agent-definition.md`, so the sub-agent reads only that
file, the brief and the clone.

The clone contains only the ancestry of the reviewed head and of its base.
`make-brief.sh` documents how it is built (`git init`, then
`git fetch --no-tags <repo> <head>:refs/heads/pr-head <base>:refs/heads/base`).
It has no remote, so the run cannot reach later fix commits or review comments.

The rendered message:

```text
You are a sub-agent of an automated architecture review. Your agent definition
is the file `<clone>/.git/agent-definition.md`. Read it in full first and
follow it exactly.

Task from the lead reviewer:
- Brief: `<clone>/brief.md`
- Pull request: #293; head and base SHAs are in the brief (`pr-head` is checked out, `base` is a local branch)
- Maintainer focus: none
- Follow your own dimension instructions.

Sandbox rules for this run: work only inside `<clone>` (use it as the working
directory for every command). Do not read any other checkout or directory on
this machine, do not use the network, GitHub, or `gh`, and do not modify
tracked files. Temporary files go under `<clone>/.git/probe-tmp/`.

When you finish, write your complete reply (both parts) to `<out-file>` and return
it as your final message.
```
