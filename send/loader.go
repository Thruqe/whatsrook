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

// Loader represents an active background text animation editing a WhatsApp message.
type Loader struct {
	ctx      *PluginContext
	msgID    types.MessageID
	baseText string
	stopChan chan struct{}
	stopped  bool
	mu       sync.Mutex
}

// StartLoader sends an initial status message and starts a background goroutine
// that periodically edits the message with an animated loader frame.
func (ctx *PluginContext) StartLoader(initialText string) *Loader {
	loader := &Loader{
		ctx:      ctx,
		baseText: initialText,
		stopChan: make(chan struct{}),
	}

	firstMsg := fmt.Sprintf("%s %s", initialText, loaderFrames[0])
	msgID, err := ctx.ReplyWithID(firstMsg)
	if err != nil {
		slog.Debug("StartLoader: failed to send initial loader message", "err", err)
		return loader
	}
	loader.msgID = msgID

	go loader.run()
	return loader
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
			newText := fmt.Sprintf("%s %s", l.baseText, frame)
			msgID := l.msgID
			l.mu.Unlock()

			_, err := l.ctx.Edit(msgID, newText)
			if err != nil {
				slog.Debug("Loader edit failed", "msgID", msgID, "err", err)
			}
		}
	}
}

// MessageID returns the underlying MessageID of the loader message.
func (l *Loader) MessageID() types.MessageID {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.msgID
}

// Stop halts the animation ticker.
func (l *Loader) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.stopped {
		l.stopped = true
		close(l.stopChan)
	}
}

// Done stops the animation ticker and updates the loader message with final text.
func (l *Loader) Done(finalText string) {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.mu.Unlock()

	if msgID != "" && finalText != "" {
		_, _ = l.ctx.Edit(msgID, finalText)
	}
}

// Delete stops the animation ticker and deletes the loader message from chat.
func (l *Loader) Delete() {
	l.Stop()
	l.mu.Lock()
	msgID := l.msgID
	l.mu.Unlock()

	if msgID != "" {
		_, _ = l.ctx.Delete(msgID)
	}
}
