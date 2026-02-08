#!/usr/bin/env bash
# run-hardener.sh — Launch migration hardener agent in a tmux session
#
# Usage: ./scripts/run-hardener.sh
#
# Launches claude in a tmux session, then uses gt nudge to send the initial
# prompt (tmux send-keys + Enter doesn't work with Claude Code TUI).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SESSION_NAME="migration-hardener"

# Kill existing session if any
tmux kill-session -t "$SESSION_NAME" 2>/dev/null || true

# Launch claude (skip project hooks — gt prime --hook hangs without GT_ROLE)
tmux new-session -d -s "$SESSION_NAME" -c "$REPO_ROOT" \
    "claude --dangerously-skip-permissions --setting-sources user"

echo "Waiting for session to initialize..."
sleep 8

# Send initial prompt via gt nudge (reliable text delivery to Claude Code TUI)
gt nudge "$SESSION_NAME" "You are a solo migration hardening agent. Read .claude/agents/at-migration-mission.md for your full mission and .claude/agents/migration-hardener.md for your role context. Execute all 5 phases autonomously. Push directly to main. VM access: gcloud compute ssh migration-test-lab --zone=us-west1-b. Start Phase 1 now."

echo ""
echo "Migration hardener launched in tmux session: $SESSION_NAME"
echo "  Monitor: tmux attach -t $SESSION_NAME"
echo "  Check:   tmux capture-pane -t $SESSION_NAME -p | tail -20"
