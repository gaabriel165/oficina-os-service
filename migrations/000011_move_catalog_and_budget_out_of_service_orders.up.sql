DROP TABLE IF EXISTS service_order_parts;
DROP TABLE IF EXISTS service_order_items;
DROP TABLE IF EXISTS parts;
DROP TABLE IF EXISTS services;

ALTER TABLE service_orders
    ADD COLUMN budget_id           UUID,
    ADD COLUMN payment_id          UUID,
    ADD COLUMN checkout_url        TEXT NOT NULL DEFAULT '',
    ADD COLUMN cancellation_reason TEXT NOT NULL DEFAULT '';
