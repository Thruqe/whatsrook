-- v5: Update account JID format
UPDATE device SET jid=REPLACE(jid, '.0', '');
