-- The returns module's four parent-child references were never foreign keys.
-- Nothing is wrong with the data: the repository sets return_request_id on
-- every item and history row explicitly, and the restock adapter carries it
-- from the item it read. These constraints exist so that stays true.
--
-- The same omission in the support module let every ticket's opening message be
-- written with a zero UUID and stay invisible for the life of the table, with
-- no error anywhere. A foreign key is what turns that class of mistake from a
-- silent data loss into a failed write.
--
-- Validated, not NOT VALID: rows written by this code are already consistent,
-- so if this migration fails it has found something worth stopping for.
--
-- Every referencing column already leads an index — return_items_request_idx,
-- return_status_history_request_created_idx,
-- return_restock_operations_request_idx, and the restock table's primary key —
-- so the cascade checks below do not need new ones.
ALTER TABLE return_items
    ADD CONSTRAINT return_items_request_fk
    FOREIGN KEY (return_request_id) REFERENCES return_requests (id) ON DELETE CASCADE;

ALTER TABLE return_status_history
    ADD CONSTRAINT return_status_history_request_fk
    FOREIGN KEY (return_request_id) REFERENCES return_requests (id) ON DELETE CASCADE;

ALTER TABLE return_restock_operations
    ADD CONSTRAINT return_restock_operations_request_fk
    FOREIGN KEY (return_request_id) REFERENCES return_requests (id) ON DELETE CASCADE;

ALTER TABLE return_restock_operations
    ADD CONSTRAINT return_restock_operations_item_fk
    FOREIGN KEY (return_item_id) REFERENCES return_items (id) ON DELETE CASCADE;
