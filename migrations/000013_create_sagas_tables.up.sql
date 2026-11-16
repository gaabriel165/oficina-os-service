CREATE TABLE sagas (
    id                    UUID PRIMARY KEY,
    service_order_id      UUID        NOT NULL UNIQUE REFERENCES service_orders(id) ON DELETE CASCADE,
    status                VARCHAR(30) NOT NULL,
    step                  VARCHAR(30) NOT NULL,
    pending_compensations TEXT        NOT NULL DEFAULT '',
    failure_reason        TEXT        NOT NULL DEFAULT '',
    deadline              TIMESTAMPTZ,
    version               INTEGER     NOT NULL DEFAULT 0,
    finished_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sagas_overdue ON sagas (deadline) WHERE status = 'running' AND deadline IS NOT NULL;

CREATE TABLE saga_steps (
    id          UUID PRIMARY KEY,
    saga_id     UUID         NOT NULL REFERENCES sagas(id) ON DELETE CASCADE,
    kind        VARCHAR(30)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    detail      TEXT         NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_saga_steps_saga ON saga_steps (saga_id, occurred_at);
