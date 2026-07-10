-- v13 (compatible with v8+): Add buffer for outgoing events to accept retry receipts
CREATE TABLE retry_buffer (
	our_jid    TEXT   NOT NULL,
	chat_jid   TEXT   NOT NULL,
	message_id TEXT   NOT NULL,
	format     TEXT   NOT NULL,
	plaintext  bytea  NOT NULL,
	timestamp  BIGINT NOT NULL,

	PRIMARY KEY (our_jid, chat_jid, message_id),
	FOREIGN KEY (our_jid) REFERENCES device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE INDEX retry_buffer_timestamp_idx ON retry_buffer (our_jid, timestamp);
