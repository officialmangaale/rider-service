BEGIN;
SET LOCAL lock_timeout='5s';
-- Rates are configured explicitly per rider. Existing riders retain their
-- existing order-pricing policy until a contract and rollout are enabled.
CREATE TABLE IF NOT EXISTS rider_earning_contracts (
 rider_id TEXT PRIMARY KEY REFERENCES rider_work_profiles(rider_id),
 enabled BOOLEAN NOT NULL DEFAULT false,
 mode TEXT NOT NULL CHECK(mode IN ('per_delivery','salary')),
 base_cents BIGINT NOT NULL DEFAULT 0 CHECK(base_cents BETWEEN 0 AND 100000000),
 per_km_cents BIGINT NOT NULL DEFAULT 0 CHECK(per_km_cents BETWEEN 0 AND 100000000),
 salary_cents BIGINT NOT NULL DEFAULT 0 CHECK(salary_cents BETWEEN 0 AND 100000000),
 salary_period TEXT NOT NULL DEFAULT 'monthly' CHECK(salary_period IN ('weekly','monthly')),
 incentive_cents BIGINT NOT NULL DEFAULT 0 CHECK(incentive_cents BETWEEN 0 AND 100000000),
 incentive_permille BIGINT NOT NULL DEFAULT 0 CHECK(incentive_permille BETWEEN 0 AND 1000),
 cancellation JSONB NOT NULL DEFAULT '{}',
 version BIGINT NOT NULL DEFAULT 1,
 updated_by TEXT NOT NULL,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(incentive_cents=0 OR incentive_permille=0),
 CHECK((mode='salary' AND base_cents=0 AND per_km_cents=0) OR (mode='per_delivery' AND salary_cents=0 AND incentive_cents=0 AND incentive_permille=0))
);
CREATE TABLE IF NOT EXISTS rider_earning_contract_audit (
 id BIGSERIAL PRIMARY KEY,rider_id TEXT NOT NULL,version BIGINT NOT NULL,policy JSONB NOT NULL,actor_id TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(rider_id,version)
);
CREATE TABLE IF NOT EXISTS rider_offer_pay (
 request_id BIGINT PRIMARY KEY REFERENCES delivery_order_requests(request_id),
 rider_id TEXT NOT NULL,delivery_order_id BIGINT NOT NULL,
 amount_cents BIGINT NOT NULL CHECK(amount_cents>=0),
 incentive_cents BIGINT NOT NULL CHECK(incentive_cents>=0),
 tip_cents BIGINT NOT NULL CHECK(tip_cents>=0),
 policy JSONB NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS rider_assignment_pay (
 delivery_order_id BIGINT NOT NULL,rider_id TEXT NOT NULL,request_id BIGINT NOT NULL REFERENCES rider_offer_pay(request_id),
 amount_cents BIGINT NOT NULL, incentive_cents BIGINT NOT NULL,tip_cents BIGINT NOT NULL,policy JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),PRIMARY KEY(delivery_order_id,rider_id)
);
CREATE TABLE IF NOT EXISTS rider_cancellation_pay_reviews (
 id BIGSERIAL PRIMARY KEY,delivery_order_id BIGINT NOT NULL,rider_id TEXT NOT NULL,
 stage TEXT NOT NULL,proposed_cents BIGINT NOT NULL CHECK(proposed_cents>=0),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','rejected')),
 reviewed_by TEXT,review_note TEXT NOT NULL DEFAULT '',reviewed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(delivery_order_id,rider_id)
);
CREATE TABLE IF NOT EXISTS rider_salary_accruals (
 id BIGSERIAL PRIMARY KEY,rider_id TEXT NOT NULL REFERENCES rider_work_profiles(rider_id),
 period_start DATE NOT NULL,period_end DATE NOT NULL,amount_cents BIGINT NOT NULL CHECK(amount_cents>=0),
 contract JSONB NOT NULL,created_by TEXT NOT NULL,note TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK(period_end>period_start),UNIQUE(rider_id,period_start)
);
CREATE OR REPLACE FUNCTION mangaale_immutable_rider_pay_quote() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Rider offer and assignment pay snapshots are immutable'; END $$;
DROP TRIGGER IF EXISTS rider_offer_pay_immutable ON rider_offer_pay;
CREATE TRIGGER rider_offer_pay_immutable BEFORE UPDATE OR DELETE ON rider_offer_pay FOR EACH ROW EXECUTE FUNCTION mangaale_immutable_rider_pay_quote();
DROP TRIGGER IF EXISTS rider_assignment_pay_immutable ON rider_assignment_pay;
CREATE TRIGGER rider_assignment_pay_immutable BEFORE UPDATE OR DELETE ON rider_assignment_pay FOR EACH ROW EXECUTE FUNCTION mangaale_immutable_rider_pay_quote();
CREATE OR REPLACE FUNCTION mangaale_audit_rider_earning_contract() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='UPDATE' THEN NEW.version:=OLD.version+1;END IF;
 NEW.updated_at:=now();
 INSERT INTO rider_earning_contract_audit(rider_id,version,policy,actor_id)VALUES(NEW.rider_id,NEW.version,to_jsonb(NEW),NEW.updated_by);
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS rider_earning_contract_audit ON rider_earning_contracts;
CREATE TRIGGER rider_earning_contract_audit BEFORE INSERT OR UPDATE ON rider_earning_contracts FOR EACH ROW EXECUTE FUNCTION mangaale_audit_rider_earning_contract();

CREATE OR REPLACE FUNCTION mangaale_queue_rider_cancellation_pay() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p rider_assignment_pay; amount BIGINT;
BEGIN
 IF NEW.delivery_status='cancelled' AND OLD.delivery_status<>'cancelled' THEN
  SELECT * INTO p FROM rider_assignment_pay WHERE delivery_order_id=NEW.delivery_order_id AND rider_id=COALESCE(OLD.assigned_rider_id,OLD.rider_user_id);
  IF FOUND THEN
   IF p.policy->>'mode'='salary' THEN
    amount:=(p.incentive_cents*COALESCE((p.policy->'cancellation'->OLD.delivery_status->>'incentive_permille')::bigint,0)+500)/1000;
   ELSE amount:=COALESCE((p.policy->'cancellation'->OLD.delivery_status->>'fixed_cents')::bigint,0); END IF;
   INSERT INTO rider_cancellation_pay_reviews(delivery_order_id,rider_id,stage,proposed_cents)
    VALUES(NEW.delivery_order_id,p.rider_id,OLD.delivery_status,amount) ON CONFLICT DO NOTHING;
  END IF;
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS rider_cancellation_pay_review ON delivery_orders;
CREATE TRIGGER rider_cancellation_pay_review AFTER UPDATE OF delivery_status ON delivery_orders FOR EACH ROW EXECUTE FUNCTION mangaale_queue_rider_cancellation_pay();
COMMIT;
