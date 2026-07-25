package utils

import (
	"context"
	"testing"
	"time"
)

func TestNetworkStateAndManualPause(t *testing.T) {
	if IsNetworkPaused() {
		t.Fatalf("expected network to not be paused initially")
	}

	SetNetworkPaused(true, "test pause")
	if !IsNetworkPaused() {
		t.Fatalf("expected network to be paused after SetNetworkPaused(true)")
	}

	paused, reason, _ := GetNetworkStatus()
	if !paused || reason != "test pause" {
		t.Fatalf("unexpected network status: paused=%v, reason=%q", paused, reason)
	}

	SetNetworkPaused(false, "")
	if IsNetworkPaused() {
		t.Fatalf("expected network to be unpaused after SetNetworkPaused(false)")
	}
}

func TestToggleManualPause(t *testing.T) {
	SetNetworkPaused(false, "")

	newState := ToggleManualPause()
	if !newState || !IsNetworkPaused() {
		t.Fatalf("expected ToggleManualPause to pause process")
	}

	newState = ToggleManualPause()
	if newState || IsNetworkPaused() {
		t.Fatalf("expected ToggleManualPause to unpause process")
	}
}

func TestNetworkGuardExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartNetworkGuard(ctx, 100*time.Millisecond)
	time.Sleep(150 * time.Millisecond)

	// Verify CheckNetworkHealth completes without crash
	CheckNetworkHealth()
}
