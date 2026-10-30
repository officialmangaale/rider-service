-- Shared restaurant/rider database. Additive: existing links and orders remain.
BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE users ADD COLUMN IF NOT EXISTS rider_work_type text CHECK(rider_work_type IN ('part_time','full_time'));
CREATE SEQUENCE rider_public_code_sequence START WITH 100001;
CREATE TABLE rider_work_profiles (
    rider_id text PRIMARY KEY,
    rider_code text NOT NULL DEFAULT ('MR' || nextval('rider_public_code_sequence')::text) UNIQUE,
    work_type text CHECK (work_type IN ('part_time','full_time')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION mangaale_keep_rider_code() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.rider_id IS DISTINCT FROM OLD.rider_id OR NEW.rider_code IS DISTINCT FROM OLD.rider_code THEN
  RAISE EXCEPTION 'Rider code is permanent';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER rider_code_immutable BEFORE UPDATE ON rider_work_profiles FOR EACH ROW EXECUTE FUNCTION mangaale_keep_rider_code();
INSERT INTO rider_work_profiles(rider_id,work_type)
SELECT id::text,rider_work_type FROM users WHERE primary_role IN ('rider','delivery_driver') ON CONFLICT DO NOTHING;
CREATE FUNCTION mangaale_sync_rider_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.primary_role IN ('rider','delivery_driver') THEN
  INSERT INTO rider_work_profiles(rider_id,work_type) VALUES(NEW.id::text,NEW.rider_work_type)
  ON CONFLICT(rider_id) DO UPDATE SET work_type=EXCLUDED.work_type;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER rider_profile_sync AFTER INSERT OR UPDATE OF rider_work_type ON users FOR EACH ROW EXECUTE FUNCTION mangaale_sync_rider_profile();
CREATE TABLE restaurant_rider_invitations (
    invitation_id bigserial PRIMARY KEY,
    restaurant_id bigint NOT NULL REFERENCES restaurants(restaurant_id),
    rider_id text NOT NULL REFERENCES rider_work_profiles(rider_id),
    invited_by text NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','declined','cancelled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days',
    responded_at timestamptz
);
CREATE UNIQUE INDEX rider_invitation_pending ON restaurant_rider_invitations(restaurant_id,rider_id) WHERE status='pending';
CREATE INDEX rider_invitation_inbox ON restaurant_rider_invitations(rider_id,created_at DESC);
CREATE FUNCTION mangaale_notify_rider_invitation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE restaurant_name text;
BEGIN
 SELECT name INTO restaurant_name FROM restaurants WHERE restaurant_id=NEW.restaurant_id;
 INSERT INTO rider_notifications(rider_id,title,body,type,data)
 VALUES(NEW.rider_id::uuid,'Restaurant hiring invitation','Do you want to join '||restaurant_name||' restaurant?','restaurant_invitation',
 jsonb_build_object('invitation_id',NEW.invitation_id,'restaurant_id',NEW.restaurant_id,'restaurant_name',restaurant_name));
 RETURN NEW;
END $$;
CREATE TRIGGER rider_invitation_notification AFTER INSERT ON restaurant_rider_invitations FOR EACH ROW EXECUTE FUNCTION mangaale_notify_rider_invitation();

-- All new membership changes serialize on the rider's permanent profile row.
-- Existing active links are grandfathered; accepting never removes them silently.
CREATE FUNCTION mangaale_answer_rider_invitation(invite bigint, rider text, accept boolean)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE i restaurant_rider_invitations; u users;
BEGIN
    PERFORM 1 FROM rider_work_profiles WHERE rider_id=rider FOR UPDATE;
    SELECT * INTO i FROM restaurant_rider_invitations WHERE invitation_id=invite AND rider_id=rider FOR UPDATE;
    IF NOT FOUND THEN RAISE EXCEPTION 'Invitation not found' USING ERRCODE='P1003'; END IF;
    IF (accept AND i.status='accepted') OR (NOT accept AND i.status='declined') THEN RETURN; END IF;
    IF i.status <> 'pending' OR i.expires_at <= now() THEN RAISE EXCEPTION 'Invitation expired or already answered' USING ERRCODE='P1003'; END IF;
    IF accept THEN
        SELECT * INTO u FROM users WHERE id::text=rider AND primary_role IN ('rider','delivery_driver') AND deleted_at IS NULL AND COALESCE(status,'active')='active';
        IF NOT FOUND THEN RAISE EXCEPTION 'Active rider account required' USING ERRCODE='P1003'; END IF;
        IF EXISTS(SELECT 1 FROM restaurant_riders WHERE rider_user_id=rider AND is_active AND COALESCE(status,'active')='active' AND restaurant_id<>i.restaurant_id) THEN
            RAISE EXCEPTION 'You are already hired by another restaurant' USING ERRCODE='P1003';
        END IF;
        -- Availability lock also used by platform acceptance; no joining mid-trip.
        PERFORM 1 FROM rider_availability WHERE rider_id=rider FOR UPDATE;
        IF EXISTS(SELECT 1 FROM rider_availability WHERE rider_id=rider AND current_order_id IS NOT NULL)
           OR EXISTS(SELECT 1 FROM delivery_orders WHERE COALESCE(NULLIF(assigned_rider_id,''),rider_user_id)=rider AND delivery_status IN ('rider_assigned','rider_arrived_restaurant','picked_up','on_the_way'))
           OR EXISTS(SELECT 1 FROM orders WHERE assigned_rider_user_id=rider AND lower(order_status::text) NOT IN ('completed','delivered','cancelled','rejected','failed')) THEN
            RAISE EXCEPTION 'Finish your active deliveries before joining' USING ERRCODE='P1003';
        END IF;
        UPDATE restaurant_rider_invitations SET status='accepted',responded_at=now() WHERE invitation_id=invite;
        INSERT INTO restaurant_riders(restaurant_id,rider_user_id,rider_name,rider_phone,rider_email,vehicle_type,vehicle_number,status,is_active,created_by_user_id)
        VALUES(i.restaurant_id,rider,COALESCE(u.display_name,concat_ws(' ',u.first_name,u.last_name)),u.phone,u.email,u.vehicle_type,u.vehicle_registration_number,'active',true,i.invited_by)
        ON CONFLICT(restaurant_id,rider_user_id) DO UPDATE SET status='active',is_active=true,updated_at=now();
        UPDATE restaurant_rider_invitations SET status='cancelled',responded_at=now() WHERE rider_id=rider AND status='pending';
        UPDATE delivery_order_requests SET status='cancelled',updated_at=now() WHERE rider_id=rider AND status='pending';
    ELSE
        UPDATE restaurant_rider_invitations SET status='declined',responded_at=now() WHERE invitation_id=invite;
    END IF;
END $$;

-- Block bypasses through old direct-link/reactivation endpoints while keeping
-- edits to live, existing links working. Deactivation requires no active trip.
CREATE FUNCTION mangaale_guard_rider_membership() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='UPDATE' AND (NEW.rider_user_id IS DISTINCT FROM OLD.rider_user_id OR NEW.restaurant_id IS DISTINCT FROM OLD.restaurant_id) THEN
        RAISE EXCEPTION 'Send a new invitation to change restaurant membership' USING ERRCODE='P1003';
    END IF;
    INSERT INTO rider_work_profiles(rider_id) VALUES(NEW.rider_user_id) ON CONFLICT DO NOTHING;
    PERFORM 1 FROM rider_work_profiles WHERE rider_id=NEW.rider_user_id FOR UPDATE;
    IF NEW.is_active AND COALESCE(NEW.status,'active')='active' AND
       (TG_OP='INSERT' OR NOT COALESCE(OLD.is_active,false) OR COALESCE(OLD.status,'active')<>'active') THEN
        IF NOT EXISTS(SELECT 1 FROM restaurant_rider_invitations WHERE rider_id=NEW.rider_user_id AND restaurant_id=NEW.restaurant_id AND status='accepted') THEN
            RAISE EXCEPTION 'Send a hiring invitation and wait for rider acceptance' USING ERRCODE='P1003';
        END IF;
        IF EXISTS(SELECT 1 FROM restaurant_riders WHERE rider_user_id=NEW.rider_user_id AND restaurant_id<>NEW.restaurant_id AND is_active AND COALESCE(status,'active')='active') THEN
            RAISE EXCEPTION 'Rider already belongs to another restaurant' USING ERRCODE='P1003';
        END IF;
    END IF;
    IF TG_OP='UPDATE' AND OLD.is_active AND (NOT NEW.is_active OR COALESCE(NEW.status,'active')<>'active') THEN
        IF EXISTS(SELECT 1 FROM orders WHERE assigned_rider_user_id=NEW.rider_user_id AND restaurant_id=NEW.restaurant_id AND lower(order_status::text) NOT IN ('completed','delivered','cancelled','rejected','failed')) THEN
            RAISE EXCEPTION 'Finish or unassign active deliveries before removing this rider' USING ERRCODE='P1003';
        END IF;
        UPDATE restaurant_rider_invitations SET status='cancelled' WHERE rider_id=NEW.rider_user_id AND restaurant_id=NEW.restaurant_id AND status='accepted';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER rider_membership_guard BEFORE INSERT OR UPDATE ON restaurant_riders FOR EACH ROW EXECUTE FUNCTION mangaale_guard_rider_membership();
COMMIT;
