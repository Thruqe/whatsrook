package store

import (
	"context"
	"testing"

	"go.mau.fi/libsignal/state/record"
	"go.mau.fi/util/exsync"
)

func TestSessionCacheHelpers(t *testing.T) {
	// A nil context or missing cache should return nil
	if cache := getSessionCache(context.Background()); cache != nil {
		t.Errorf("Expected nil cache, got %v", cache)
	}

	if sess := getCachedSession(context.Background(), "addr"); sess != nil {
		t.Errorf("Expected nil session, got %v", sess)
	}

	if ok := putCachedSession(context.Background(), "addr", nil); ok {
		t.Error("Expected putCachedSession to fail with empty cache context")
	}

	// Create context with cache
	cache := exsync.NewMap[string, sessionCacheEntry]()
	ctx := context.WithValue(context.Background(), contextKeySessionCache, (*sessionCache)(cache))

	if retrieved := getSessionCache(ctx); retrieved != (*sessionCache)(cache) {
		t.Error("Failed to retrieve correct cache from context")
	}

	dummySession := record.NewSession(SignalProtobufSerializer.Session, SignalProtobufSerializer.State)
	if ok := putCachedSession(ctx, "test_addr", dummySession); !ok {
		t.Error("Failed to put cached session")
	}

	if retrievedSess := getCachedSession(ctx, "test_addr"); retrievedSess != dummySession {
		t.Errorf("Expected retrieved session %v, got %v", dummySession, retrievedSess)
	}
}
