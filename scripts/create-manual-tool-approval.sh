#!/usr/bin/env bash
set -euo pipefail

BROKER_URL="${BROKER_URL:-http://localhost:8000}"
ADMIN_URL="${ADMIN_URL:-http://localhost:14000}"
UPSTREAM_URL="${UPSTREAM_URL:-http://localhost:9001}"
TOOL_NAME="${1:-send_finance_report}"

require_command() {
	command -v "$1" >/dev/null || {
		echo "error: $1 is required" >&2
		exit 1
	}
}

for command in curl python3; do
	require_command "$command"
done

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT
agent_id=$(
	curl -fsS -H 'X-Remote-User: dev@example.com' "$ADMIN_URL/api/agents" -o "$tmp_dir/agents.json"
	python3 - "$tmp_dir/agents.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response:
    for agent in json.load(response):
        if agent.get("client_id") == "upstream-oauth2-client":
            print(agent["id"])
            break
    else:
        raise SystemExit("the Sample Agent is not seeded")
PY
)

[ -n "$agent_id" ]


code=$(
	curl -fsS -D "$tmp_dir/headers" -o /dev/null -X POST "$UPSTREAM_URL/oauth/authorize" \
		--data-urlencode 'client_id=upstream-oauth2-client' \
		--data-urlencode 'redirect_uri=http://localhost:9999/callback' \
		--data-urlencode 'response_type=code' \
		--data-urlencode 'scope=openid profile email' \
		--data-urlencode 'state=manual-tool-approval' \
		--data-urlencode 'approval=approve'
	awk 'tolower($1) == "location:" { print $2 }' "$tmp_dir/headers" | tr -d '\r' | python3 -c '
import sys
from urllib.parse import parse_qs, urlparse
print(parse_qs(urlparse(sys.stdin.read().strip()).query)["code"][0])
'
)

subject_token=$(
	curl -fsS -X POST "$UPSTREAM_URL/oauth/token" \
		--data-urlencode 'grant_type=authorization_code' \
		--data-urlencode "code=$code" \
		--data-urlencode 'redirect_uri=http://localhost:9999/callback' \
		--data-urlencode 'client_id=upstream-oauth2-client' \
		--data-urlencode 'client_secret=upstream-oauth2-secret-xyz' \
		-o "$tmp_dir/subject-token.json"
	python3 - "$tmp_dir/subject-token.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response:
    print(json.load(response)["access_token"])
PY
)

client_assertion=$(
	curl -fsS -X POST "$UPSTREAM_URL/oauth/token" \
		--data-urlencode 'grant_type=client_credentials' \
		--data-urlencode 'client_id=extproc-gateway' \
		--data-urlencode 'client_secret=extproc-dev-secret' \
		-o "$tmp_dir/client-assertion.json"
	python3 - "$tmp_dir/client-assertion.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as response:
    print(json.load(response)["access_token"])
PY
)

python3 - "$TOOL_NAME" >"$tmp_dir/request.json" <<'PY'
import json
import sys
import uuid

json.dump({
    "metadata": {
        "mcp_session_id": "manual-mcp-session",
        "agent_session_id": "manual-agent-session",
        "tool_invocation_id": str(uuid.uuid4()),
        "description": "Manual local tool-approval test",
    },
    "tool_name": sys.argv[1],
    "arguments": {"requested_at": str(uuid.uuid4())},
    "risk_level": "medium",
}, sys.stdout)
PY

response=$(curl -fsS -X POST "$BROKER_URL/api/approvals" \
	-H "Authorization: Bearer $subject_token" \
	-H "X-Client-Assertion: $client_assertion" \
	-H 'Content-Type: application/json' \
	--data-binary "@$tmp_dir/request.json")

printf '%s' "$response" | python3 -c '
import json, sys
approval = json.load(sys.stdin)["data"]
print("Pending approval created")
print("ID: " + approval["id"])
print("Open: " + approval["approval_url"])
'
