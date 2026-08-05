-- v18 -> v19 (compatible with v8+): Participant activity tracking store
CREATE TABLE IF NOT EXISTS participant_activity (
    our_jid     TEXT    NOT NULL,
    chat_jid    TEXT    NOT NULL,
    user_jid    TEXT    NOT NULL,
    last_active BIGINT  NOT NULL,
    PRIMARY KEY (our_jid, chat_jid, user_jid),
    FOREIGN KEY (our_jid) REFERENCES device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);
