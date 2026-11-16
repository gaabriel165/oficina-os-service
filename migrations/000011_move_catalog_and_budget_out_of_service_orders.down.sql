ALTER TABLE service_orders
    DROP COLUMN budget_id,
    DROP COLUMN payment_id,
    DROP COLUMN checkout_url,
    DROP COLUMN cancellation_reason;

CREATE TABLE parts (
    id             UUID PRIMARY KEY,
    name           VARCHAR(255)    NOT NULL,
    description    TEXT,
    unit_price     DECIMAL(10,2)   NOT NULL,
    stock_quantity INT             NOT NULL DEFAULT 0,
    created_at     TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE services (
    id                UUID PRIMARY KEY,
    name              VARCHAR(255)  NOT NULL,
    description       TEXT,
    labor_price       DECIMAL(10,2) NOT NULL,
    estimated_minutes INT           NOT NULL,
    created_at        TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE service_order_items (
    id               UUID PRIMARY KEY,
    service_order_id UUID          NOT NULL REFERENCES service_orders(id) ON DELETE CASCADE,
    service_id       UUID          NOT NULL REFERENCES services(id),
    service_name     VARCHAR(255)  NOT NULL,
    labor_price      DECIMAL(10,2) NOT NULL
);

CREATE TABLE service_order_parts (
    id               UUID PRIMARY KEY,
    service_order_id UUID          NOT NULL REFERENCES service_orders(id) ON DELETE CASCADE,
    part_id          UUID          NOT NULL REFERENCES parts(id),
    part_name        VARCHAR(255)  NOT NULL,
    quantity         INT           NOT NULL,
    unit_price       DECIMAL(10,2) NOT NULL
);

CREATE INDEX idx_service_order_items_service_order_id ON service_order_items (service_order_id);
CREATE INDEX idx_service_order_parts_service_order_id ON service_order_parts (service_order_id);
