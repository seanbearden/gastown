package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveBase(t *testing.T) {
	// Override gtDir for testing
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := DefaultBase()

	if err := SaveBase(cfg); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(BasePath()); err != nil {
		t.Fatalf("base config file not created: %v", err)
	}

	loaded, err := LoadBase()
	if err != nil {
		t.Fatalf("LoadBase failed: %v", err)
	}

	if len(loaded.SessionStart) != 1 {
		t.Errorf("expected 1 SessionStart hook, got %d", len(loaded.SessionStart))
	}
	if len(loaded.PreCompact) != 1 {
		t.Errorf("expected 1 PreCompact hook, got %d", len(loaded.PreCompact))
	}
	if len(loaded.UserPromptSubmit) != 1 {
		t.Errorf("expected 1 UserPromptSubmit hook, got %d", len(loaded.UserPromptSubmit))
	}
	if len(loaded.Stop) != 1 {
		t.Errorf("expected 1 Stop hook, got %d", len(loaded.Stop))
	}
}

func TestLoadSaveOverride(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &HooksConfig{
		PreToolUse: []HookEntry{
			{
				Matcher: "Bash(git push*)",
				Hooks:   []Hook{{Type: "command", Command: "echo blocked && exit 2"}},
			},
		},
	}

	if err := SaveOverride("crew", cfg); err != nil {
		t.Fatalf("SaveOverride failed: %v", err)
	}

	loaded, err := LoadOverride("crew")
	if err != nil {
		t.Fatalf("LoadOverride failed: %v", err)
	}

	if len(loaded.PreToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse hook, got %d", len(loaded.PreToolUse))
	}
	if loaded.PreToolUse[0].Matcher != "Bash(git push*)" {
		t.Errorf("expected matcher 'Bash(git push*)', got %q", loaded.PreToolUse[0].Matcher)
	}
}

func TestLoadSaveOverrideRigRole(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "echo gastown-crew"}}},
		},
	}

	if err := SaveOverride("gastown/crew", cfg); err != nil {
		t.Fatalf("SaveOverride failed: %v", err)
	}

	// Verify the file path uses __ separator
	expectedPath := filepath.Join(tmpDir, ".gt", "hooks-overrides", "gastown__crew.json")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("expected override file at %s: %v", expectedPath, err)
	}

	loaded, err := LoadOverride("gastown/crew")
	if err != nil {
		t.Fatalf("LoadOverride failed: %v", err)
	}

	if len(loaded.SessionStart) != 1 {
		t.Fatalf("expected 1 SessionStart hook, got %d", len(loaded.SessionStart))
	}
}

func TestLoadMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	_, err := LoadBase()
	if err == nil {
		t.Error("expected error loading missing base config")
	}

	_, err = LoadOverride("crew")
	if err == nil {
		t.Error("expected error loading missing override config")
	}
}

func TestValidTarget(t *testing.T) {
	tests := []struct {
		target string
		valid  bool
	}{
		{"crew", true},
		{"witness", true},
		{"refinery", true},
		{"polecats", true},
		{"mayor", true},
		{"deacon", true},
		{"rig", true},
		{"gastown/rig", true},
		{"gastown/crew", true},
		{"beads/witness", true},
		{"sky/polecats", true},
		{"wyvern/refinery", true},
		{"", false},
		{"invalid", false},
		{"gastown/invalid", false},
		{"/crew", false},
		{"gastown/", false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			if got := ValidTarget(tt.target); got != tt.valid {
				t.Errorf("ValidTarget(%q) = %v, want %v", tt.target, got, tt.valid)
			}
		})
	}
}

func TestGetApplicableOverrides(t *testing.T) {
	tests := []struct {
		target   string
		expected []string
	}{
		{"mayor", []string{"mayor"}},
		{"crew", []string{"crew"}},
		{"gastown/crew", []string{"crew", "gastown/crew"}},
		{"beads/witness", []string{"witness", "beads/witness"}},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := GetApplicableOverrides(tt.target)
			if len(got) != len(tt.expected) {
				t.Fatalf("GetApplicableOverrides(%q) returned %d items, want %d", tt.target, len(got), len(tt.expected))
			}
			for i, v := range got {
				if v != tt.expected[i] {
					t.Errorf("GetApplicableOverrides(%q)[%d] = %q, want %q", tt.target, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestDefaultBase(t *testing.T) {
	cfg := DefaultBase()

	if len(cfg.SessionStart) == 0 {
		t.Error("DefaultBase should have SessionStart hooks")
	}
	if len(cfg.PreCompact) == 0 {
		t.Error("DefaultBase should have PreCompact hooks")
	}
	if len(cfg.UserPromptSubmit) == 0 {
		t.Error("DefaultBase should have UserPromptSubmit hooks")
	}
	if len(cfg.Stop) == 0 {
		t.Error("DefaultBase should have Stop hooks")
	}

	// Verify gt prime is in SessionStart
	found := false
	for _, entry := range cfg.SessionStart {
		for _, h := range entry.Hooks {
			if h.Command != "" && len(h.Command) > 0 {
				found = true
			}
		}
	}
	if !found {
		t.Error("DefaultBase SessionStart should have a command")
	}
}

func TestMerge(t *testing.T) {
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "base-session"}}},
		},
		Stop: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "base-stop"}}},
		},
	}

	override := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "override-session"}}},
		},
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git*)", Hooks: []Hook{{Type: "command", Command: "block-git"}}},
		},
	}

	result := Merge(base, override)

	// SessionStart should be replaced by override
	if len(result.SessionStart) != 1 || result.SessionStart[0].Hooks[0].Command != "override-session" {
		t.Errorf("expected override SessionStart, got %v", result.SessionStart)
	}

	// Stop should be preserved from base (not in override)
	if len(result.Stop) != 1 || result.Stop[0].Hooks[0].Command != "base-stop" {
		t.Errorf("expected base Stop, got %v", result.Stop)
	}

	// PreToolUse should come from override
	if len(result.PreToolUse) != 1 || result.PreToolUse[0].Matcher != "Bash(git*)" {
		t.Errorf("expected override PreToolUse, got %v", result.PreToolUse)
	}

	// Original base should not be mutated
	if len(base.PreToolUse) != 0 {
		t.Error("Merge mutated the original base config")
	}
}

func TestMergeEmptyOverride(t *testing.T) {
	base := DefaultBase()
	override := &HooksConfig{}

	result := Merge(base, override)

	// Everything should be preserved from base
	if !HooksEqual(base, result) {
		t.Error("empty override should not change base config")
	}
}

func TestComputeExpected(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Save a base config
	base := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "base-cmd"}}},
		},
	}
	if err := SaveBase(base); err != nil {
		t.Fatalf("SaveBase failed: %v", err)
	}

	// Save a crew override
	crewOverride := &HooksConfig{
		PreToolUse: []HookEntry{
			{Matcher: "Bash(git*)", Hooks: []Hook{{Type: "command", Command: "crew-guard"}}},
		},
	}
	if err := SaveOverride("crew", crewOverride); err != nil {
		t.Fatalf("SaveOverride crew failed: %v", err)
	}

	// Save a gastown/crew override
	gcOverride := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "gastown-crew-session"}}},
		},
	}
	if err := SaveOverride("gastown/crew", gcOverride); err != nil {
		t.Fatalf("SaveOverride gastown/crew failed: %v", err)
	}

	// Compute expected for gastown/crew (should apply: base → crew → gastown/crew)
	expected, err := ComputeExpected("gastown/crew")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}

	// SessionStart should come from gastown/crew override (most specific)
	if len(expected.SessionStart) != 1 || expected.SessionStart[0].Hooks[0].Command != "gastown-crew-session" {
		t.Errorf("expected gastown/crew SessionStart, got %v", expected.SessionStart)
	}

	// PreToolUse should come from crew override
	if len(expected.PreToolUse) != 1 || expected.PreToolUse[0].Hooks[0].Command != "crew-guard" {
		t.Errorf("expected crew PreToolUse, got %v", expected.PreToolUse)
	}
}

func TestComputeExpectedNoBase(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// No base config, no overrides - should fall back to DefaultBase
	expected, err := ComputeExpected("mayor")
	if err != nil {
		t.Fatalf("ComputeExpected failed: %v", err)
	}

	defaultBase := DefaultBase()
	if !HooksEqual(expected, defaultBase) {
		t.Error("expected DefaultBase when no configs exist")
	}
}

func TestHooksEqual(t *testing.T) {
	a := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}
	b := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}
	c := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "different"}}},
		},
	}

	if !HooksEqual(a, b) {
		t.Error("identical configs should be equal")
	}
	if HooksEqual(a, c) {
		t.Error("different configs should not be equal")
	}
	if !HooksEqual(&HooksConfig{}, &HooksConfig{}) {
		t.Error("empty configs should be equal")
	}
}

func TestLoadSettings(t *testing.T) {
	tmpDir := t.TempDir()

	// Test loading existing file
	settings := SettingsJSON{
		EditorMode: "vim",
		Hooks: HooksConfig{
			SessionStart: []HookEntry{
				{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
			},
		},
	}
	data, _ := json.Marshal(settings)
	path := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	loaded, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if loaded.EditorMode != "vim" {
		t.Errorf("expected editorMode vim, got %q", loaded.EditorMode)
	}
	if len(loaded.Hooks.SessionStart) != 1 {
		t.Errorf("expected 1 SessionStart hook, got %d", len(loaded.Hooks.SessionStart))
	}

	// Test loading non-existent file (should return zero-value)
	missing, err := LoadSettings(filepath.Join(tmpDir, "missing.json"))
	if err != nil {
		t.Fatalf("LoadSettings missing file failed: %v", err)
	}
	if missing.EditorMode != "" || len(missing.Hooks.SessionStart) != 0 {
		t.Error("missing file should return zero-value SettingsJSON")
	}
}

func TestDiscoverTargets(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal workspace structure
	// mayor/town.json (workspace marker)
	os.MkdirAll(filepath.Join(tmpDir, "mayor"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "mayor", "town.json"), []byte(`{"name":"test"}`), 0644)

	// deacon directory
	os.MkdirAll(filepath.Join(tmpDir, "deacon"), 0755)

	// rig with crew and witness
	os.MkdirAll(filepath.Join(tmpDir, "testrig", "crew", "alice"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "testrig", "crew", "bob"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "testrig", "witness"), 0755)

	targets, err := DiscoverTargets(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverTargets failed: %v", err)
	}

	// Should find: mayor, deacon, testrig/rig, testrig/crew, testrig/crew (alice), testrig/crew (bob), testrig/witness
	if len(targets) < 5 {
		t.Errorf("expected at least 5 targets, got %d", len(targets))
		for _, tgt := range targets {
			t.Logf("  target: %s (key=%s)", tgt.DisplayKey(), tgt.Key)
		}
	}

	// Verify specific targets exist
	found := make(map[string]bool)
	for _, tgt := range targets {
		found[tgt.DisplayKey()] = true
	}

	for _, expected := range []string{"mayor", "deacon", "testrig/crew", "testrig/witness"} {
		if !found[expected] {
			t.Errorf("expected target %q not found", expected)
		}
	}
}

func TestTargetDisplayKey(t *testing.T) {
	tests := []struct {
		target   Target
		expected string
	}{
		{Target{Key: "mayor", Role: "mayor"}, "mayor"},
		{Target{Key: "gastown/crew", Rig: "gastown", Role: "crew"}, "gastown/crew"},
		{Target{Key: "beads/witness", Rig: "beads", Role: "witness"}, "beads/witness"},
	}

	for _, tt := range tests {
		if got := tt.target.DisplayKey(); got != tt.expected {
			t.Errorf("DisplayKey() = %q, want %q", got, tt.expected)
		}
	}
}

func TestGetSetEntries(t *testing.T) {
	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}

	// GetEntries for existing type
	entries := cfg.GetEntries("SessionStart")
	if len(entries) != 1 {
		t.Errorf("expected 1 SessionStart entry, got %d", len(entries))
	}

	// GetEntries for empty type
	entries = cfg.GetEntries("PreToolUse")
	if len(entries) != 0 {
		t.Errorf("expected 0 PreToolUse entries, got %d", len(entries))
	}

	// GetEntries for unknown type
	entries = cfg.GetEntries("Unknown")
	if entries != nil {
		t.Errorf("expected nil for unknown event type, got %v", entries)
	}

	// SetEntries
	cfg.SetEntries("PreToolUse", []HookEntry{
		{Matcher: "Bash(*)", Hooks: []Hook{{Type: "command", Command: "guard"}}},
	})
	if len(cfg.PreToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse entry after SetEntries, got %d", len(cfg.PreToolUse))
	}
}

func TestToMap(t *testing.T) {
	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "start"}}},
		},
		Stop: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "stop"}}},
		},
	}

	m := cfg.ToMap()
	if len(m) != 2 {
		t.Errorf("expected 2 entries in map, got %d", len(m))
	}
	if _, ok := m["SessionStart"]; !ok {
		t.Error("expected SessionStart in map")
	}
	if _, ok := m["Stop"]; !ok {
		t.Error("expected Stop in map")
	}
	if _, ok := m["PreToolUse"]; ok {
		t.Error("empty PreToolUse should not be in map")
	}
}

func TestAddEntry(t *testing.T) {
	cfg := &HooksConfig{}

	// Add first entry
	added := cfg.AddEntry("PreToolUse", HookEntry{
		Matcher: "Bash(git*)",
		Hooks:   []Hook{{Type: "command", Command: "guard"}},
	})
	if !added {
		t.Error("expected first entry to be added")
	}
	if len(cfg.PreToolUse) != 1 {
		t.Errorf("expected 1 PreToolUse entry, got %d", len(cfg.PreToolUse))
	}

	// Try adding duplicate matcher - should not add
	added = cfg.AddEntry("PreToolUse", HookEntry{
		Matcher: "Bash(git*)",
		Hooks:   []Hook{{Type: "command", Command: "different"}},
	})
	if added {
		t.Error("expected duplicate matcher to not be added")
	}
	if len(cfg.PreToolUse) != 1 {
		t.Errorf("expected still 1 PreToolUse entry, got %d", len(cfg.PreToolUse))
	}

	// Add different matcher - should add
	added = cfg.AddEntry("PreToolUse", HookEntry{
		Matcher: "Bash(rm*)",
		Hooks:   []Hook{{Type: "command", Command: "block"}},
	})
	if !added {
		t.Error("expected new matcher to be added")
	}
	if len(cfg.PreToolUse) != 2 {
		t.Errorf("expected 2 PreToolUse entries, got %d", len(cfg.PreToolUse))
	}
}

func TestMarshalConfig(t *testing.T) {
	cfg := &HooksConfig{
		SessionStart: []HookEntry{
			{Matcher: "", Hooks: []Hook{{Type: "command", Command: "test"}}},
		},
	}

	data, err := MarshalConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalConfig failed: %v", err)
	}

	// Should be pretty-printed
	if len(data) == 0 {
		t.Error("MarshalConfig returned empty data")
	}

	// Should be valid JSON that round-trips
	loaded := &HooksConfig{}
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatalf("round-trip failed: %v", err)
	}

	if len(loaded.SessionStart) != 1 {
		t.Errorf("round-trip lost SessionStart hooks")
	}
}
