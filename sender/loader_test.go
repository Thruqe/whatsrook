package sender

import (
	"context"
	"testing"
	"time"
)

func TestLoaderLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmdCtx := &Context{
		Ctx: ctx,
	}

	loader := cmdCtx.StartLoader("Test task")
	if loader == nil {
		t.Fatalf("expected non-nil loader")
	}

	time.Sleep(100 * time.Millisecond)

	loader.Stop()
	loader.Done("Finished")
	loader.Delete()
}
