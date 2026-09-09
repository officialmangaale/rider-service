-- Rider Service: idempotency key for referral bonus earnings.
--
-- ADDITIVE ONLY. Adds one nullable column and one partial unique index to the
-- existing rider_earnings table. No column is dropped, renamed or retyped, and
-- every existing row stays valid because the new column is NULL for them.
--
-- Why
-- ---
-- Rider referral rewards are paid as a `bonus` row in rider_earnings (the
-- 'bonus' type already exists in the table's CHECK constraint, so no
-- constraint change is needed). The referral domain already guarantees
-- exactly-once crediting via referral_reward_ledger.idempotency_key
-- (restaurant-service migration 088); this index is defence in depth at the
-- destination ledger, so a redelivered delivery-completed event can never
-- produce two bonus rows even if the referral layer is bypassed.
--
-- The index is partial: it constrains only rows that carry a reference key,
-- so ordinary delivery_fee/tip/incentive/penalty earnings are unaffected.
--
-- Rollback: see 009_referral_bonus_idempotency.down.sql.

ALTER TABLE rider_earnings
    ADD COLUMN IF NOT EXISTS reference_key TEXT;

COMMENT ON COLUMN rider_earnings.reference_key IS
    'Optional idempotency key for programmatically credited earnings (e.g. referral bonuses). NULL for ordinary delivery earnings.';

CREATE UNIQUE INDEX IF NOT EXISTS idx_rider_earnings_reference_key
    ON rider_earnings (reference_key)
    WHERE reference_key IS NOT NULL;
