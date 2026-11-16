CREATE TABLE service_order_status_history (
    id               UUID PRIMARY KEY,
    service_order_id UUID        NOT NULL REFERENCES service_orders(id) ON DELETE CASCADE,
    from_status      VARCHAR(50) NOT NULL DEFAULT '',
    to_status        VARCHAR(50) NOT NULL,
    reason           TEXT        NOT NULL DEFAULT '',
    occurred_at      TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_service_order_status_history_order ON service_order_status_history (service_order_id, occurred_at);
