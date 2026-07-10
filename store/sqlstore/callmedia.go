package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go.mau.fi/whatsmeow/types"
)

type CallMediaKind string

const (
	CallMediaAudio CallMediaKind = "audio"
	CallMediaVideo CallMediaKind = "video"
)

const (
	putCallMediaConfigQuery = `
		INSERT INTO call_media_config (our_jid, sender, kind, file_path, updated_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (our_jid, sender, kind) DO UPDATE SET file_path=excluded.file_path, updated_at=excluded.updated_at
	`
	getCallMediaConfigQuery    = `SELECT file_path FROM call_media_config WHERE our_jid=$1 AND sender=$2 AND kind=$3`
	deleteCallMediaConfigQuery = `DELETE FROM call_media_config WHERE our_jid=$1 AND sender=$2 AND kind=$3`
)

// PutCallMediaConfig saves (or updates) the default call media file path for a sender+kind.
func (s *SQLStore) PutCallMediaConfig(ctx context.Context, sender types.JID, kind CallMediaKind, filePath string) error {
	_, err := s.db.Exec(ctx, putCallMediaConfigQuery, s.JID, sender.ToNonAD().String(), string(kind), filePath, time.Now().Unix())
	return err
}

// GetCallMediaConfig returns the saved default media file path for a sender+kind, if any.
func (s *SQLStore) GetCallMediaConfig(ctx context.Context, sender types.JID, kind CallMediaKind) (string, error) {
	var path string
	err := s.db.QueryRow(ctx, getCallMediaConfigQuery, s.JID, sender.ToNonAD().String(), string(kind)).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	return path, nil
}

// DeleteCallMediaConfig removes a sender's saved default media config for a kind.
func (s *SQLStore) DeleteCallMediaConfig(ctx context.Context, sender types.JID, kind CallMediaKind) error {
	_, err := s.db.Exec(ctx, deleteCallMediaConfigQuery, s.JID, sender.ToNonAD().String(), string(kind))
	return err
}
