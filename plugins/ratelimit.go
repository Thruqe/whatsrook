// Per-user rate limiting for download-heavy commands.
package commands

import (
	"sync"
	"time"
)

type downloadLimiter struct {
	mu      sync.Mutex
	pending map[string]bool
	last    map[string]time.Time
}

var dlLimiter = &downloadLimiter{
	pending: make(map[string]bool),
	last:    make(map[string]time.Time),
}

func (l *downloadLimiter) Acquire(userJID string, cooldown time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending[userJID] {
		return false
	}
	if last, ok := l.last[userJID]; ok && time.Since(last) < cooldown {
		return false
	}
	l.pending[userJID] = true
	return true
}

func (l *downloadLimiter) Release(userJID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.pending, userJID)
	l.last[userJID] = time.Now()
}
