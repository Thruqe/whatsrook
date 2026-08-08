-- v18: Participant activity tracking.
CREATE TABLE IF NOT EXISTS participant_activity (
    our_jid     TEXT    NOT NULL,
    chat_jid    TEXT    NOT NULL,
    user_jid    TEXT    NOT NULL,
    last_active INTEGER NOT NULL,
    PRIMARY KEY (our_jid, chat_jid, user_jid)
);
