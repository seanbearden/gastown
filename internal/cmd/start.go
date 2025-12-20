package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var (
	shutdownGraceful bool
	shutdownWait     int
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Gas Town",
	Long: `Start Gas Town by launching the Deacon and Mayor.

The Deacon is the health-check orchestrator that monitors Mayor and Witnesses.
The Mayor is the global coordinator that dispatches work.

Other agents (Witnesses, Refineries, Polecats) are started lazily as needed.

To stop Gas Town, use 'gt shutdown'.`,
	RunE: runStart,
}

var shutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Shutdown Gas Town",
	Long: `Shutdown Gas Town by stopping all agents.

By default, immediately kills all sessions. Use --graceful to allow agents
time to save their state and update handoff beads.

Stops agents in the correct order:
1. Deacon (health monitor) - so it doesn't restart others
2. All polecats, witnesses, refineries, crew
3. Mayor (global coordinator)`,
	RunE: runShutdown,
}

func init() {
	shutdownCmd.Flags().BoolVarP(&shutdownGraceful, "graceful", "g", false,
		"Send ESC to agents and wait for them to handoff before killing")
	shutdownCmd.Flags().IntVarP(&shutdownWait, "wait", "w", 30,
		"Seconds to wait for graceful shutdown (default 30)")

	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(shutdownCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Verify we're in a Gas Town workspace
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	t := tmux.NewTmux()

	fmt.Printf("Starting Gas Town from %s\n\n", style.Dim.Render(townRoot))

	// Start Mayor first (so Deacon sees it as up)
	mayorRunning, _ := t.HasSession(MayorSessionName)
	if mayorRunning {
		fmt.Printf("  %s Mayor already running\n", style.Dim.Render("○"))
	} else {
		fmt.Printf("  %s Starting Mayor...\n", style.Bold.Render("→"))
		if err := startMayorSession(t); err != nil {
			return fmt.Errorf("starting Mayor: %w", err)
		}
		fmt.Printf("  %s Mayor started\n", style.Bold.Render("✓"))
	}

	// Start Deacon (health monitor)
	deaconRunning, _ := t.HasSession(DeaconSessionName)
	if deaconRunning {
		fmt.Printf("  %s Deacon already running\n", style.Dim.Render("○"))
	} else {
		fmt.Printf("  %s Starting Deacon...\n", style.Bold.Render("→"))
		if err := startDeaconSession(t); err != nil {
			return fmt.Errorf("starting Deacon: %w", err)
		}
		fmt.Printf("  %s Deacon started\n", style.Bold.Render("✓"))
	}

	fmt.Println()
	fmt.Printf("%s Gas Town is running\n", style.Bold.Render("✓"))
	fmt.Println()
	fmt.Printf("  Attach to Mayor:  %s\n", style.Dim.Render("gt mayor attach"))
	fmt.Printf("  Attach to Deacon: %s\n", style.Dim.Render("gt deacon attach"))
	fmt.Printf("  Check status:     %s\n", style.Dim.Render("gt status"))

	return nil
}

func runShutdown(cmd *cobra.Command, args []string) error {
	t := tmux.NewTmux()

	if shutdownGraceful {
		return runGracefulShutdown(t)
	}
	return runImmediateShutdown(t)
}

func runGracefulShutdown(t *tmux.Tmux) error {
	fmt.Printf("Graceful shutdown of Gas Town (waiting up to %ds)...\n\n", shutdownWait)

	// Collect all gt-* sessions
	sessions, err := t.ListSessions()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}

	var gtSessions []string
	for _, sess := range sessions {
		if strings.HasPrefix(sess, "gt-") {
			gtSessions = append(gtSessions, sess)
		}
	}

	if len(gtSessions) == 0 {
		fmt.Printf("%s Gas Town was not running\n", style.Dim.Render("○"))
		return nil
	}

	// Phase 1: Send ESC to all agents to interrupt them
	fmt.Printf("Phase 1: Sending ESC to %d agent(s)...\n", len(gtSessions))
	for _, sess := range gtSessions {
		fmt.Printf("  %s Interrupting %s\n", style.Bold.Render("→"), sess)
		_ = t.SendKeysRaw(sess, "Escape")
	}

	// Phase 2: Send shutdown message asking agents to handoff
	fmt.Printf("\nPhase 2: Requesting handoff from agents...\n")
	shutdownMsg := "[SHUTDOWN] Gas Town is shutting down. Please save your state and update your handoff bead, then type /exit or wait to be terminated."
	for _, sess := range gtSessions {
		// Small delay then send the message
		time.Sleep(500 * time.Millisecond)
		_ = t.SendKeys(sess, shutdownMsg)
	}

	// Phase 3: Wait for agents to complete handoff
	fmt.Printf("\nPhase 3: Waiting %ds for agents to complete handoff...\n", shutdownWait)
	fmt.Printf("  %s\n", style.Dim.Render("(Press Ctrl-C to force immediate shutdown)"))

	// Wait with countdown
	for remaining := shutdownWait; remaining > 0; remaining -= 5 {
		if remaining < shutdownWait {
			fmt.Printf("  %s %ds remaining...\n", style.Dim.Render("⏳"), remaining)
		}
		sleepTime := 5
		if remaining < 5 {
			sleepTime = remaining
		}
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	// Phase 4: Kill all sessions
	fmt.Printf("\nPhase 4: Terminating sessions...\n")
	stopped := 0

	// Stop Deacon first
	for _, sess := range gtSessions {
		if sess == DeaconSessionName {
			if err := t.KillSession(sess); err == nil {
				fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), sess)
				stopped++
			}
		}
	}

	// Stop others (except Mayor)
	for _, sess := range gtSessions {
		if sess == DeaconSessionName || sess == MayorSessionName {
			continue
		}
		if err := t.KillSession(sess); err == nil {
			fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), sess)
			stopped++
		}
	}

	// Stop Mayor last
	for _, sess := range gtSessions {
		if sess == MayorSessionName {
			if err := t.KillSession(sess); err == nil {
				fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), sess)
				stopped++
			}
		}
	}

	fmt.Println()
	fmt.Printf("%s Graceful shutdown complete (%d sessions stopped)\n", style.Bold.Render("✓"), stopped)
	return nil
}

func runImmediateShutdown(t *tmux.Tmux) error {
	fmt.Println("Shutting down Gas Town...\n")

	stopped := 0

	// 1. Stop Deacon first (so it doesn't try to restart others)
	deaconRunning, _ := t.HasSession(DeaconSessionName)
	if deaconRunning {
		fmt.Printf("  %s Stopping Deacon...\n", style.Bold.Render("→"))
		if err := t.KillSession(DeaconSessionName); err != nil {
			fmt.Printf("  %s Failed to stop Deacon: %v\n", style.Dim.Render("!"), err)
		} else {
			fmt.Printf("  %s Deacon stopped\n", style.Bold.Render("✓"))
			stopped++
		}
	} else {
		fmt.Printf("  %s Deacon not running\n", style.Dim.Render("○"))
	}

	// 2. Stop all other gt-* sessions (polecats, witnesses, refineries, crew)
	sessions, err := t.ListSessions()
	if err == nil {
		for _, sess := range sessions {
			// Skip Mayor (we'll stop it last) and Deacon (already stopped)
			if sess == MayorSessionName || sess == DeaconSessionName {
				continue
			}
			// Only kill gt-* sessions
			if !strings.HasPrefix(sess, "gt-") {
				continue
			}

			fmt.Printf("  %s Stopping %s...\n", style.Bold.Render("→"), sess)
			if err := t.KillSession(sess); err != nil {
				fmt.Printf("  %s Failed to stop %s: %v\n", style.Dim.Render("!"), sess, err)
			} else {
				fmt.Printf("  %s %s stopped\n", style.Bold.Render("✓"), sess)
				stopped++
			}
		}
	}

	// 3. Stop Mayor last
	mayorRunning, _ := t.HasSession(MayorSessionName)
	if mayorRunning {
		fmt.Printf("  %s Stopping Mayor...\n", style.Bold.Render("→"))
		if err := t.KillSession(MayorSessionName); err != nil {
			fmt.Printf("  %s Failed to stop Mayor: %v\n", style.Dim.Render("!"), err)
		} else {
			fmt.Printf("  %s Mayor stopped\n", style.Bold.Render("✓"))
			stopped++
		}
	} else {
		fmt.Printf("  %s Mayor not running\n", style.Dim.Render("○"))
	}

	fmt.Println()
	if stopped > 0 {
		fmt.Printf("%s Gas Town shutdown complete (%d sessions stopped)\n", style.Bold.Render("✓"), stopped)
	} else {
		fmt.Printf("%s Gas Town was not running\n", style.Dim.Render("○"))
	}

	return nil
}
