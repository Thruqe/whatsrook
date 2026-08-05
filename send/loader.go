package send

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// Braille spinner frames for smooth text animation
var loaderFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var (
	activeLoaders   sync.Map
	loaderIDCounter uint64
	loaderIDMu      sync.Mutex
)

func nextLoaderID() string {
	loaderIDMu.Lock()
	defer loaderIDMu.Unlock()
	loaderIDCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), loaderIDCounter)
}

// Loader represents an active background text animation editing a WhatsApp message.
type Loader struct {
	ctx        *PluginContext
	id         string
	msgID      types.MessageID
	delayTimer *time.Timer
	stopChan   chan struct{}
	active     bool
	stopped    bool
	mu         sync.Mutex
}

// StartLoader returns a Loader that displays an interactive loader with animation & Cancel button
// ONLY if the operation takes longer than 1.5 seconds.
func (ctx *PluginContext) StartLoader(initialText string) *Loader {
	loaderID := nextLoaderID()
	l := &Loader{
		ctx:      ctx,
		id:       loaderID,
		stopChan: make(chan struct{}),
	}

	activeLoaders.Store(loaderID, l)

	// Delay loader display by 1.5 seconds. If operation finishes earlier, no message is ever sent.
	l.delayTimer = time.AfterFunc(1500*time.Millisecond, func() {
		l.activate()
	})

	return l
}

func (l *Loader) activate() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return
	}
	l.active = true
	l.mu.Unlock()

	// Only loading animation frame, no text
	frame := loaderFrames[0]

	// Interactive button to cancel in-flight operation
	buttons := []struct{ ID, Text string }{
		{ID: "cancel_loader_" + l.id, Text: "Cancel"},
	}

	btnMsg := CreateInteractiveButtonMessage(frame, buttons)

	slog.Debug("StartLoader: activating delayed loader", "id", l.id)
	resp, err := l.ctx.Client.SendMessage(l.ctx.Ctx, l.ctx.Chat, btnMsg)
	if err != nil {
		slog.Debug("StartLoader: failed to send loader interactive button", "err", err)
		return
	}

	l.mu.Lock()
	l.msgID = resp.ID
	l.mu.Unlock()

	go l.run()
}

func (l *Loader) run() {
	ticker := time.NewTicker(1200 * time.Millisecond)
	defer ticker.Stop()

	frameIdx := 1
	for {
		select {
		case <-l.stopChan:
			return
		case <-l.ctx.Ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			if l.stopped || l.msgID == "" {
				l.mu.Unlock()
				return
			}
			frame := loaderFrames[frameIdx%len(loaderFrames)]
			frameIdx++
			msgID := l.msgID
			l.mu.Unlock()

			// Update message with ONLY the loading animation frame
			_, err := l.ctx.Edit(msgID, frame)
			if err != nil {
				slog.Debug("Loader animation edit failed", "msgID", msgID, "err", err)
			}
		}
	}
}

// Cancel cancels the running operation context associated with this loader.
func (l *Loader) Cancel() {
	l.Stop()
	if l.ctx != nil {
		l.ctx.Cancel()
	}
	l.mu.Lock()
	msgID := l.msgID
	l.mu.Unlock()

	if msgID != "" {
		_, _ = l.ctx.Edit(msgID, "Operation cancelled.")
	}
}

// CancelLoader looks up active loader by ID and cancels its operation.
func CancelLoader(id string) bool {
	val, ok := activeLoaders.Load(id)
	if !ok {
		return false
	}
	if l, ok := val.(*Loader); ok {
		l.Cancel()
		return true
	}
	return false
}

// MessageID returns the underlying MessageID of the loader message.
func (l *Loader) MessageID() types.MessageID {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.msgID
}

// Stop halts the delay timer and animation ticker.
func (l *Loader) Stop() {
	l.mu.Lock()
	if l.delayTimer != nil {
		l.delayTimer.Stop()
	}
	if !l.stopped {
		l.stopped = true
		close(l.stopChan)
		activeLoaders.Delete(l.id)
	}
	l.mu.Unlock()
}

// Done stops the animation ticker and updates the loader message with final text if active.
func (l *Loader) Done(finalText string) {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.mu.Unlock()

	if msgID != "" && finalText != "" {
		_, _ = l.ctx.Edit(msgID, finalText)
	}
}

// Delete stops the animation ticker and deletes the loader message from chat if active.
func (l *Loader) Delete() {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.mu.Unlock()

	if msgID != "" {
		_, _ = l.ctx.Delete(msgID)
	}
}
