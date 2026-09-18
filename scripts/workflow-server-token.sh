#!/bin/sh
# Print a Bearer token for calling the trex workflow-server's own API
# (POST /definitions, GET /agents, POST /agents/{id}/register, etc).
#
# Reuses the token cached by `agentctl login` — the same Keycloak realm
# the workflow-server validates against (confirmed working across
# clients: agentctl's own agent-cli client and the UI's
# managed-agents-console client are both accepted). Re-run
# `agentctl login` first if this fails — the cached token is short-lived
# (~5 min observed).
#
# Usage: TOK=$(./scripts/workflow-server-token.sh)
set -e

TOKEN_FILE="$HOME/.agentctl/tokens/default.yaml"
if [ ! -f "$TOKEN_FILE" ]; then
  echo "no cached agentctl token found at $TOKEN_FILE; run 'agentctl login' first" >&2
  exit 1
fi

python3 -c "
import yaml, json
d = yaml.safe_load(open('$TOKEN_FILE'))
print(json.loads(d['auth_token'])['access_token'])
"
