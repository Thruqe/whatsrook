-- v16: Call media configuration storage.
CREATE TABLE IF NOT EXISTS call_media_config (
    our_jid    TEXT    NOT NULL,
    sender     TEXT    NOT NULL,
    kind       TEXT    NOT NULL DEFAULT 'audio',
    file_path  TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (our_jid, sender, kind)
);
