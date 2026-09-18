#!/bin/sh
# Register a managed-agents platform agent as a Temporal activity worker on
# the local workflow-server, so `call: activity, name: invoke-agent` on
# task queue agent-<id> reaches it. See specs/agent-discovery-and-registration.md.
#
# Usage: register-agent.sh <agent-id> <agent-name> [workflow-server-url]
#
# Auth: reuses the token cached by `agentctl login` (the same Keycloak
# realm the workflow-server validates against). Re-run `agentctl login`
# first if this fails with "invalid or expired token" — the cached token
# is short-lived (~5 min observed).
set -e

AGENT_ID="$1"
AGENT_NAME="$2"
SERVER_URL="${3:-http://localhost:8080}"

if [ -z "$AGENT_ID" ] || [ -z "$AGENT_NAME" ]; then
  echo "usage: $0 <agent-id> <agent-name> [workflow-server-url]" >&2
  exit 1
fi

TOKEN_FILE="$HOME/.agentctl/tokens/default.yaml"
if [ ! -f "$TOKEN_FILE" ]; then
  echo "no cached agentctl token found at $TOKEN_FILE; run 'agentctl login' first" >&2
  exit 1
fi

TOKEN=$(python3 -c "
import yaml, json
d = yaml.safe_load(open('$TOKEN_FILE'))
print(json.loads(d['auth_token'])['access_token'])
")

curl -sf -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"$AGENT_NAME\"}" \
  "$SERVER_URL/agents/$AGENT_ID/register"
echo
