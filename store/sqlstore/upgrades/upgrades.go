// Copyright (c) 2025 Tulir Asokan
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package upgrades

import (
	"context"
	"embed"

	"go.mau.fi/util/dbutil"
)

var Table dbutil.UpgradeTable

//go:embed *.sql
var upgrades embed.FS

func init() {
	Table.RegisterFS(upgrades)

	// v16 → v17: migrate rows from call_audio_config → call_media_config and
	// drop the old table, but only if call_audio_config actually exists.
	//
	// We cannot do this in a .sql file because SQLite validates all table
	// references at parse time — even a WHERE subquery cannot prevent the
	// "no such table" error at the FROM clause.
	// Fresh installs that started at v14+ never had call_audio_config, so
	// this step is a safe no-op for them.
	Table.Register(16, 17, 8,
		"Migrate call_audio_config → call_media_config (no-op on fresh installs)",
		dbutil.TxnModeOn,
		func(ctx context.Context, db *dbutil.Database) error {
			exists, err := db.TableExists(ctx, "call_audio_config")
			if err != nil || !exists {
				return err // nothing to do
			}
			if _, err = db.Exec(ctx,
				`INSERT INTO call_media_config (our_jid, sender, kind, file_path, updated_at)
				 SELECT our_jid, sender, 'audio', file_path, updated_at FROM call_audio_config`,
			); err != nil {
				return err
			}
			_, err = db.Exec(ctx, "DROP TABLE call_audio_config")
			return err
		},
	)
}

