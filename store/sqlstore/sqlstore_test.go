package sqlstore

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func TestSQLStoreSettings(t *testing.T) {
	ctx := context.Background()

	// Initialize database in-memory
	container, err := New(ctx, "sqlite3", "file::memory:?cache=shared&_foreign_keys=on", waLog.Noop)
	if err != nil {
		t.Fatalf("Failed to initialize store container: %v", err)
	}
	defer container.Close()

	jid := types.NewJID("12345", types.DefaultUserServer)
	store := NewSQLStore(container, jid)
	_, err = store.GetDB().Exec(ctx, `
		INSERT INTO device (
			jid, registration_id, noise_key, identity_key, signed_pre_key, signed_pre_key_id, signed_pre_key_sig,
			adv_key, adv_details, adv_account_sig, adv_account_sig_key, adv_device_sig
		) VALUES (
			$1, 1, zeroblob(32), zeroblob(32), zeroblob(32), 1, zeroblob(64),
			zeroblob(32), zeroblob(0), zeroblob(64), zeroblob(32), zeroblob(64)
		)
	`, jid.String())
	if err != nil {
		t.Fatalf("Failed to insert mock device: %v", err)
	}

	// Test GetSetting on empty store
	val, err := store.GetSetting(ctx, "test_key")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "" {
		t.Errorf("Expected empty string for non-existent setting, got %q", val)
	}

	// Test PutSetting
	err = store.PutSetting(ctx, "test_key", "test_value")
	if err != nil {
		t.Fatalf("PutSetting failed: %v", err)
	}

	// Test GetSetting after Put
	val, err = store.GetSetting(ctx, "test_key")
	if err != nil {
		t.Fatalf("GetSetting failed: %v", err)
	}
	if val != "test_value" {
		t.Errorf("Expected 'test_value', got %q", val)
	}

	// Test DeleteSetting
	err = store.DeleteSetting(ctx, "test_key")
	if err != nil {
		t.Fatalf("DeleteSetting failed: %v", err)
	}

	// Verify it was deleted
	val, err = store.GetSetting(ctx, "test_key")
	if err != nil {
		t.Fatalf("GetSetting failed after delete: %v", err)
	}
	if val != "" {
		t.Errorf("Expected empty string after delete, got %q", val)
	}
}
