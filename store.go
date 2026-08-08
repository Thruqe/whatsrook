package whatsrook

import (
	"context"

	"whatsrook/wa-core/store/sqlstore"
	"whatsrook/wa-core/types"
)

// WhatsRook Custom Table Names
const (
	TableBotSettings         = "bot_settings"
	TableCallMediaConfig     = "call_media_config"
	TableParticipantActivity = "participant_activity"
)

// WhatsRook Bot Setting Keys
const (
	SettingPrefix   = "prefix"
	SettingBotMode  = "mode"
	SettingSudoers  = "sudoers"
	SettingDisabled = "disabled_commands"
)

// Store wraps custom WhatsRook bot database tables and settings operations.
type Store struct {
	SQLStore *sqlstore.SQLStore
}

// NewStore creates a Store instance wrapping the underlying SQL store.
func NewStore(sqlStore *sqlstore.SQLStore) *Store {
	return &Store{SQLStore: sqlStore}
}

// GetSetting retrieves a setting value from the bot_settings table.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	if s == nil || s.SQLStore == nil {
		return "", nil
	}
	return s.SQLStore.GetSetting(ctx, key)
}

// PutSetting saves a key-value setting to the bot_settings table.
func (s *Store) PutSetting(ctx context.Context, key, value string) error {
	if s == nil || s.SQLStore == nil {
		return nil
	}
	return s.SQLStore.PutSetting(ctx, key, value)
}

// DeleteSetting removes a key-value setting from the bot_settings table.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	if s == nil || s.SQLStore == nil {
		return nil
	}
	return s.SQLStore.DeleteSetting(ctx, key)
}

// GetCallMediaConfig retrieves call media configuration from the call_media_config table.
func (s *Store) GetCallMediaConfig(ctx context.Context, sender types.JID, kind sqlstore.CallMediaKind) (string, error) {
	if s == nil || s.SQLStore == nil {
		return "", nil
	}
	return s.SQLStore.GetCallMediaConfig(ctx, sender, kind)
}

// PutCallMediaConfig stores call media configuration in the call_media_config table.
func (s *Store) PutCallMediaConfig(ctx context.Context, sender types.JID, kind sqlstore.CallMediaKind, filePath string) error {
	if s == nil || s.SQLStore == nil {
		return nil
	}
	return s.SQLStore.PutCallMediaConfig(ctx, sender, kind, filePath)
}
