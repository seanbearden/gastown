# [BUG] Claude Code orphans processes when parent tmux session is killed (SIGHUP handling)

## Summary

Claude Code fails to properly terminate when receiving SIGHUP from tmux session kill, leaving orphaned Claude processes and MCP servers consuming memory indefinitely.

## Environment

| Property | Value |
|----------|-------|
| **Claude Code Version** | latest (2.x) |
| **Platform** | Linux (WSL2) |
| **OS Version** | Ubuntu 24.04 on WSL2 6.6.87.2 |
| **Terminal/Shell** | tmux 3.x + zsh |

## Description

When running Claude Code inside tmux and the tmux session is killed (e.g., via `tmux kill-session`), Claude Code receives SIGHUP but doesn't properly shut down. Instead:

1. Claude process becomes orphaned (reparented to init or tmux server)
2. All spawned MCP servers also become orphaned
3. These processes continue consuming memory indefinitely
4. Over time, dozens of orphaned processes accumulate

This is particularly problematic for automation systems that manage multiple Claude sessions.

## Reproduction Steps

```bash
# 1. Start Claude in tmux
tmux new-session -d -s test-session "claude"

# 2. Wait for Claude to fully start and spawn MCP servers
sleep 15

# 3. Count processes before killing session
echo "Before kill:" && pgrep -c -x claude && pgrep -c -f mcp-server

# 4. Kill the tmux session (sends SIGHUP to Claude)
tmux kill-session -t test-session

# 5. Wait a moment
sleep 2

# 6. Count processes after - orphans remain!
echo "After kill:" && pgrep -c -x claude && pgrep -c -f mcp-server
# Expected: 0 processes
# Actual: Claude + MCP servers still running as orphans
```

## Evidence from Production System

Running multiple Claude sessions via tmux automation, observed after 8+ hours:

- 19 Claude processes running
- Only 15 tmux sessions active
- **12 orphaned Claude processes** consuming memory
- **115 MCP server processes** accumulated
- **~5.8 GB memory** used by orphans alone

Example orphan process tree:
```
claude(1199837)─┬─npm exec @model(1200459)─┬─sh(1201087)───node(1201093)
                ├─node(mcp-playwright)
                ├─node(context7-mcp)
                ├─node(mcp-filesystem)
                └─node(sequential-thinking)
```

This Claude process (PID 1199837) has been orphaned for 8+ hours with PPID pointing to init.

## Expected Behavior

When Claude Code receives SIGHUP (from tmux session kill):

1. Claude should catch SIGHUP and initiate graceful shutdown
2. Send termination signals to all spawned MCP servers
3. Wait briefly for MCP servers to cleanup
4. Exit cleanly

## Actual Behavior

1. Claude ignores SIGHUP (or doesn't fully handle it)
2. Claude becomes orphaned, continues running
3. MCP servers remain running as orphans
4. Memory leak accumulates over time

## Suggested Fix

```javascript
// Pseudo-code for signal handling

process.on('SIGHUP', () => {
  console.log('Received SIGHUP, initiating graceful shutdown...');
  gracefulShutdown();
});

async function gracefulShutdown() {
  // 1. Stop accepting new requests
  // 2. Send shutdown to MCP servers
  for (const server of mcpServers) {
    await server.shutdown();
  }
  // 3. Kill any remaining children in process group
  process.kill(-process.pid, 'SIGTERM');
  // 4. Exit
  process.exit(0);
}
```

Alternative: Use `setsid` to create process group and `killpg` on exit.

## Related Issues

- #1935 - MCP servers not properly terminated when Claude Code exits (REOPENED)
- #5545 - Orphaned Processes Persist After Claude Code Execution (dup of #1935)
- #13126 - OOM killer kills Claude due to subprocess issue

## Workaround

Manual cleanup script:
```bash
# Kill orphaned Claude processes not in any tmux pane
pgrep -x claude | while read pid; do
  tmux list-panes -a -F '#{pane_pid}' 2>/dev/null | grep -q "^${pid}$" || kill -9 $pid
done
```

## Impact

- **Memory leak**: ~500MB per orphaned Claude instance
- **CPU usage**: Idle orphans still consume some CPU
- **System stability**: Eventually causes OOM conditions
- **User confusion**: Zombie sessions prevent clean restarts

## Additional Context

This issue is related to but distinct from #1935. Issue #1935 focuses on MCP cleanup during normal Claude exit. This issue focuses on signal handling when the parent session is externally terminated (SIGHUP from tmux kill-session).

The fix for #1935 (v1.0.21) may not have addressed SIGHUP handling specifically, which is why orphans still occur in tmux automation scenarios.
