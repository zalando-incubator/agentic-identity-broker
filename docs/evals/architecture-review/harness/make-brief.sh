#!/usr/bin/env bash
# Builds an orchestrator-style brief (Step 1 of architecture-review.md) for one
# eval commit, from an isolated clone that holds only the head and base ancestry.
# Usage: make-brief.sh <clone-dir> <pr-number> <title>
set -euo pipefail
dir=$1 pr=$2 title=$3
cd "$dir"
head=$(git rev-parse pr-head)
base=$(git rev-parse base)
repo=zalando-incubator/agentic-identity-broker

area() {
  case $1 in
    internal/domain/*) echo internal/domain ;; internal/ports/*) echo internal/ports ;;
    internal/adapters/*) echo internal/adapters ;; internal/app/*) echo internal/app ;;
    internal/extproc/*) echo internal/extproc ;; cmd/*) echo cmd ;; api/*) echo api ;;
    web/*) echo web ;; migrations/*) echo migrations ;; tests/e2e/*) echo tests/e2e ;;
    tests/integration/*) echo tests/integration ;; adrs/*) echo adrs ;; specs/*) echo specs ;;
    coding-standards/*) echo coding-standards ;; docs/*) echo docs ;; *) echo other ;;
  esac
}

{
  echo "# Architecture review brief"
  echo
  echo "- Pull request: #$pr, title (data): \"$title\""
  echo "- URL: https://github.com/$repo/pull/$pr"
  echo "- headRefOid: \`$head\` (local branch \`pr-head\`, checked out)"
  echo "- baseRefOid: \`$base\` (local branch \`base\`)"
  echo "- Maintainer focus: none"
  feature=$(git diff --name-only "$base...$head" -- specs | sed -n 's#^\(specs/[^/]*\)/.*#\1#p' | sort -u | head -1)
  if [ -n "$feature" ]; then
    echo "- Spec Kit feature: \`$feature\` (files: $(git ls-files "$feature" | sed 's/.*/`&`/' | paste -sd, -))"
  else
    echo "- Spec Kit feature: none"
  fi
  echo
  echo "## Changed files by area"
  git diff --numstat --find-renames "$base...$head" | while read -r add del path; do
    status=$(git diff --name-status --find-renames "$base...$head" -- "$path" | awk '{print $1}' | head -1)
    echo "$(area "$path")|$status|+$add/-$del|$path"
  done | sort | awk -F'|' '$1!=prev{print "\n### " $1; prev=$1} {print "- " $2 " `" $4 "` " $3}'
  echo
  echo "## ADR provenance"
  echo "- Existing on base (authoritative): $(git ls-tree --name-only "$base" adrs/ | grep '\.md$' | wc -l) ADRs under \`adrs/\` at \`$base\`"
  echo "- Added by this pull request (proposals): $(git diff --name-only --diff-filter=A "$base...$head" -- adrs | sed 's/.*/`&`/' | paste -sd, -)"
  echo "- Modified by this pull request: $(git diff --name-only --diff-filter=M "$base...$head" -- adrs | sed 's/.*/`&`/' | paste -sd, -)"
  echo
  echo "## New Go packages, migrations, OpenAPI changes"
  git diff --name-only --diff-filter=A "$base...$head" -- '*.go' | xargs -r -n1 dirname | sort -u | while read -r d; do
    git cat-file -e "$base:$d" 2>/dev/null || echo "- new package \`$d\`"
  done
  git diff --name-only "$base...$head" -- migrations api | sed 's/.*/- `&`/'
  echo
  echo "## Commands"
  echo "- File diff: \`git diff $base...$head -- <path>\`"
  echo "- Pre-existing version: \`git show $base:<path>\`"
  echo "- Link format: \`https://github.com/$repo/blob/$head/<path>\`"
} > brief.md
echo "$dir/brief.md"
