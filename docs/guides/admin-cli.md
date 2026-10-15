---
title: "Administer with aibctl"
description: Install the aibctl command-line tool, configure a context for your broker, authenticate with a token command, and manage agents, services, permission sets, and keys with tab completion.
---

# Administer with aibctl

`aibctl` is the command-line client for the admin API on port 14000. It sends the right
headers on every request, resolves resources by canonical ID or UUID, completes
identifiers in your shell, and returns stable exit codes for scripts.

## What you need

- A broker admin endpoint. Locally that is `http://localhost:14000`. In production it is
  the admin URL behind your authenticating proxy.
- A credential. Locally the admin server trusts the principal header directly. Behind a
  proxy you present a Bearer token, usually produced by a single-sign-on helper command.

## Install

Download the archive for your platform from the release page and put `aibctl` on your
`PATH`, or build it from the repository:

```bash
just aibctl-build
./bin/aibctl version
```

## Configure a context

A context names one broker and how to reach it. The first context you create becomes the
current one. The file lives in your OS configuration directory (`aibctl config path`
prints it) and is written with owner-only permissions.

Local development, sending the principal header directly:

```bash
aibctl context set local --server http://localhost:14000 --principal admin@example.com
aibctl health
```

A proxied deployment, deriving the Bearer token from a command each time:

```bash
aibctl context set staging \
  --server https://broker-admin.staging.example.com \
  --token-command "ztoken --audience broker-admin" \
  --output json --use
```

The token command is split on whitespace and executed without a shell. Its trimmed
standard output is the token. If it fails, times out (10 seconds by default), or prints
nothing, `aibctl` exits with code 3 and shows the command's error output.

Switch and inspect contexts:

```bash
aibctl context list
aibctl context use local
aibctl config view            # tokens and sensitive headers are redacted
```

Any setting can be overridden per invocation with a flag or an `AIBCTL_*` variable, for
example `--context`, `--server`, `--principal`, `--token`, `-o json`. See
[Configuration](../configuration.md#aibctl-configuration) for the full reference and
`examples/config/aibctl.yaml` for a complete file.

If you reach the broker through a port-forward or tunnel, the broker rejects requests whose
`Host` differs from its `server.admin.public_url`. Set `--host-header` (or `host_header`
in the context) to the hostname the broker expects.

## Enable tab completion

```bash
# bash
source <(aibctl completion bash)
# zsh
aibctl completion zsh > "${fpath[1]}/_aibctl"
# fish
aibctl completion fish | source
# PowerShell
aibctl completion powershell | Out-String | Invoke-Expression
```

Besides commands and flags, completion queries the current context for identifiers:
`aibctl agent get <TAB>` lists agents by canonical ID with their display names. Lookups
are bounded by `AIBCTL_COMPLETION_TIMEOUT` (default 2 seconds) and fail silently.

## Identifiers

Every resource has a UUID and may have a canonical ID (`[A-Za-z0-9._-]{1,128}`). Commands
accept either. Tables show the canonical ID and fall back to the UUID; add `--show-uuid` or
`-o wide` to see both. JSON and YAML output carry `id` and `canonical_id`, and nested
references (`service_id`, `permission_set_id`) are rendered canonically.

## Manage resources

Create the objects in dependency order: service, permission set, agent.

```bash
# Register a confidential OAuth2 service
aibctl service create --display-name GitHub --canonical-id github \
  --client-id "$GITHUB_CLIENT_ID" --client-secret "$GITHUB_CLIENT_SECRET" \
  --oauth2-flavor github --issuer-uri https://github.com \
  --token-endpoint https://github.com/login/oauth/access_token \
  --authorize-endpoint https://github.com/login/oauth/authorize \
  --scope repo="Repository access" --protected-resource https://api.github.com/

# Group scopes into a permission set
aibctl permission-set create --name "GitHub Read" --description "Read repositories" \
  --canonical-id github-read --service-scope github=repo:mandatory

# Register an agent that requires the permission set
aibctl agent create --display-name "Research Agent" --description "Summarises repositories" \
  --canonical-id research-agent --permission-set github-read:mandatory

# Issue broker client credentials (local and hybrid server modes)
aibctl agent client-credentials create research-agent
```

Agents, services, and permission sets support `list`, `get`, `create`, `update`, and
`delete`; their create and update accept either field flags or a document:

```bash
aibctl agent create -f agent.yaml
aibctl agent get research-agent -o yaml > agent.yaml   # edit, then:
aibctl agent update research-agent -f agent.yaml
```

With `--file` the document is the complete new representation. With field flags the
current resource is fetched and only the given fields change; list flags such as
`--permission-set` replace the whole list. To remove an optional identifier use
`--clear-canonical-id` or `--clear-client-id`.

Service updates are conditional: `aibctl` reads the service's ETag first and sends it as
`If-Match`, so a concurrent change fails with exit code 5. Because a replacement must carry
the secret, `service update` requires `--client-secret` (or `client_secret` in the
document) for static confidential services and refuses the redacted placeholder from
`get` output.

Protected resources have their own verbs:

```bash
aibctl service protected-resource list github
aibctl service protected-resource add github https://uploads.github.com/
aibctl service protected-resource rename github https://uploads.github.com https://uploads.github.com/v2
aibctl service protected-resource remove github https://uploads.github.com/v2 --yes
```

Keys for the OAuth2 server and for outbound CIMD client authentication:

```bash
aibctl signing-key list
aibctl signing-key create
aibctl signing-key set-current <kid>
aibctl cimd-client-key list
```

Destructive commands ask for confirmation. Pass `--yes` or set `AIBCTL_ASSUME_YES=1` in
scripts.

## Scripting

Standard output carries data only; prompts, confirmations, and errors go to standard
error. Use `-o json` and the exit code:

```bash
aibctl agent list -o json | jq -r '.[].canonical_id // .[].id'
if ! aibctl agent get research-agent -o json >/dev/null; then
  echo "missing"   # exit code 4
fi
```

| Exit code | Meaning |
|---|---|
| 0 | Success |
| 1 | Transport failure, server error, or unexpected status |
| 2 | Usage: invalid arguments, flags, input document, or configuration |
| 3 | Authentication: token command failed, or HTTP 401/403 |
| 4 | Not found (HTTP 404) |
| 5 | Conflict or precondition (HTTP 409, 412, 428) |
| 6 | Server-side validation (HTTP 400, 422) |

Add `-v` to log each request and response status to standard error. Tokens are never
logged.
