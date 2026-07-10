package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	putCallAudioConfigQuery = `
		INSERT INTO call_audio_config (our_jid, sender, file_path, updated_at) VALUES ($1, $2, $3, $4)
		ON CONFLICT (our_jid, sender) DO UPDATE SET file_path=excluded.file_path, updated_at=excluded.updated_at
	`
	getCallAudioConfigQuery    = `SELECT file_path FROM call_audio_config WHERE our_jid=$1 AND sender=$2`
	deleteCallAudioConfigQuery = `DELETE FROM call_audio_config WHERE our_jid=$1 AND sender=$2`
)

// PutCallAudioConfig saves (or updates) the default call audio file path for a given sender.
func (s *SQLStore) PutCallAudioConfig(ctx context.Context, sender types.JID, filePath string) error {
	_, err := s.db.Exec(ctx, putCallAudioConfigQuery, s.JID, sender.ToNonAD().String(), filePath, time.Now().Unix())
	return err
}

// GetCallAudioConfig returns the saved default audio file path for a sender, if any.
func (s *SQLStore) GetCallAudioConfig(ctx context.Context, sender types.JID) (string, error) {
	var path string
	err := s.db.QueryRow(ctx, getCallAudioConfigQuery, s.JID, sender.ToNonAD().String()).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return path, nil
}

// DeleteCallAudioConfig removes a sender's saved default audio config.
func (s *SQLStore) DeleteCallAudioConfig(ctx context.Context, sender types.JID) error {
	_, err := s.db.Exec(ctx, deleteCallAudioConfigQuery, s.JID, sender)
	return err
}
