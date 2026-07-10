-- v15 (compatible with v8+): Add call audio config table for saved call audio per-sender
CREATE TABLE call_audio_config (
    our_jid    TEXT    NOT NULL,
    sender     TEXT    NOT NULL,
    file_path  TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (our_jid, sender),
    FOREIGN KEY (our_jid) REFERENCES device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);