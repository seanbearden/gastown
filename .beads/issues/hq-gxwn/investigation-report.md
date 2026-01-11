# tmux/Claude Zombie Session Investigation Report

**Issue ID**: hq-gxwn
**Date**: 2026-01-11
**Investigator**: morsov (polecat)

## Executive Summary

Investigation confirmed **three distinct problems** causing resource leaks and session management issues:

1. **Orphaned Claude processes** - Claude instances survive tmux session death
2. **Orphaned MCP server processes** - MCP servers accumulate without cleanup
3. **Process tree isolation failure** - Child processes not properly terminated when parent session killed

## Current System State

At time of investigation:
- **19 Claude processes** running
- **15 tmux sessions** active
- **12 orphaned Claude processes** (no matching tmux session)
- **115 MCP server processes** running
- **~5.8 GB memory** consumed by orphan Claude processes alone

### Orphan Details

| PID | Elapsed Time | Status |
|-----|--------------|--------|
| 1199837 | 8h 39m | No tmux session (very old) |
| 1526607 | 9m | Reparented to tmux server |
| 1541788 | 8m | Reparented to tmux server |
| 1551157 | 7m | Reparented to tmux server |
| 1559357 | 6m | Reparented to tmux server |
| ... | ... | (7 more recent orphans) |

## Root Cause Analysis

### Problem 1: Orphaned Claude Processes

When a tmux session is killed or recycled:
1. Claude process receives SIGHUP from tmux
2. Claude doesn't properly handle SIGHUP for graceful shutdown
3. Claude process becomes orphaned (reparented to init or tmux server)
4. MCP servers spawned by Claude also become orphaned

**Evidence**: Orphan PID 1526607 has PPID 1522098 (the tmux server), not init (PID 1).

### Problem 2: Process Group Isolation

Claude Code spawns MCP servers as child processes but doesn't:
- Create a proper process group for cleanup
- Use `setsid` to isolate process trees
- Properly signal child processes on exit

**Impact**: When Claude exits (normally or abnormally), MCP servers persist.

### Problem 3: Session Cleanup Race Condition

Gas Town's `tmux.KillSession()` sends `kill-session` to tmux, which:
1. Sends SIGHUP to pane processes
2. Immediately destroys the session
3. Doesn't wait for process termination

**Result**: Child processes escape and become orphans before they can be cleaned up.

## Reproduction Steps

### Minimum Reproduction

```bash
# 1. Start Claude in tmux
tmux new-session -d -s test-session "claude"

# 2. Wait for Claude to start and spawn MCP servers
sleep 10

# 3. Count processes before
echo "Before:" && pgrep -c -x claude && pgrep -c -f mcp-server

# 4. Kill the session (simulates Gas Town session cleanup)
tmux kill-session -t test-session

# 5. Count processes after - orphans will remain
echo "After:" && pgrep -c -x claude && pgrep -c -f mcp-server
```

### Reproduction in Gas Town

1. Start Gas Town with multiple polecats: `gt up`
2. Let polecats cycle a few times (they restart periodically)
3. Run orphan check: `pgrep -x claude | wc -l` vs `tmux list-sessions | wc -l`
4. Observe orphan accumulation over time

## Upstream Issues

### Existing Reports

| Issue | Title | Status | Relevance |
|-------|-------|--------|-----------|
| [#1935](https://github.com/anthropics/claude-code/issues/1935) | MCP servers not properly terminated when Claude Code exits | REOPENED | Primary - MCP orphan issue |
| [#5545](https://github.com/anthropics/claude-code/issues/5545) | Orphaned Processes Persist After Claude Code Execution | Closed (dup of #1935) | Duplicate |
| [#13126](https://github.com/anthropics/claude-code/issues/13126) | Claude code is killed by OOM killer due to subprocess issue | OPEN | Related - process group isolation |

### Root Issue Not Fully Addressed

Issue #1935 was marked "fixed in v1.0.21" but reopened as the fix regressed. The fundamental problem remains:

> Claude Code fails to properly terminate child processes (MCP servers, shells, subprocesses) when:
> - User exits Claude Code
> - Claude Code crashes
> - Parent terminal/tmux session is killed
> - SIGHUP/SIGTERM signals received

## Proposed Solutions

### Claude Code Side (Upstream)

1. **Signal Handler**: Add proper SIGHUP handler that initiates graceful shutdown
2. **Process Group**: Create process group for child processes and kill group on exit
3. **MCP Cleanup**: Implement proper MCP server shutdown sequence per MCP spec
4. **Timeout Kill**: If graceful fails, SIGKILL remaining children after timeout

### Gas Town Side (Local)

1. **Graceful Stop**: Send Ctrl-C before kill-session to trigger Claude's exit handler
2. **Orphan GC**: Enhance `gt doctor --fix` to detect and kill orphaned Claude processes
3. **Pre-Kill Wait**: Add delay between SIGHUP and session destruction
4. **Process Tracking**: Track spawned processes by tmux session for cleanup

## Immediate Mitigations

### Manual Cleanup Script

```bash
#!/bin/bash
# cleanup-orphans.sh - Kill orphaned Claude and MCP processes

echo "Finding orphaned Claude processes..."
pgrep -x claude | while read pid; do
  # Check if PID is a tmux pane
  is_pane=false
  tmux list-panes -a -F '#{pane_pid}' 2>/dev/null | grep -q "^${pid}$" && is_pane=true
  if [ "$is_pane" = "false" ]; then
    echo "Killing orphan Claude: $pid"
    kill -9 $pid 2>/dev/null
  fi
done

echo "Killing orphaned MCP servers..."
# MCP servers with no Claude parent
pgrep -f mcp-server | while read pid; do
  ppid=$(ps -o ppid= -p $pid 2>/dev/null | tr -d ' ')
  # If parent is init (1) or doesn't exist, it's orphaned
  if [ "$ppid" = "1" ] || ! ps -p $ppid >/dev/null 2>&1; then
    echo "Killing orphan MCP: $pid"
    kill -9 $pid 2>/dev/null
  fi
done

echo "Done. Memory freed."
```

### Cron Job for Periodic Cleanup

```bash
# Add to crontab for hourly cleanup
0 * * * * /path/to/cleanup-orphans.sh >> /tmp/orphan-cleanup.log 2>&1
```

## Recommended Next Steps

1. **File/Update Upstream Issue**: Add Gas Town findings to #1935 or file new detailed issue
2. **Implement Gas Town Mitigations**: Add orphan cleanup to daemon heartbeat
3. **Monitor**: Track orphan counts over time to measure improvement
4. **Coordinate**: Work with Anthropic team on proper fix timeline

## Appendix: System Information

- **Platform**: Linux (WSL2) 6.6.87.2-microsoft-standard-WSL2
- **Claude Code Version**: Observed in multiple recent versions
- **Terminal**: tmux 3.x inside WSL2
- **Total RAM**: 81 GB
- **Investigation Date**: 2026-01-11

## References

- [Claude Code Issue #1935](https://github.com/anthropics/claude-code/issues/1935) - MCP server orphan processes
- [Claude Code Issue #13126](https://github.com/anthropics/claude-code/issues/13126) - OOM killer targeting Claude
- [Claude Code Issue #5545](https://github.com/anthropics/claude-code/issues/5545) - General orphan processes
- Gas Town `internal/tmux/tmux.go` - Session management code
- Gas Town `internal/doctor/orphan_check.go` - Existing orphan detection
