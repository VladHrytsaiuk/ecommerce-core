-- Every warehouse identifier in this core is DEFAULT_WAREHOUSE_ID: checkout
-- reserves there, the admin catalog facade adjusts stock there, and returns
-- restock there. Nothing created that row.
--
-- .env.example shipped this exact UUID, it passed validation because it is a
-- well-formed UUID, and the store booted. The first stock adjustment and every
-- checkout reservation then failed on the foreign key to a warehouse that did
-- not exist — so a store following the documented setup could neither take
-- goods in nor sell them, and nothing at startup said why.
--
-- Seeding it here rather than asking an operator to write SQL keeps the shipped
-- default working with no manual step. A store with its own warehouse inserts
-- it and points DEFAULT_WAREHOUSE_ID at that one instead; this row is then
-- simply unused, not in the way.
INSERT INTO warehouses (id, code, name, is_active)
VALUES ('00000000-0000-4000-8000-000000000001', 'default', 'Default warehouse', TRUE)
ON CONFLICT (id) DO NOTHING;
