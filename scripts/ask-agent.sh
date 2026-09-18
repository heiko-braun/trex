#!/bin/sh
set -e
AGENT_ID="$1"
MESSAGE="$2"
TMPFILE=$(mktemp)
trap 'rm -f "$TMPFILE"' EXIT

SEND_OUT=$(agentctl a2a send "$AGENT_ID" "$MESSAGE" --async 2>/dev/null)

case "$SEND_OUT" in
  "task "*)
    TASK_ID=$(echo "$SEND_OUT" | awk '{print $2}')
    for i in $(seq 1 40); do
      sleep 5
      agentctl a2a task get "$TASK_ID" --agent "$AGENT_ID" > "$TMPFILE" 2>/dev/null
      STATE=$(python3 -c "import json; print(json.load(open('$TMPFILE'))['status']['state'])")
      if [ "$STATE" = "TASK_STATE_COMPLETED" ] || [ "$STATE" = "TASK_STATE_FAILED" ]; then
        cat "$TMPFILE"
        exit 0
      fi
    done
    echo "TIMEOUT waiting for task $TASK_ID" >&2
    exit 1
    ;;
  *)
    python3 -c "import json,sys; print(json.dumps({'status':{'state':'TASK_STATE_COMPLETED'},'artifacts':[{'parts':[{'text': sys.argv[1]}]}]}))" "$SEND_OUT"
    ;;
esac
