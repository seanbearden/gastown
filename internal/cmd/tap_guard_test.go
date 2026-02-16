package cmd

import (
	"os"
	"testing"
)

func TestTapGuardTaskDispatch_BlocksWhenMayor(t *testing.T) {
	t.Setenv("GT_MAYOR", "true")

	err := runTapGuardTaskDispatch(nil, nil)
	if err == nil {
		t.Fatal("expected error (exit 2) when GT_MAYOR is set")
	}

	se, ok := err.(*SilentExitError)
	if !ok {
		t.Fatalf("expected SilentExitError, got %T: %v", err, err)
	}
	if se.Code != 2 {
		t.Errorf("expected exit code 2, got %d", se.Code)
	}
}

func TestTapGuardTaskDispatch_AllowsWhenNotMayor(t *testing.T) {
	// Ensure GT_MAYOR is not set
	os.Unsetenv("GT_MAYOR")

	err := runTapGuardTaskDispatch(nil, nil)
	if err != nil {
		t.Errorf("expected nil error when GT_MAYOR is not set, got %v", err)
	}
}

func TestTapGuardTaskDispatch_AllowsForCrew(t *testing.T) {
	t.Setenv("GT_CREW", "rhett")
	os.Unsetenv("GT_MAYOR")

	err := runTapGuardTaskDispatch(nil, nil)
	if err != nil {
		t.Errorf("expected nil error for crew member, got %v", err)
	}
}

func TestTapGuardTaskDispatch_AllowsForPolecat(t *testing.T) {
	t.Setenv("GT_POLECAT", "alpha")
	os.Unsetenv("GT_MAYOR")

	err := runTapGuardTaskDispatch(nil, nil)
	if err != nil {
		t.Errorf("expected nil error for polecat, got %v", err)
	}
}
