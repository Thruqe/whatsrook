-- v17 -> v18 (compatible with v8+): Bot settings key-value store
CREATE TABLE bot_settings (
    our_jid TEXT    NOT NULL,
    key     TEXT    NOT NULL,
    value   TEXT    NOT NULL,
    PRIMARY KEY (our_jid, key),
    FOREIGN KEY (our_jid) REFERENCES device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);
