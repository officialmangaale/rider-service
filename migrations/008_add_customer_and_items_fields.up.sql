-- =====================================================
-- Migration 008: Add customer contact and items fields
-- Enriches delivery_orders for rider-app display
-- =====================================================

ALTER TABLE public.delivery_orders
ADD COLUMN IF NOT EXISTS customer_name VARCHAR(200) NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS customer_phone VARCHAR(30) NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS items_summary TEXT NOT NULL DEFAULT '',
ADD COLUMN IF NOT EXISTS rider_arrived_at TIMESTAMPTZ;
