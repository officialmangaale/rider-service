ALTER TABLE public.delivery_orders
DROP COLUMN IF EXISTS rider_user_id,
DROP COLUMN IF EXISTS assignment_type,
DROP COLUMN IF EXISTS restaurant_owned,
DROP COLUMN IF EXISTS restaurant_name,
DROP COLUMN IF EXISTS restaurant_phone,
DROP COLUMN IF EXISTS assigned_at;
