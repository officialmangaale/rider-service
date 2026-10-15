-- Customer <-> assigned rider chat, one conversation per delivery (order).
--
-- A message records WHO sent it and which rider the delivery was assigned to at
-- that moment, so history survives a reassignment and a rider can only ever
-- read the messages exchanged while the delivery was theirs.
--
-- Additive: no existing table is touched. Apply by hand like the other
-- migrations.

CREATE TABLE IF NOT EXISTS delivery_chat_messages (
    message_id        BIGSERIAL PRIMARY KEY,
    order_id          INTEGER      NOT NULL,
    delivery_order_id INTEGER      NOT NULL,
    customer_id       TEXT         NOT NULL,
    rider_id          TEXT         NOT NULL,
    sender_role       TEXT         NOT NULL CHECK (sender_role IN ('customer', 'rider')),
    sender_id         TEXT         NOT NULL,
    body              TEXT         NOT NULL CHECK (char_length(body) BETWEEN 1 AND 500),
    client_message_id TEXT         NOT NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    read_at           TIMESTAMPTZ
);

-- Idempotency: the same client_message_id from the same sender on the same
-- order is the same message, however many times the request is retried.
CREATE UNIQUE INDEX IF NOT EXISTS uq_delivery_chat_client_message
    ON delivery_chat_messages (order_id, sender_id, client_message_id);

CREATE INDEX IF NOT EXISTS idx_delivery_chat_order
    ON delivery_chat_messages (order_id, message_id);

CREATE INDEX IF NOT EXISTS idx_delivery_chat_order_rider
    ON delivery_chat_messages (order_id, rider_id, message_id);
