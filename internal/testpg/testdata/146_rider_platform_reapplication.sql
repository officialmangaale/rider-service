BEGIN;
SET LOCAL lock_timeout = '5s';
CREATE TABLE IF NOT EXISTS rider_platform_access (
 rider_id TEXT PRIMARY KEY REFERENCES rider_work_profiles(rider_id),
 state TEXT NOT NULL CHECK(state IN ('reapply_required','pending','approved','rejected')),
 last_restaurant_id BIGINT REFERENCES restaurants(restaurant_id),
 requested_at TIMESTAMPTZ,
 reviewed_at TIMESTAMPTZ,
 reviewed_by TEXT,
 review_note TEXT NOT NULL DEFAULT '',
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS rider_platform_access_audit (
 id BIGSERIAL PRIMARY KEY,
 rider_id TEXT NOT NULL,
 state TEXT NOT NULL,
 restaurant_id BIGINT,
 actor_id TEXT NOT NULL,
 note TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS rider_platform_review_queue ON rider_platform_access(state,requested_at);

CREATE OR REPLACE FUNCTION mangaale_guard_rider_removal_workload() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.is_active AND COALESCE(OLD.status,'active')='active'
  AND (NOT NEW.is_active OR COALESCE(NEW.status,'active')<>'active') THEN
  PERFORM 1 FROM rider_work_profiles WHERE rider_id=NEW.rider_user_id FOR UPDATE;
  PERFORM 1 FROM rider_availability WHERE rider_id=NEW.rider_user_id FOR UPDATE;
  IF EXISTS(SELECT 1 FROM rider_availability WHERE rider_id=NEW.rider_user_id AND current_order_id IS NOT NULL)
   OR EXISTS(SELECT 1 FROM delivery_orders WHERE COALESCE(NULLIF(assigned_rider_id,''),rider_user_id)=NEW.rider_user_id
    AND delivery_status IN ('rider_assigned','rider_arrived_restaurant','picked_up','on_the_way')) THEN
   RAISE EXCEPTION 'Finish or unassign active deliveries before removing this rider' USING ERRCODE='P1003';
  END IF;
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS rider_removal_workload_guard ON restaurant_riders;
CREATE TRIGGER rider_removal_workload_guard BEFORE UPDATE ON restaurant_riders FOR EACH ROW EXECUTE FUNCTION mangaale_guard_rider_removal_workload();

CREATE OR REPLACE FUNCTION mangaale_rider_removed_from_restaurant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.is_active AND COALESCE(OLD.status,'active')='active'
    AND (NOT NEW.is_active OR COALESCE(NEW.status,'active')<>'active') THEN
  -- The existing membership BEFORE trigger locks the permanent profile and
  -- refuses removal while the restaurant rider has an unfinished order.
  INSERT INTO rider_platform_access(rider_id,state,last_restaurant_id)
  VALUES(NEW.rider_user_id,'reapply_required',NEW.restaurant_id)
  ON CONFLICT(rider_id) DO UPDATE SET state='reapply_required',last_restaurant_id=EXCLUDED.last_restaurant_id,
   requested_at=NULL,reviewed_at=NULL,reviewed_by=NULL,review_note='',updated_at=now();
  INSERT INTO rider_platform_access_audit(rider_id,state,restaurant_id,actor_id)
  VALUES(NEW.rider_user_id,'reapply_required',NEW.restaurant_id,'restaurant_owner');
  UPDATE rider_availability SET is_online=false,is_available=false,updated_at=now() WHERE rider_id=NEW.rider_user_id;
  UPDATE users SET is_available=false WHERE id::text=NEW.rider_user_id;
  UPDATE delivery_order_requests SET status='cancelled',updated_at=now() WHERE rider_id=NEW.rider_user_id AND status='pending';
  INSERT INTO rider_notifications(rider_id,title,body,type,data)
  VALUES(NEW.rider_user_id::uuid,'Restaurant membership ended',
   'Re-apply to Platform and wait for admin approval before going online for platform orders.',
   'platform_reapplication',jsonb_build_object('restaurant_id',NEW.restaurant_id,'state','reapply_required'));
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS rider_platform_reapplication_on_removal ON restaurant_riders;
CREATE TRIGGER rider_platform_reapplication_on_removal AFTER UPDATE ON restaurant_riders
 FOR EACH ROW EXECUTE FUNCTION mangaale_rider_removed_from_restaurant();

CREATE OR REPLACE FUNCTION mangaale_guard_platform_online() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.is_online THEN
  PERFORM 1 FROM rider_work_profiles WHERE rider_id=NEW.rider_id FOR UPDATE;
  IF EXISTS(SELECT 1 FROM rider_platform_access WHERE rider_id=NEW.rider_id AND state<>'approved')
   AND NOT EXISTS(SELECT 1 FROM restaurant_riders WHERE rider_user_id=NEW.rider_id AND is_active AND COALESCE(status,'active')='active') THEN
   RAISE EXCEPTION 'Re-apply to Platform and wait for admin approval before going online' USING ERRCODE='P1003';
  END IF;
 END IF;
 RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS platform_online_approval_guard ON rider_availability;
CREATE TRIGGER platform_online_approval_guard BEFORE INSERT OR UPDATE ON rider_availability
 FOR EACH ROW EXECUTE FUNCTION mangaale_guard_platform_online();
COMMIT;
