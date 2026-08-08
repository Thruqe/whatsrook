-- v17: Key-value bot settings store per session.
CREATE TABLE IF NOT EXISTS bot_settings (
    our_jid TEXT NOT NULL,
    key     TEXT NOT NULL,
    value   TEXT NOT NULL,
    PRIMARY KEY (our_jid, key)
);
