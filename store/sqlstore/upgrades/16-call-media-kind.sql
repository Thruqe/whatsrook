-- v16 (compatible with v8+): Support saving both audio and video call media per sender
CREATE TABLE call_media_config (
    our_jid    TEXT    NOT NULL,
    sender     TEXT    NOT NULL,
    kind       TEXT    NOT NULL DEFAULT 'audio',
    file_path  TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (our_jid, sender, kind),
    FOREIGN KEY (our_jid) REFERENCES device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);