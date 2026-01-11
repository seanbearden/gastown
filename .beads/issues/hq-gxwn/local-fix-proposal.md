# Local Fix Proposal: Orphan Process Cleanup

## Summary

Until Claude Code fixes SIGHUP handling upstream, Gas Town can implement local mitigations to reduce orphan accumulation.

## Proposed Changes

### 1. Enhanced Orphan Detection (`internal/doctor/orphan_check.go`)

Make `OrphanProcessCheck` fixable by adding ability to kill orphaned processes:

```go
// OrphanProcessCheck should become a FixableCheck
type OrphanProcessCheck struct {
    FixableCheck
    orphanProcesses []processInfo // Cache for Fix
}

// Fix kills orphaned Claude processes that are clearly Gas Town orphans
func (c *OrphanProcessCheck) Fix(ctx *CheckContext) error {
    for _, proc := range c.orphanProcesses {
        // Safety: Only kill if definitely a Gas Town process
        // Check environment or cwd for Gas Town markers
        if c.isGasTownProcess(proc) {
            // Kill process group to get child MCP servers too
            syscall.Kill(-proc.pid, syscall.SIGKILL)
        }
    }
    return nil
}
```

### 2. Graceful Session Stop (`internal/tmux/tmux.go`)

Send Ctrl-C before killing session to give Claude a chance to cleanup:

```go
// GracefulKillSession sends Ctrl-C, waits, then kills the session
func (t *Tmux) GracefulKillSession(name string, timeout time.Duration) error {
    // Send Ctrl-C to trigger Claude's interrupt handler
    if _, err := t.run("send-keys", "-t", name, "C-c"); err != nil {
        // Continue even if this fails
    }

    // Wait for process to exit gracefully
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if !t.IsClaudeRunning(name) {
            break
        }
        time.Sleep(200 * time.Millisecond)
    }

    // Force kill the session
    return t.KillSession(name)
}
```

### 3. Daemon Heartbeat Cleanup (`internal/daemon/daemon.go`)

Add orphan cleanup to daemon heartbeat:

```go
func (d *Daemon) heartbeat() {
    // ... existing heartbeat code ...

    // Periodic orphan cleanup (every 10 heartbeats)
    if d.heartbeatCount % 10 == 0 {
        d.cleanupOrphanProcesses()
    }
}

func (d *Daemon) cleanupOrphanProcesses() {
    // Get all tmux pane PIDs
    panePIDs := d.getTmuxPanePIDs()

    // Find Claude processes not in any pane
    claudeProcs := findClaudeProcesses()
    for _, pid := range claudeProcs {
        if !panePIDs[pid] && isGasTownProcess(pid) {
            d.logger.Printf("Cleaning up orphan Claude process: %d", pid)
            syscall.Kill(-pid, syscall.SIGKILL)
        }
    }
}
```

### 4. Session Restart Pre-Cleanup (`internal/daemon/lifecycle.go`)

Before restarting a session, ensure old orphans are cleaned:

```go
func (d *Daemon) restartSession(sessionName, identity string) error {
    // Clean up any orphaned processes from previous incarnation
    d.cleanupSessionOrphans(sessionName)

    // ... existing restart code ...
}
```

## Implementation Priority

1. **High**: Graceful session stop (immediate benefit, low risk)
2. **Medium**: Daemon heartbeat cleanup (catches stragglers)
3. **Low**: Enhanced doctor check (manual cleanup option)

## Testing Plan

1. Start multiple Claude sessions via `gt up`
2. Force-kill some sessions via `tmux kill-session`
3. Verify orphans are detected and cleaned
4. Monitor memory usage over time

## Risks

- **False positives**: Killing user's personal Claude session
  - Mitigation: Check for Gas Town environment markers (GT_ROLE, etc.)

- **Race conditions**: Killing process during cleanup
  - Mitigation: Use SIGKILL with error handling

## Metrics

Track improvement by monitoring:
- Number of orphan processes detected per heartbeat
- Memory recovered per cleanup cycle
- Time between orphan creation and cleanup
