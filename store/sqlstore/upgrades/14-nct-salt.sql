-- v14 (compatible with v8+): Add NCT salt table for cstoken derivation
CREATE TABLE nct_salt (
	our_jid TEXT PRIMARY KEY,
	salt    bytea NOT NULL,
	FOREIGN KEY (our_jid) REFERENCES device(jid) ON DELETE CASCADE ON UPDATE CASCADE
);
