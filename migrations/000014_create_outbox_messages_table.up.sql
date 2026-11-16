CREATE TABLE IF NOT EXISTS outbox_messages (
    id UUID PRIMARY KEY,
    destination VARCHAR(100) NOT NULL,
    message_type VARCHAR(100) NOT NULL,
    saga_id UUID NOT NULL,
    order_id UUID NOT NULL,
    payload JSONB NOT NULL,
    trace_context JSONB NOT NULL DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_messages_pending ON outbox_messages (created_at) WHERE published_at IS NULL;
