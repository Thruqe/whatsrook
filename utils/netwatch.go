// Network connectivity monitoring and process auto-pause manager.
package utils

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// NetworkState tracks the global connectivity and pause status of the process.
type NetworkState struct {
	mu           sync.RWMutex
	paused       atomic.Bool
	manualPause  bool
	reason       string
	latency      time.Duration
	lastCheck    time.Time
	failureCount int
}

var globalNetState NetworkState

// IsNetworkPaused returns true if process execution is currently paused due to
// network disconnection, poor network quality, or manual pause.
func IsNetworkPaused() bool {
	return globalNetState.paused.Load()
}

// GetNetworkStatus returns whether process is paused, the pause reason, and last latency.
func GetNetworkStatus() (paused bool, reason string, latency time.Duration) {
	globalNetState.mu.RLock()
	defer globalNetState.mu.RUnlock()
	return globalNetState.paused.Load(), globalNetState.reason, globalNetState.latency
}

// SetNetworkPaused manually overrides the pause state.
func SetNetworkPaused(paused bool, reason string) {
	globalNetState.mu.Lock()
	defer globalNetState.mu.Unlock()
	globalNetState.manualPause = paused
	globalNetState.paused.Store(paused)
	if paused {
		if reason == "" {
			reason = "manually paused by user request"
		}
		globalNetState.reason = reason
		slog.Warn("process operations manually paused", "reason", reason)
	} else {
		globalNetState.reason = ""
		globalNetState.failureCount = 0
		slog.Info("process operations manually resumed")
	}
}

// ToggleManualPause toggles manual pause state and returns new state.
func ToggleManualPause() bool {
	globalNetState.mu.Lock()
	defer globalNetState.mu.Unlock()

	newState := !globalNetState.manualPause
	globalNetState.manualPause = newState
	globalNetState.paused.Store(newState)

	if newState {
		globalNetState.reason = "manually paused by user request"
		slog.Warn("process operations manually paused")
	} else {
		globalNetState.reason = ""
		globalNetState.failureCount = 0
		slog.Info("process operations manually resumed")
	}

	return newState
}

// StartNetworkGuard starts a background goroutine that periodically tests network
// health and latency. If the network becomes unreachable or latency exceeds 3s,
// the process auto-pauses. When connectivity is restored with acceptable latency,
// the process automatically resumes.
func StartNetworkGuard(ctx context.Context, checkInterval time.Duration) {
	if checkInterval <= 0 {
		checkInterval = 10 * time.Second
	}

	go func() {
		slog.Info("starting background network guard", "interval", checkInterval)
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		// Initial check
		CheckNetworkHealth()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				CheckNetworkHealth()
			}
		}
	}()
}

// CheckNetworkHealth performs an immediate network ping and latency check.
func CheckNetworkHealth() {
	globalNetState.mu.Lock()
	if globalNetState.manualPause {
		globalNetState.mu.Unlock()
		return
	}
	globalNetState.mu.Unlock()

	endpoints := []string{"web.whatsapp.com:443", "1.1.1.1:53", "8.8.8.8:53"}
	timeout := 3 * time.Second
	maxLatency := 3 * time.Second

	var bestLatency time.Duration
	var lastErr error
	success := false

	for _, ep := range endpoints {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", ep, timeout)
		if err == nil {
			dur := time.Since(start)
			_ = conn.Close()
			if !success || dur < bestLatency {
				bestLatency = dur
			}
			success = true
			break
		}
		lastErr = err
	}

	globalNetState.mu.Lock()
	defer globalNetState.mu.Unlock()

	globalNetState.lastCheck = time.Now()
	globalNetState.latency = bestLatency

	if success && bestLatency <= maxLatency {
		globalNetState.failureCount = 0
		if globalNetState.paused.Load() {
			globalNetState.paused.Store(false)
			globalNetState.reason = ""
			slog.Info("network connection restored — resuming process operations", "latency", bestLatency.String())
		}
	} else {
		globalNetState.failureCount++
		// Pause if 2 consecutive checks fail or latency exceeds limit
		if globalNetState.failureCount >= 2 && !globalNetState.paused.Load() {
			globalNetState.paused.Store(true)
			if !success {
				globalNetState.reason = fmt.Sprintf("network unreachable (%v)", lastErr)
			} else {
				globalNetState.reason = fmt.Sprintf("network connection too poor (latency %v > %v limit)", bestLatency, maxLatency)
			}
			slog.Warn("network connection lost or too poor — pausing process operations", "reason", globalNetState.reason, "failures", globalNetState.failureCount)
		}
	}
}
