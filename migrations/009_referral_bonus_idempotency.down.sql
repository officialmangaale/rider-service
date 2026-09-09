-- Rollback for 009_referral_bonus_idempotency.up.sql.
--
-- Safe to run: the column is additive and nothing outside the referral bonus
-- path reads it. Dropping it does not affect existing earnings rows.
--
-- Run the index drop first so the column drop does not have to cascade.

DROP INDEX IF EXISTS idx_rider_earnings_reference_key;

ALTER TABLE rider_earnings
    DROP COLUMN IF EXISTS reference_key;
