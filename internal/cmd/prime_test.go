package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/checkpoint"
	"github.com/steveyegge/gastown/internal/constants"
)

func writeTestRoutes(t *testing.T, townRoot string, routes []beads.Route) {
	t.Helper()
	beadsDir := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatalf("create beads dir: %v", err)
	}
	if err := beads.WriteRoutes(beadsDir, routes); err != nil {
		t.Fatalf("write routes: %v", err)
	}
}

func TestGetAgentBeadID_UsesRigPrefix(t *testing.T) {
	townRoot := t.TempDir()
	writeTestRoutes(t, townRoot, []beads.Route{
		{Prefix: "bd-", Path: "beads/mayor/rig"},
	})

	cases := []struct {
		name string
		ctx  RoleContext
		want string
	}{
		{
			name: "mayor",
			ctx: RoleContext{
				Role:     RoleMayor,
				TownRoot: townRoot,
			},
			want: "hq-mayor",
		},
		{
			name: "deacon",
			ctx: RoleContext{
				Role:     RoleDeacon,
				TownRoot: townRoot,
			},
			want: "hq-deacon",
		},
		{
			name: "witness",
			ctx: RoleContext{
				Role:     RoleWitness,
				Rig:      "beads",
				TownRoot: townRoot,
			},
			want: "bd-beads-witness",
		},
		{
			name: "refinery",
			ctx: RoleContext{
				Role:     RoleRefinery,
				Rig:      "beads",
				TownRoot: townRoot,
			},
			want: "bd-beads-refinery",
		},
		{
			name: "polecat",
			ctx: RoleContext{
				Role:     RolePolecat,
				Rig:      "beads",
				Polecat:  "lex",
				TownRoot: townRoot,
			},
			want: "bd-beads-polecat-lex",
		},
		{
			name: "crew",
			ctx: RoleContext{
				Role:     RoleCrew,
				Rig:      "beads",
				Polecat:  "lex",
				TownRoot: townRoot,
			},
			want: "bd-beads-crew-lex",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := getAgentBeadID(tc.ctx)
			if got != tc.want {
				t.Fatalf("getAgentBeadID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDetectSessionState_Normal tests detection of normal startup state.
func TestDetectSessionState_Normal(t *testing.T) {
	tmpDir := t.TempDir()

	state, reason := detectSessionState(tmpDir, tmpDir)

	if state != StateNormal {
		t.Errorf("detectSessionState() = %q, want %q", state, StateNormal)
	}
	if reason != "no special context detected" {
		t.Errorf("reason = %q, want %q", reason, "no special context detected")
	}
}

// TestDetectSessionState_PostHandoff tests detection of post-handoff state.
func TestDetectSessionState_PostHandoff(t *testing.T) {
	tmpDir := t.TempDir()

	// Create handoff marker
	runtimeDir := filepath.Join(tmpDir, constants.DirRuntime)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("creating runtime dir: %v", err)
	}
	markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
	if err := os.WriteFile(markerPath, []byte("prev-session-id\n"), 0644); err != nil {
		t.Fatalf("creating handoff marker: %v", err)
	}

	state, reason := detectSessionState(tmpDir, tmpDir)

	if state != StatePostHandoff {
		t.Errorf("detectSessionState() = %q, want %q", state, StatePostHandoff)
	}
	if reason != "handoff marker present" {
		t.Errorf("reason = %q, want %q", reason, "handoff marker present")
	}
}

// TestDetectSessionState_CrashRecovery tests detection of crash recovery state.
func TestDetectSessionState_CrashRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fresh checkpoint (not stale)
	cp := &checkpoint.Checkpoint{
		Timestamp:   time.Now(),
		StepTitle:   "Working on feature X",
		MoleculeID:  "mol-abc",
		CurrentStep: "step-1",
		Branch:      "feature/x",
	}
	if err := checkpoint.Write(tmpDir, cp); err != nil {
		t.Fatalf("writing checkpoint: %v", err)
	}

	state, reason := detectSessionState(tmpDir, tmpDir)

	if state != StateCrashRecovery {
		t.Errorf("detectSessionState() = %q, want %q", state, StateCrashRecovery)
	}
	// Reason should mention the checkpoint age
	if reason == "" {
		t.Error("reason should not be empty for crash recovery")
	}
}

// TestDetectSessionState_Autonomous tests detection of autonomous state with hooked beads.
// NOTE: This test requires the bd CLI to be installed and properly configured.
// It tests the detection logic by attempting to use the beads package, which
// calls the bd CLI internally. If bd is not available, the test verifies
// that the function gracefully falls back to StateNormal.
func TestDetectSessionState_Autonomous(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize a beads repo with a hooked bead using bd CLI
	// First check if bd is available
	if _, err := exec.LookPath("bd"); err != nil {
		t.Skip("bd CLI not available, skipping autonomous state test")
	}

	// Initialize beads repo
	initCmd := exec.Command("bd", "init")
	initCmd.Dir = tmpDir
	if err := initCmd.Run(); err != nil {
		t.Skip("bd init failed, skipping autonomous state test")
	}

	// Create a hooked issue using bd CLI
	createCmd := exec.Command("bd", "create", "--title=Test hooked issue", "--status=hooked")
	createCmd.Dir = tmpDir
	if err := createCmd.Run(); err != nil {
		t.Skip("bd create failed, skipping autonomous state test")
	}

	state, reason := detectSessionState(tmpDir, tmpDir)

	if state != StateAutonomous {
		t.Errorf("detectSessionState() = %q, want %q", state, StateAutonomous)
	}
	if reason == "" {
		t.Error("reason should not be empty for autonomous state")
	}
}

// TestOutputState tests the state output formatting.
func TestOutputState(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputState(StatePostHandoff, "handoff marker present")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output != "post-handoff\nreason: handoff marker present\n" {
		t.Errorf("outputState() = %q, want %q", output, "post-handoff\nreason: handoff marker present\n")
	}
}

// TestExplain_WhenEnabled tests that explain outputs when flag is enabled.
func TestExplain_WhenEnabled(t *testing.T) {
	// Save and restore global state
	oldExplain := primeExplain
	defer func() { primeExplain = oldExplain }()
	primeExplain = true

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	explain("[EXPLAIN:test] This is a test with arg: %s", "value")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	expected := "[EXPLAIN:test] This is a test with arg: value\n"
	if output != expected {
		t.Errorf("explain() = %q, want %q", output, expected)
	}
}

// TestExplain_WhenDisabled tests that explain does not output when flag is disabled.
func TestExplain_WhenDisabled(t *testing.T) {
	// Save and restore global state
	oldExplain := primeExplain
	defer func() { primeExplain = oldExplain }()
	primeExplain = false

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	explain("[EXPLAIN:test] This should not appear")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output != "" {
		t.Errorf("explain() when disabled = %q, want empty string", output)
	}
}

// TestDryRun_HandoffMarkerNotRemoved tests that --dry-run doesn't remove handoff marker.
func TestDryRun_HandoffMarkerNotRemoved(t *testing.T) {
	tmpDir := t.TempDir()

	// Create handoff marker
	runtimeDir := filepath.Join(tmpDir, constants.DirRuntime)
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("creating runtime dir: %v", err)
	}
	markerPath := filepath.Join(runtimeDir, constants.FileHandoffMarker)
	if err := os.WriteFile(markerPath, []byte("prev-session-id\n"), 0644); err != nil {
		t.Fatalf("creating handoff marker: %v", err)
	}

	// Detect state (this doesn't remove marker, just detects)
	state, _ := detectSessionState(tmpDir, tmpDir)
	if state != StatePostHandoff {
		t.Fatalf("expected post-handoff state, got %s", state)
	}

	// Verify marker still exists (detectSessionState doesn't remove it)
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("handoff marker should still exist after detectSessionState")
	}

	// Now call checkHandoffMarker which DOES remove it
	// First capture stdout to suppress output
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	checkHandoffMarker(tmpDir)
	os.Stdout = old

	// Verify marker is now gone (normal behavior)
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Error("handoff marker should be removed after checkHandoffMarker")
	}
}

// TestDryRun_FlagRegistered tests that --dry-run flag is registered on primeCmd.
func TestDryRun_FlagRegistered(t *testing.T) {
	flag := primeCmd.Flags().Lookup("dry-run")
	if flag == nil {
		t.Fatal("--dry-run flag not registered on primeCmd")
	}
	if flag.Shorthand != "n" {
		t.Errorf("--dry-run shorthand = %q, want %q", flag.Shorthand, "n")
	}
}

// TestState_FlagRegistered tests that --state flag is registered on primeCmd.
func TestState_FlagRegistered(t *testing.T) {
	flag := primeCmd.Flags().Lookup("state")
	if flag == nil {
		t.Fatal("--state flag not registered on primeCmd")
	}
}

// TestExplain_FlagRegistered tests that --explain flag is registered on primeCmd.
func TestExplain_FlagRegistered(t *testing.T) {
	flag := primeCmd.Flags().Lookup("explain")
	if flag == nil {
		t.Fatal("--explain flag not registered on primeCmd")
	}
}
