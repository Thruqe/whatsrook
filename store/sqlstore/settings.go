// Bot settings key-value store backed by SQLite.
package sqlstore

import (
	"context"
	"database/sql"
	"errors"
	"go.mau.fi/util/dbutil"
)

const (
	getSettingQuery = `SELECT value FROM bot_settings WHERE our_jid=$1 AND key=$2`
	putSettingQuery = `
		INSERT INTO bot_settings (our_jid, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (our_jid, key) DO UPDATE SET value=excluded.value`
	deleteSettingQuery = `DELETE FROM bot_settings WHERE our_jid=$1 AND key=$2`
)

// GetSetting returns the stored value for key, or "" if not set.
func (s *SQLStore) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRow(ctx, getSettingQuery, s.JID, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// PutSetting stores (or overwrites) a key→value pair for this session.
func (s *SQLStore) PutSetting(ctx context.Context, key, value string) error {
	_, err := s.db.Exec(ctx, putSettingQuery, s.JID, key, value)
	return err
}

// DeleteSetting removes a key for this session.
func (s *SQLStore) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.Exec(ctx, deleteSettingQuery, s.JID, key)
	return err
}

// GetDB returns the database connection.
func (s *SQLStore) GetDB() *dbutil.Database {
	return s.db
}
