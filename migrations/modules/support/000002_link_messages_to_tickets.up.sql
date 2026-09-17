-- support_messages.ticket_id was never a foreign key, so the database accepted
-- messages pointing at a ticket that does not exist. That is how every ticket's
-- opening message came to be written with the zero UUID and stayed invisible
-- without a single error.
--
-- NOT VALID deliberately: it enforces the reference on every new and updated
-- row from now on, while leaving rows already detached in place. Those rows
-- hold customers' original problem descriptions and there is no reliable way to
-- re-attach them automatically — only created_at correlation, which is a guess.
-- Triage them by hand, then run:
--
--   ALTER TABLE support_messages VALIDATE CONSTRAINT support_messages_ticket_fk;
--
-- to close the constraint over the existing rows. Find them with:
--
--   SELECT m.* FROM support_messages m
--    WHERE NOT EXISTS (SELECT 1 FROM support_tickets t WHERE t.id = m.ticket_id);
ALTER TABLE support_messages
    ADD CONSTRAINT support_messages_ticket_fk
    FOREIGN KEY (ticket_id) REFERENCES support_tickets (id) ON DELETE CASCADE
    NOT VALID;
