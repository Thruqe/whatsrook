-- v16 (compatible with v8+): Support saving both audio and video call media per sender
CREATE TABLE call_media_config (
    our_jid    TEXT    NOT NULL,
    sender     TEXT    NOT NULL,
    kind       TEXT    NOT NULL DEFAULT audio,
    file_path  TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (our_jid, sender, kind),
    FOREIGN KEY (our_jid) REFERENCES device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);

INSERT INTO call_media_config (our_jid, sender, kind, file_path, updated_at)
SELECT our_jid, sender, 'audio', file_path, updated_at FROM call_audio_config;

DROP TABLE call_audio_config;