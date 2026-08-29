ALTER TABLE public.delivery_orders
DROP COLUMN IF EXISTS customer_name,
DROP COLUMN IF EXISTS customer_phone,
DROP COLUMN IF EXISTS items_summary,
DROP COLUMN IF EXISTS rider_arrived_at;
