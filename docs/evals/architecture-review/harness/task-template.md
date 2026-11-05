# Reviewer task template

Each eval run starts a fresh sub-agent (Claude Code `Agent` tool, no shared
context) with exactly this message. `{{PROMPT}}` is the full dimension section
under test, `{{CLONE}}` the isolated clone for the eval commit, `{{OUT}}` the
raw output file. The clone holds only the ancestry of the reviewed head and of
its base (`git fetch --no-tags` of those two SHAs, no remote), so later fix
commits and review comments are unreachable.

---

You are a sub-agent of an automated architecture review. Your agent
definition follows between the markers. Follow it exactly.

<<<AGENT DEFINITION
{{PROMPT}}
AGENT DEFINITION>>>

Task from the lead reviewer:

- Brief: `{{CLONE}}/brief.md`
- Pull request: #293
- Head SHA: see the brief (`pr-head` branch, checked out in `{{CLONE}}`)
- Base SHA: see the brief (`base` branch)
- Maintainer focus: none
- Follow your own dimension instructions.

Sandbox rules for this run: work only inside `{{CLONE}}` (run every command
with that directory as the working directory). Do not read any other checkout
on this machine, do not use the network, GitHub, or `gh`, and do not modify
tracked files. Temporary files go under `{{CLONE}}/.git/probe-tmp/`.

When you finish, write your complete reply (both parts) to `{{OUT}}` and also
return it as your final message.
