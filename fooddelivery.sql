--
-- PostgreSQL database dump
--

-- Dumped from database version 17.6
-- Dumped by pg_dump version 17.5

-- Started on 2026-03-20 23:04:35

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- TOC entry 2 (class 3079 OID 18605)
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- TOC entry 5511 (class 0 OID 0)
-- Dependencies: 2
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


--
-- TOC entry 3 (class 3079 OID 19092)
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- TOC entry 5512 (class 0 OID 0)
-- Dependencies: 3
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: 
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- TOC entry 1003 (class 1247 OID 18200)
-- Name: order_status_enum; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.order_status_enum AS ENUM (
    'pending',
    'placed',
    'accepted',
    'preparing',
    'ready',
    'out_for_delivery',
    'delivered',
    'cancelled'
);


ALTER TYPE public.order_status_enum OWNER TO postgres;

--
-- TOC entry 1051 (class 1247 OID 18522)
-- Name: payment_method_enum; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.payment_method_enum AS ENUM (
    'cash',
    'card',
    'upi',
    'wallet',
    'online'
);


ALTER TYPE public.payment_method_enum OWNER TO postgres;

--
-- TOC entry 1006 (class 1247 OID 18218)
-- Name: payment_status_enum; Type: TYPE; Schema: public; Owner: postgres
--

CREATE TYPE public.payment_status_enum AS ENUM (
    'pending',
    'paid',
    'failed',
    'refunded'
);


ALTER TYPE public.payment_status_enum OWNER TO postgres;

--
-- TOC entry 376 (class 1255 OID 18517)
-- Name: update_updated_at_column(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.update_updated_at_column() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.update_updated_at_column() OWNER TO postgres;

--
-- TOC entry 375 (class 1255 OID 19129)
-- Name: users_search_vector_update(); Type: FUNCTION; Schema: public; Owner: postgres
--

CREATE FUNCTION public.users_search_vector_update() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.search_vector := to_tsvector('english',
        COALESCE(NEW.first_name, '') || ' ' ||
        COALESCE(NEW.last_name, '') || ' ' ||
        COALESCE(NEW.email, '') || ' ' ||
        COALESCE(NEW.phone, '') || ' ' ||
        COALESCE(NEW.display_name, '') || ' ' ||
        COALESCE(NEW.business_name, '')
    );
    RETURN NEW;
END;
$$;


ALTER FUNCTION public.users_search_vector_update() OWNER TO postgres;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- TOC entry 265 (class 1259 OID 18739)
-- Name: api_idempotency_keys; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.api_idempotency_keys (
    tenant_id text NOT NULL,
    endpoint text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text,
    response_body jsonb,
    status_code integer,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.api_idempotency_keys OWNER TO postgres;

--
-- TOC entry 241 (class 1259 OID 18373)
-- Name: bank_verifications; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.bank_verifications (
    id integer NOT NULL,
    restaurant_id integer NOT NULL,
    contact_id character varying(255),
    fund_account_id character varying(255) NOT NULL,
    payout_id character varying(255) NOT NULL,
    expected_amount_paise bigint NOT NULL,
    confirmed boolean DEFAULT false,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    confirmed_at timestamp without time zone
);


ALTER TABLE public.bank_verifications OWNER TO postgres;

--
-- TOC entry 240 (class 1259 OID 18372)
-- Name: bank_verifications_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.bank_verifications_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.bank_verifications_id_seq OWNER TO postgres;

--
-- TOC entry 5513 (class 0 OID 0)
-- Dependencies: 240
-- Name: bank_verifications_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.bank_verifications_id_seq OWNED BY public.bank_verifications.id;


--
-- TOC entry 312 (class 1259 OID 19661)
-- Name: customer_devices; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.customer_devices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    customer_id uuid NOT NULL,
    device_id character varying(255) NOT NULL,
    restaurant_id integer NOT NULL,
    user_agent text DEFAULT ''::text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.customer_devices OWNER TO postgres;

--
-- TOC entry 314 (class 1259 OID 19709)
-- Name: customer_favorite_items; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.customer_favorite_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    customer_id uuid NOT NULL,
    menu_item_id integer NOT NULL,
    item_name character varying(255) DEFAULT ''::character varying NOT NULL,
    order_count integer DEFAULT 0,
    last_ordered_at timestamp with time zone
);


ALTER TABLE public.customer_favorite_items OWNER TO postgres;

--
-- TOC entry 267 (class 1259 OID 18761)
-- Name: customer_feedback; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.customer_feedback (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    order_id text,
    rating smallint NOT NULL,
    comment text,
    source text DEFAULT 'RECEIPT_QR'::text NOT NULL,
    customer_name text,
    customer_phone text,
    customer_email text,
    tags text[],
    feedback_status text DEFAULT 'OPEN'::text NOT NULL,
    internal_note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    ip_hash text,
    user_agent text,
    CONSTRAINT customer_feedback_rating_check CHECK (((rating >= 1) AND (rating <= 5)))
);


ALTER TABLE public.customer_feedback OWNER TO postgres;

--
-- TOC entry 313 (class 1259 OID 19687)
-- Name: customer_visits; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.customer_visits (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    customer_id uuid NOT NULL,
    restaurant_id integer NOT NULL,
    visit_type character varying(20) DEFAULT 'qrunch'::character varying,
    table_no integer,
    order_id integer,
    started_at timestamp with time zone DEFAULT now(),
    ended_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT customer_visits_visit_type_check CHECK (((visit_type)::text = ANY ((ARRAY['qrunch'::character varying, 'dine_in'::character varying, 'takeaway'::character varying, 'manual'::character varying])::text[])))
);


ALTER TABLE public.customer_visits OWNER TO postgres;

--
-- TOC entry 275 (class 1259 OID 18861)
-- Name: customer_wallet; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.customer_wallet (
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    customer_id text NOT NULL,
    balance numeric(18,6) DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.customer_wallet OWNER TO postgres;

--
-- TOC entry 311 (class 1259 OID 19630)
-- Name: customers; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.customers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    restaurant_id integer NOT NULL,
    phone_number character varying(20),
    name character varying(255) DEFAULT ''::character varying,
    total_visits integer DEFAULT 0,
    total_orders integer DEFAULT 0,
    avg_order_value numeric(10,2) DEFAULT 0,
    total_spent numeric(12,2) DEFAULT 0,
    last_visit_at timestamp with time zone,
    loyalty_status character varying(20) DEFAULT 'NEW'::character varying,
    reward_eligibility boolean DEFAULT false,
    customer_segment character varying(20) DEFAULT 'first_time'::character varying,
    tags text[] DEFAULT '{}'::text[],
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT customers_customer_segment_check CHECK (((customer_segment)::text = ANY ((ARRAY['first_time'::character varying, 'occasional'::character varying, 'regular'::character varying, 'vip'::character varying])::text[]))),
    CONSTRAINT customers_loyalty_status_check CHECK (((loyalty_status)::text = ANY ((ARRAY['NEW'::character varying, 'REGULAR'::character varying, 'VIP'::character varying, 'PREMIUM'::character varying])::text[])))
);


ALTER TABLE public.customers OWNER TO postgres;

--
-- TOC entry 259 (class 1259 OID 18653)
-- Name: daily_outlet_metrics; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.daily_outlet_metrics (
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    day date NOT NULL,
    revenue numeric(18,6) DEFAULT 0 NOT NULL,
    orders_count integer DEFAULT 0 NOT NULL,
    avg_ticket numeric(18,6) DEFAULT 0 NOT NULL,
    discount_total numeric(18,6) DEFAULT 0 NOT NULL,
    refund_total numeric(18,6) DEFAULT 0 NOT NULL,
    void_count integer DEFAULT 0 NOT NULL,
    cash_sales numeric(18,6) DEFAULT 0 NOT NULL,
    online_sales numeric(18,6) DEFAULT 0 NOT NULL,
    cogs_estimate numeric(18,6) DEFAULT 0 NOT NULL,
    gross_profit_estimate numeric(18,6) DEFAULT 0 NOT NULL,
    wastage_estimate numeric(18,6) DEFAULT 0 NOT NULL,
    variance_estimate numeric(18,6) DEFAULT 0 NOT NULL,
    health_score integer DEFAULT 100 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);


ALTER TABLE public.daily_outlet_metrics OWNER TO postgres;

--
-- TOC entry 268 (class 1259 OID 18776)
-- Name: feedback_events; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.feedback_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    order_id text,
    token text,
    event_type text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.feedback_events OWNER TO postgres;

--
-- TOC entry 288 (class 1259 OID 19015)
-- Name: foodshare_participants; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.foodshare_participants (
    session_id character varying(32) NOT NULL,
    user_id text NOT NULL,
    joined_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.foodshare_participants OWNER TO postgres;

--
-- TOC entry 287 (class 1259 OID 18995)
-- Name: foodshare_sessions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.foodshare_sessions (
    session_id character varying(32) NOT NULL,
    restaurant_id bigint NOT NULL,
    host_user_id text NOT NULL,
    group_name character varying(255) NOT NULL,
    max_participants integer NOT NULL,
    split_type character varying(20) NOT NULL,
    status character varying(20) DEFAULT 'OPEN'::character varying NOT NULL,
    invite_link character varying(1024),
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT foodshare_sessions_max_participants_check CHECK (((max_participants >= 2) AND (max_participants <= 100))),
    CONSTRAINT foodshare_sessions_split_type_check CHECK (((split_type)::text = ANY ((ARRAY['INDIVIDUAL'::character varying, 'EQUAL'::character varying])::text[]))),
    CONSTRAINT foodshare_sessions_status_check CHECK (((status)::text = ANY ((ARRAY['OPEN'::character varying, 'CLOSED'::character varying, 'CANCELLED'::character varying, 'EXPIRED'::character varying, 'COMPLETED'::character varying])::text[])))
);


ALTER TABLE public.foodshare_sessions OWNER TO postgres;

--
-- TOC entry 299 (class 1259 OID 19273)
-- Name: incentive_cycles; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.incentive_cycles (
    id integer NOT NULL,
    restaurant_id integer NOT NULL,
    cycle_number integer NOT NULL,
    start_date timestamp without time zone NOT NULL,
    end_date timestamp without time zone NOT NULL,
    current_sales numeric(10,2) DEFAULT 0.00,
    status character varying(50) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.incentive_cycles OWNER TO postgres;

--
-- TOC entry 298 (class 1259 OID 19272)
-- Name: incentive_cycles_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.incentive_cycles_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.incentive_cycles_id_seq OWNER TO postgres;

--
-- TOC entry 5514 (class 0 OID 0)
-- Dependencies: 298
-- Name: incentive_cycles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.incentive_cycles_id_seq OWNED BY public.incentive_cycles.id;


--
-- TOC entry 301 (class 1259 OID 19289)
-- Name: incentive_milestones; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.incentive_milestones (
    id integer NOT NULL,
    cycle_id integer NOT NULL,
    type character varying(50) NOT NULL,
    target_amount numeric(10,2) NOT NULL,
    reward_amount numeric(10,2) NOT NULL,
    status character varying(50) DEFAULT 'pending'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.incentive_milestones OWNER TO postgres;

--
-- TOC entry 300 (class 1259 OID 19288)
-- Name: incentive_milestones_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.incentive_milestones_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.incentive_milestones_id_seq OWNER TO postgres;

--
-- TOC entry 5515 (class 0 OID 0)
-- Dependencies: 300
-- Name: incentive_milestones_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.incentive_milestones_id_seq OWNED BY public.incentive_milestones.id;


--
-- TOC entry 250 (class 1259 OID 18478)
-- Name: ingredients; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.ingredients (
    ingredient_id integer NOT NULL,
    tenant_id integer DEFAULT 1,
    restaurant_id integer NOT NULL,
    name character varying(255) NOT NULL,
    sku character varying(100),
    base_unit_id integer NOT NULL,
    category character varying(100),
    subcategory character varying(100),
    description text,
    is_perishable boolean DEFAULT false,
    track_batch boolean DEFAULT false,
    track_serial boolean DEFAULT false,
    average_cost numeric(10,2) DEFAULT 0.00,
    last_purchase_cost numeric(10,2),
    preferred_vendor_id integer,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    created_by integer,
    reorder_level numeric(10,2) DEFAULT 0
);


ALTER TABLE public.ingredients OWNER TO postgres;

--
-- TOC entry 249 (class 1259 OID 18477)
-- Name: ingredients_ingredient_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.ingredients_ingredient_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.ingredients_ingredient_id_seq OWNER TO postgres;

--
-- TOC entry 5516 (class 0 OID 0)
-- Dependencies: 249
-- Name: ingredients_ingredient_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.ingredients_ingredient_id_seq OWNED BY public.ingredients.ingredient_id;


--
-- TOC entry 257 (class 1259 OID 18592)
-- Name: inventory_idempotency_keys; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.inventory_idempotency_keys (
    tenant_id bigint DEFAULT 1 NOT NULL,
    endpoint character varying(128) NOT NULL,
    idempotency_key character varying(128) NOT NULL,
    request_hash character varying(128),
    response_body jsonb,
    status_code integer,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.inventory_idempotency_keys OWNER TO postgres;

--
-- TOC entry 254 (class 1259 OID 18555)
-- Name: inventory_recipe_lines; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.inventory_recipe_lines (
    recipe_line_id bigint NOT NULL,
    recipe_id bigint NOT NULL,
    ingredient_id integer NOT NULL,
    qty numeric(18,6) NOT NULL,
    unit character varying(16) NOT NULL,
    wastage_pct numeric(8,4) DEFAULT 0 NOT NULL,
    rounding character varying(16) DEFAULT 'HALF_UP'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT inventory_recipe_lines_qty_check CHECK ((qty > (0)::numeric)),
    CONSTRAINT inventory_recipe_lines_wastage_pct_check CHECK ((wastage_pct >= (0)::numeric))
);


ALTER TABLE public.inventory_recipe_lines OWNER TO postgres;

--
-- TOC entry 253 (class 1259 OID 18554)
-- Name: inventory_recipe_lines_recipe_line_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.inventory_recipe_lines_recipe_line_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.inventory_recipe_lines_recipe_line_id_seq OWNER TO postgres;

--
-- TOC entry 5517 (class 0 OID 0)
-- Dependencies: 253
-- Name: inventory_recipe_lines_recipe_line_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.inventory_recipe_lines_recipe_line_id_seq OWNED BY public.inventory_recipe_lines.recipe_line_id;


--
-- TOC entry 252 (class 1259 OID 18535)
-- Name: inventory_recipes; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.inventory_recipes (
    recipe_id bigint NOT NULL,
    tenant_id bigint DEFAULT 1 NOT NULL,
    outlet_id character varying(64),
    menu_item_id integer NOT NULL,
    variant character varying(64) NOT NULL,
    version integer NOT NULL,
    version_note text,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.inventory_recipes OWNER TO postgres;

--
-- TOC entry 251 (class 1259 OID 18534)
-- Name: inventory_recipes_recipe_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.inventory_recipes_recipe_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.inventory_recipes_recipe_id_seq OWNER TO postgres;

--
-- TOC entry 5518 (class 0 OID 0)
-- Dependencies: 251
-- Name: inventory_recipes_recipe_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.inventory_recipes_recipe_id_seq OWNED BY public.inventory_recipes.recipe_id;


--
-- TOC entry 256 (class 1259 OID 18577)
-- Name: inventory_stock_ledger; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.inventory_stock_ledger (
    ledger_id bigint NOT NULL,
    tenant_id bigint DEFAULT 1 NOT NULL,
    outlet_id character varying(64),
    ingredient_id integer NOT NULL,
    entry_type character varying(32) NOT NULL,
    qty_base_unit numeric(18,6) NOT NULL,
    ref_type character varying(32) NOT NULL,
    ref_id character varying(128) NOT NULL,
    base_unit_symbol character varying(16),
    meta jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


ALTER TABLE public.inventory_stock_ledger OWNER TO postgres;

--
-- TOC entry 255 (class 1259 OID 18576)
-- Name: inventory_stock_ledger_ledger_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.inventory_stock_ledger_ledger_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.inventory_stock_ledger_ledger_id_seq OWNER TO postgres;

--
-- TOC entry 5519 (class 0 OID 0)
-- Dependencies: 255
-- Name: inventory_stock_ledger_ledger_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.inventory_stock_ledger_ledger_id_seq OWNED BY public.inventory_stock_ledger.ledger_id;


--
-- TOC entry 264 (class 1259 OID 18724)
-- Name: inventory_stock_snapshots; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.inventory_stock_snapshots (
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    ingredient_id bigint NOT NULL,
    on_hand numeric(18,6) DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.inventory_stock_snapshots OWNER TO postgres;

--
-- TOC entry 315 (class 1259 OID 19730)
-- Name: item_pairings; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.item_pairings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    restaurant_id integer NOT NULL,
    source_item_id integer NOT NULL,
    target_item_id integer NOT NULL,
    pairing_type character varying(30) DEFAULT 'pairing'::character varying NOT NULL,
    reason text DEFAULT ''::text,
    price_impact numeric(10,2) DEFAULT 0,
    co_order_count integer DEFAULT 0,
    priority integer DEFAULT 100,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT item_pairings_pairing_type_check CHECK (((pairing_type)::text = ANY ((ARRAY['pairing'::character varying, 'addon'::character varying, 'upgrade'::character varying, 'complete_meal'::character varying])::text[])))
);


ALTER TABLE public.item_pairings OWNER TO postgres;

--
-- TOC entry 277 (class 1259 OID 18880)
-- Name: loyalty_points; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.loyalty_points (
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    customer_id text NOT NULL,
    points integer DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.loyalty_points OWNER TO postgres;

--
-- TOC entry 233 (class 1259 OID 18280)
-- Name: menu_categories; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.menu_categories (
    category_id integer NOT NULL,
    restaurant_id integer NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    display_order integer DEFAULT 0,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    category_type character varying(20) DEFAULT 'regular'::character varying
);


ALTER TABLE public.menu_categories OWNER TO postgres;

--
-- TOC entry 232 (class 1259 OID 18279)
-- Name: menu_categories_category_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.menu_categories_category_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.menu_categories_category_id_seq OWNER TO postgres;

--
-- TOC entry 5520 (class 0 OID 0)
-- Dependencies: 232
-- Name: menu_categories_category_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.menu_categories_category_id_seq OWNED BY public.menu_categories.category_id;


--
-- TOC entry 293 (class 1259 OID 19203)
-- Name: menu_item_addons; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.menu_item_addons (
    addon_id integer NOT NULL,
    item_id bigint,
    category_id bigint,
    restaurant_id bigint NOT NULL,
    addon_name character varying(100) NOT NULL,
    addon_type character varying(50) DEFAULT 'topping'::character varying,
    price numeric(10,2) DEFAULT 0.00,
    description text,
    display_order integer DEFAULT 1,
    is_available boolean DEFAULT true,
    is_multiple_selection boolean DEFAULT true,
    max_quantity integer DEFAULT 10,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.menu_item_addons OWNER TO postgres;

--
-- TOC entry 292 (class 1259 OID 19202)
-- Name: menu_item_addons_addon_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.menu_item_addons_addon_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.menu_item_addons_addon_id_seq OWNER TO postgres;

--
-- TOC entry 5521 (class 0 OID 0)
-- Dependencies: 292
-- Name: menu_item_addons_addon_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.menu_item_addons_addon_id_seq OWNED BY public.menu_item_addons.addon_id;


--
-- TOC entry 303 (class 1259 OID 19304)
-- Name: menu_item_combos; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.menu_item_combos (
    combo_item_id bigint NOT NULL,
    combo_menu_item_id bigint NOT NULL,
    item_id bigint NOT NULL,
    variant_id bigint,
    quantity integer DEFAULT 1 NOT NULL,
    display_order integer DEFAULT 0 NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT menu_item_combos_quantity_check CHECK ((quantity > 0))
);


ALTER TABLE public.menu_item_combos OWNER TO postgres;

--
-- TOC entry 302 (class 1259 OID 19303)
-- Name: menu_item_combos_combo_item_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.menu_item_combos_combo_item_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.menu_item_combos_combo_item_id_seq OWNER TO postgres;

--
-- TOC entry 5522 (class 0 OID 0)
-- Dependencies: 302
-- Name: menu_item_combos_combo_item_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.menu_item_combos_combo_item_id_seq OWNED BY public.menu_item_combos.combo_item_id;


--
-- TOC entry 260 (class 1259 OID 18674)
-- Name: menu_item_daily_metrics; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.menu_item_daily_metrics (
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    day date NOT NULL,
    menu_item_id text NOT NULL,
    qty_sold numeric(18,6) DEFAULT 0 NOT NULL,
    revenue numeric(18,6) DEFAULT 0 NOT NULL,
    cogs_estimate numeric(18,6) DEFAULT 0 NOT NULL,
    gross_profit_estimate numeric(18,6) DEFAULT 0 NOT NULL,
    margin_pct numeric(9,4) DEFAULT 0 NOT NULL
);


ALTER TABLE public.menu_item_daily_metrics OWNER TO postgres;

--
-- TOC entry 291 (class 1259 OID 19177)
-- Name: menu_item_variants; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.menu_item_variants (
    variant_id integer NOT NULL,
    menu_item_id bigint NOT NULL,
    variant_name character varying(100) NOT NULL,
    variant_type character varying(50) DEFAULT 'size'::character varying,
    price numeric(10,2) NOT NULL,
    measurement_unit character varying(20) DEFAULT 'piece'::character varying,
    measurement_value numeric(10,2),
    description text,
    display_order integer DEFAULT 1,
    is_available boolean DEFAULT true,
    is_default boolean DEFAULT false,
    sku character varying(100),
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.menu_item_variants OWNER TO postgres;

--
-- TOC entry 290 (class 1259 OID 19176)
-- Name: menu_item_variants_variant_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.menu_item_variants_variant_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.menu_item_variants_variant_id_seq OWNER TO postgres;

--
-- TOC entry 5523 (class 0 OID 0)
-- Dependencies: 290
-- Name: menu_item_variants_variant_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.menu_item_variants_variant_id_seq OWNED BY public.menu_item_variants.variant_id;


--
-- TOC entry 235 (class 1259 OID 18297)
-- Name: menu_items; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.menu_items (
    item_id integer NOT NULL,
    restaurant_id integer NOT NULL,
    category_id integer,
    name character varying(255) NOT NULL,
    description text,
    price numeric(8,2) NOT NULL,
    image_url character varying(500),
    is_vegetarian boolean DEFAULT false,
    is_vegan boolean DEFAULT false,
    is_gluten_free boolean DEFAULT false,
    calories integer,
    preparation_time integer DEFAULT 15,
    is_available boolean DEFAULT true,
    display_order integer DEFAULT 0,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    is_qrunch boolean DEFAULT false,
    promo_eligible boolean DEFAULT false NOT NULL,
    promo_cost numeric(18,6) DEFAULT 0 NOT NULL,
    is_taxable boolean DEFAULT false,
    measurement_unit character varying(20) DEFAULT 'piece'::character varying,
    has_variants boolean DEFAULT false,
    has_addons boolean DEFAULT false
);


ALTER TABLE public.menu_items OWNER TO postgres;

--
-- TOC entry 234 (class 1259 OID 18296)
-- Name: menu_items_item_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.menu_items_item_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.menu_items_item_id_seq OWNER TO postgres;

--
-- TOC entry 5524 (class 0 OID 0)
-- Dependencies: 234
-- Name: menu_items_item_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.menu_items_item_id_seq OWNED BY public.menu_items.item_id;


--
-- TOC entry 262 (class 1259 OID 18697)
-- Name: notification_devices; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.notification_devices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    user_id text NOT NULL,
    platform text NOT NULL,
    push_token text NOT NULL,
    created_at timestamp with time zone NOT NULL
);


ALTER TABLE public.notification_devices OWNER TO postgres;

--
-- TOC entry 274 (class 1259 OID 18848)
-- Name: offer_applications; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.offer_applications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    order_id text NOT NULL,
    offer_id uuid NOT NULL,
    applied_at timestamp with time zone DEFAULT now() NOT NULL,
    computed_json jsonb NOT NULL,
    idempotency_key text NOT NULL
);


ALTER TABLE public.offer_applications OWNER TO postgres;

--
-- TOC entry 273 (class 1259 OID 18835)
-- Name: offer_benefits; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.offer_benefits (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    offer_id uuid NOT NULL,
    benefit_json jsonb NOT NULL
);


ALTER TABLE public.offer_benefits OWNER TO postgres;

--
-- TOC entry 272 (class 1259 OID 18822)
-- Name: offer_rules; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.offer_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    offer_id uuid NOT NULL,
    rule_json jsonb NOT NULL
);


ALTER TABLE public.offer_rules OWNER TO postgres;

--
-- TOC entry 317 (class 1259 OID 19801)
-- Name: offer_suggested_items; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.offer_suggested_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    offer_id uuid NOT NULL,
    menu_item_id integer NOT NULL,
    item_type character varying(20) DEFAULT 'suggested_addon'::character varying NOT NULL,
    reason text DEFAULT ''::text,
    quantity integer DEFAULT 1,
    display_order integer DEFAULT 0,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT offer_suggested_items_item_type_check CHECK (((item_type)::text = ANY ((ARRAY['combo_item'::character varying, 'suggested_addon'::character varying])::text[])))
);


ALTER TABLE public.offer_suggested_items OWNER TO postgres;

--
-- TOC entry 271 (class 1259 OID 18809)
-- Name: offers; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.offers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    name text NOT NULL,
    offer_type text NOT NULL,
    status text NOT NULL,
    start_at timestamp with time zone,
    end_at timestamp with time zone,
    priority integer DEFAULT 100 NOT NULL,
    stackable boolean DEFAULT false NOT NULL,
    created_by text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    trigger_context character varying(30) DEFAULT 'menu'::character varying,
    display_style character varying(30) DEFAULT 'badge'::character varying,
    recommendation_reason text DEFAULT ''::text,
    visit_min_count integer DEFAULT 0,
    customer_segment_in text[] DEFAULT '{}'::text[],
    anchor_price numeric(10,2),
    final_price numeric(10,2),
    saving_text character varying(100) DEFAULT ''::character varying,
    badge_text character varying(50) DEFAULT ''::character varying,
    subtitle text DEFAULT ''::text,
    personalization_eligible boolean DEFAULT false,
    visit_based boolean DEFAULT false,
    eligible_item_ids integer[] DEFAULT '{}'::integer[],
    visibility character varying(20) DEFAULT 'all'::character varying,
    CONSTRAINT offers_visibility_check CHECK (((visibility)::text = ANY ((ARRAY['all'::character varying, 'qrunch_only'::character varying, 'staff_only'::character varying])::text[])))
);


ALTER TABLE public.offers OWNER TO postgres;

--
-- TOC entry 297 (class 1259 OID 19253)
-- Name: order_item_addons; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.order_item_addons (
    id integer NOT NULL,
    order_item_id bigint NOT NULL,
    addon_id bigint NOT NULL,
    addon_name character varying(100),
    addon_price numeric(10,2),
    quantity integer DEFAULT 1,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.order_item_addons OWNER TO postgres;

--
-- TOC entry 296 (class 1259 OID 19252)
-- Name: order_item_addons_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.order_item_addons_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.order_item_addons_id_seq OWNER TO postgres;

--
-- TOC entry 5525 (class 0 OID 0)
-- Dependencies: 296
-- Name: order_item_addons_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.order_item_addons_id_seq OWNED BY public.order_item_addons.id;


--
-- TOC entry 295 (class 1259 OID 19234)
-- Name: order_item_variants; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.order_item_variants (
    id integer NOT NULL,
    order_item_id bigint NOT NULL,
    variant_id bigint NOT NULL,
    variant_name character varying(100),
    variant_price numeric(10,2),
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.order_item_variants OWNER TO postgres;

--
-- TOC entry 294 (class 1259 OID 19233)
-- Name: order_item_variants_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.order_item_variants_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.order_item_variants_id_seq OWNER TO postgres;

--
-- TOC entry 5526 (class 0 OID 0)
-- Dependencies: 294
-- Name: order_item_variants_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.order_item_variants_id_seq OWNED BY public.order_item_variants.id;


--
-- TOC entry 239 (class 1259 OID 18348)
-- Name: order_items; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.order_items (
    order_item_id integer NOT NULL,
    order_id integer NOT NULL,
    menu_item_id integer,
    quantity integer DEFAULT 1 NOT NULL,
    is_taxable boolean DEFAULT false NOT NULL,
    unit_price numeric(8,2) NOT NULL,
    total_price numeric(8,2) NOT NULL,
    special_instructions text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    name character varying(255),
    options jsonb
);


ALTER TABLE public.order_items OWNER TO postgres;

--
-- TOC entry 5527 (class 0 OID 0)
-- Dependencies: 239
-- Name: COLUMN order_items.is_taxable; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.order_items.is_taxable IS 'Snapshot of taxability at order time.';


--
-- TOC entry 238 (class 1259 OID 18347)
-- Name: order_items_order_item_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.order_items_order_item_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.order_items_order_item_id_seq OWNER TO postgres;

--
-- TOC entry 5528 (class 0 OID 0)
-- Dependencies: 238
-- Name: order_items_order_item_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.order_items_order_item_id_seq OWNED BY public.order_items.order_item_id;


--
-- TOC entry 237 (class 1259 OID 18324)
-- Name: orders; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.orders (
    order_id integer NOT NULL,
    customer_id character varying(255),
    restaurant_id integer NOT NULL,
    delivery_partner_id character varying(255),
    order_status character varying(50) DEFAULT 'pending'::character varying,
    payment_status character varying(50) DEFAULT 'pending'::character varying,
    subtotal numeric(10,2) DEFAULT 0 NOT NULL,
    tax_amount numeric(10,2) DEFAULT 0.00,
    delivery_fee numeric(8,2) DEFAULT 0.00,
    tip_amount numeric(8,2) DEFAULT 0.00,
    discount_amount numeric(8,2) DEFAULT 0.00,
    total_amount numeric(10,2) DEFAULT 0 NOT NULL,
    delivery_address text,
    delivery_latitude numeric(10,8),
    delivery_longitude numeric(11,8),
    special_instructions text,
    estimated_delivery_time timestamp without time zone,
    actual_delivery_time timestamp without time zone,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    is_qrunch boolean DEFAULT false,
    order_number character varying(50),
    order_type character varying(20) DEFAULT 'DELIVERY'::character varying,
    dining_session_id integer,
    metadata jsonb,
    qrunch_customer_name character varying(255),
    table_no integer,
    pay_by public.payment_method_enum DEFAULT 'cash'::public.payment_method_enum,
    cgst numeric(10,2) DEFAULT 0.00,
    sgst numeric(10,2) DEFAULT 0.00,
    delivery_address_id integer,
    kds_status character varying(20) DEFAULT 'NEW'::character varying,
    kds_priority character varying(10) DEFAULT 'NORMAL'::character varying,
    kds_station_ids text[] DEFAULT '{}'::text[],
    kds_notes text[] DEFAULT '{}'::text[],
    kds_chef character varying(255),
    kds_sla_minutes integer DEFAULT 15,
    user_id bigint,
    customer_uuid uuid,
    discount_breakdown jsonb
);


ALTER TABLE public.orders OWNER TO postgres;

--
-- TOC entry 5529 (class 0 OID 0)
-- Dependencies: 237
-- Name: COLUMN orders.pay_by; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.orders.pay_by IS 'Payment method used: cash, card, upi, wallet, online';


--
-- TOC entry 236 (class 1259 OID 18323)
-- Name: orders_order_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.orders_order_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.orders_order_id_seq OWNER TO postgres;

--
-- TOC entry 5530 (class 0 OID 0)
-- Dependencies: 236
-- Name: orders_order_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.orders_order_id_seq OWNED BY public.orders.order_id;


--
-- TOC entry 307 (class 1259 OID 19536)
-- Name: organization_users; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.organization_users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role character varying(50) DEFAULT 'org_member'::character varying NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT organization_users_role_check CHECK (((role)::text = ANY ((ARRAY['org_owner'::character varying, 'org_admin'::character varying, 'finance_admin'::character varying, 'operations_admin'::character varying, 'org_member'::character varying])::text[])))
);


ALTER TABLE public.organization_users OWNER TO postgres;

--
-- TOC entry 306 (class 1259 OID 19501)
-- Name: organizations; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.organizations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(255) NOT NULL,
    slug character varying(255),
    legal_name character varying(255),
    gst_number character varying(50),
    pan_number character varying(50),
    logo_url character varying(500),
    website character varying(500),
    timezone character varying(50) DEFAULT 'Asia/Kolkata'::character varying,
    currency character varying(10) DEFAULT 'INR'::character varying,
    default_tax_rate numeric(5,2) DEFAULT 0,
    default_service_charge numeric(5,2) DEFAULT 0,
    metadata jsonb DEFAULT '{}'::jsonb,
    status character varying(20) DEFAULT 'active'::character varying,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT organizations_status_check CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'inactive'::character varying, 'suspended'::character varying])::text[])))
);


ALTER TABLE public.organizations OWNER TO postgres;

--
-- TOC entry 309 (class 1259 OID 19582)
-- Name: outlet_menu_overrides; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.outlet_menu_overrides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    restaurant_id integer NOT NULL,
    menu_item_id integer NOT NULL,
    price_override numeric(8,2),
    is_available boolean,
    is_hidden boolean DEFAULT false,
    display_name_override character varying(255),
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.outlet_menu_overrides OWNER TO postgres;

--
-- TOC entry 270 (class 1259 OID 18796)
-- Name: outlet_offer_constraints; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.outlet_offer_constraints (
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    min_gross_margin_pct numeric(9,4) DEFAULT 20 NOT NULL,
    max_next_visit_credit numeric(18,6) DEFAULT 100 NOT NULL,
    max_daily_credit_per_customer numeric(18,6) DEFAULT 200 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.outlet_offer_constraints OWNER TO postgres;

--
-- TOC entry 269 (class 1259 OID 18787)
-- Name: outlet_review_settings; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.outlet_review_settings (
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    google_review_url text,
    rating_redirect_threshold smallint DEFAULT 4 NOT NULL,
    thank_you_message text,
    offer_message text,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.outlet_review_settings OWNER TO postgres;

--
-- TOC entry 310 (class 1259 OID 19607)
-- Name: outlet_settings_overrides; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.outlet_settings_overrides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    restaurant_id integer NOT NULL,
    setting_key character varying(100) NOT NULL,
    setting_value jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


ALTER TABLE public.outlet_settings_overrides OWNER TO postgres;

--
-- TOC entry 308 (class 1259 OID 19556)
-- Name: outlet_staff_assignments; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.outlet_staff_assignments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    organization_id uuid NOT NULL,
    user_id uuid NOT NULL,
    restaurant_id integer NOT NULL,
    role character varying(50) DEFAULT 'staff'::character varying NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT outlet_staff_assignments_role_check CHECK (((role)::text = ANY ((ARRAY['outlet_manager'::character varying, 'cashier'::character varying, 'kitchen_staff'::character varying, 'waiter'::character varying, 'delivery_staff'::character varying, 'support_staff'::character varying])::text[])))
);


ALTER TABLE public.outlet_staff_assignments OWNER TO postgres;

--
-- TOC entry 261 (class 1259 OID 18687)
-- Name: owner_insights; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.owner_insights (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    severity text NOT NULL,
    category text NOT NULL,
    title text NOT NULL,
    detail text NOT NULL,
    data jsonb,
    status text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);


ALTER TABLE public.owner_insights OWNER TO postgres;

--
-- TOC entry 278 (class 1259 OID 18889)
-- Name: owner_referral_codes; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.owner_referral_codes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    owner_user_id text NOT NULL,
    code text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.owner_referral_codes OWNER TO postgres;

--
-- TOC entry 279 (class 1259 OID 18900)
-- Name: owner_referrals; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.owner_referrals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    referrer_tenant_id text NOT NULL,
    referrer_owner_user_id text NOT NULL,
    referred_business_name text NOT NULL,
    referred_phone text,
    referred_email text,
    referral_code text NOT NULL,
    status text NOT NULL,
    referred_tenant_id text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.owner_referrals OWNER TO postgres;

--
-- TOC entry 285 (class 1259 OID 18960)
-- Name: payment_sessions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.payment_sessions (
    id bigint NOT NULL,
    order_id bigint NOT NULL,
    token_hash character varying(128) NOT NULL,
    amount numeric(10,2) NOT NULL,
    status character varying(20) DEFAULT 'PENDING'::character varying NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    paid_at timestamp with time zone,
    psp_txn_id character varying(128),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT payment_sessions_status_check CHECK (((status)::text = ANY ((ARRAY['PENDING'::character varying, 'PAID'::character varying, 'EXPIRED'::character varying, 'CANCELLED'::character varying])::text[])))
);


ALTER TABLE public.payment_sessions OWNER TO postgres;

--
-- TOC entry 284 (class 1259 OID 18959)
-- Name: payment_sessions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.payment_sessions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.payment_sessions_id_seq OWNER TO postgres;

--
-- TOC entry 5531 (class 0 OID 0)
-- Dependencies: 284
-- Name: payment_sessions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.payment_sessions_id_seq OWNED BY public.payment_sessions.id;


--
-- TOC entry 222 (class 1259 OID 17903)
-- Name: permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.permissions (
    id bigint NOT NULL,
    name character varying(100) NOT NULL,
    description text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.permissions OWNER TO postgres;

--
-- TOC entry 221 (class 1259 OID 17902)
-- Name: permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.permissions_id_seq OWNER TO postgres;

--
-- TOC entry 5532 (class 0 OID 0)
-- Dependencies: 221
-- Name: permissions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.permissions_id_seq OWNED BY public.permissions.id;


--
-- TOC entry 258 (class 1259 OID 18642)
-- Name: pos_events; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.pos_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    event_type text NOT NULL,
    event_time timestamp with time zone NOT NULL,
    actor_user_id text,
    actor_role text,
    ref_type text,
    ref_id text,
    amount numeric(18,6),
    meta jsonb
);


ALTER TABLE public.pos_events OWNER TO postgres;

--
-- TOC entry 283 (class 1259 OID 18941)
-- Name: qr_sessions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.qr_sessions (
    session_id bigint NOT NULL,
    restaurant_id bigint NOT NULL,
    table_no integer,
    qr_token character varying(120),
    status character varying(32) DEFAULT 'active'::character varying NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    expired_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.qr_sessions OWNER TO postgres;

--
-- TOC entry 282 (class 1259 OID 18940)
-- Name: qr_sessions_session_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.qr_sessions_session_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.qr_sessions_session_id_seq OWNER TO postgres;

--
-- TOC entry 5533 (class 0 OID 0)
-- Dependencies: 282
-- Name: qr_sessions_session_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.qr_sessions_session_id_seq OWNED BY public.qr_sessions.session_id;


--
-- TOC entry 304 (class 1259 OID 19474)
-- Name: qr_tokens; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.qr_tokens (
    token character varying(120) NOT NULL,
    restaurant_id bigint NOT NULL,
    table_number character varying(80),
    active boolean DEFAULT true NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.qr_tokens OWNER TO postgres;

--
-- TOC entry 266 (class 1259 OID 18747)
-- Name: rating_links; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.rating_links (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    order_id text,
    token text NOT NULL,
    expires_at timestamp with time zone,
    max_uses integer DEFAULT 50 NOT NULL,
    used_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.rating_links OWNER TO postgres;

--
-- TOC entry 316 (class 1259 OID 19764)
-- Name: recommendation_events; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.recommendation_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    restaurant_id integer NOT NULL,
    customer_id uuid,
    event_type character varying(30) NOT NULL,
    source_type character varying(30) DEFAULT 'offer'::character varying NOT NULL,
    source_id text,
    context character varying(30) DEFAULT 'menu'::character varying,
    item_ids integer[] DEFAULT '{}'::integer[],
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT recommendation_events_context_check CHECK (((context)::text = ANY ((ARRAY['menu'::character varying, 'cart'::character varying, 'checkout'::character varying, 'item_added'::character varying, 'review'::character varying, 'manual_order'::character varying])::text[]))),
    CONSTRAINT recommendation_events_event_type_check CHECK (((event_type)::text = ANY ((ARRAY['shown'::character varying, 'clicked'::character varying, 'applied'::character varying, 'converted'::character varying, 'dismissed'::character varying])::text[]))),
    CONSTRAINT recommendation_events_source_type_check CHECK (((source_type)::text = ANY ((ARRAY['offer'::character varying, 'pairing'::character varying, 'personalized'::character varying, 'popularity'::character varying, 'manual'::character varying])::text[])))
);


ALTER TABLE public.recommendation_events OWNER TO postgres;

--
-- TOC entry 281 (class 1259 OID 18926)
-- Name: referral_attribution_events; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.referral_attribution_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    referral_id uuid NOT NULL,
    event_type text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    meta jsonb
);


ALTER TABLE public.referral_attribution_events OWNER TO postgres;

--
-- TOC entry 280 (class 1259 OID 18913)
-- Name: referral_rewards; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.referral_rewards (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    referral_id uuid NOT NULL,
    reward_type text NOT NULL,
    amount numeric(18,6),
    months integer,
    issued_at timestamp with time zone,
    status text NOT NULL,
    meta jsonb
);


ALTER TABLE public.referral_rewards OWNER TO postgres;

--
-- TOC entry 229 (class 1259 OID 18249)
-- Name: restaurant_hours; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.restaurant_hours (
    id integer NOT NULL,
    restaurant_id integer NOT NULL,
    weekday integer NOT NULL,
    open_time time without time zone,
    close_time time without time zone,
    is_closed boolean DEFAULT false,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT restaurant_hours_weekday_check CHECK (((weekday >= 0) AND (weekday <= 6)))
);


ALTER TABLE public.restaurant_hours OWNER TO postgres;

--
-- TOC entry 228 (class 1259 OID 18248)
-- Name: restaurant_hours_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.restaurant_hours_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.restaurant_hours_id_seq OWNER TO postgres;

--
-- TOC entry 5534 (class 0 OID 0)
-- Dependencies: 228
-- Name: restaurant_hours_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.restaurant_hours_id_seq OWNED BY public.restaurant_hours.id;


--
-- TOC entry 305 (class 1259 OID 19487)
-- Name: restaurant_sessions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.restaurant_sessions (
    session_id character varying(64) NOT NULL,
    restaurant_id bigint NOT NULL,
    table_number character varying(80),
    customer_name character varying(120),
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    expires_at timestamp without time zone NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.restaurant_sessions OWNER TO postgres;

--
-- TOC entry 242 (class 1259 OID 18390)
-- Name: restaurant_staff; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.restaurant_staff (
    user_id uuid NOT NULL,
    restaurant_id integer NOT NULL,
    role_id integer NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    created_by_user_id uuid,
    id uuid DEFAULT gen_random_uuid() NOT NULL
);


ALTER TABLE public.restaurant_staff OWNER TO postgres;

--
-- TOC entry 231 (class 1259 OID 18264)
-- Name: restaurant_tables; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.restaurant_tables (
    id integer NOT NULL,
    restaurant_id integer NOT NULL,
    table_identifier character varying(100) NOT NULL,
    seats integer NOT NULL,
    qr_token character varying(100),
    qr_url character varying(500),
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.restaurant_tables OWNER TO postgres;

--
-- TOC entry 230 (class 1259 OID 18263)
-- Name: restaurant_tables_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.restaurant_tables_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.restaurant_tables_id_seq OWNER TO postgres;

--
-- TOC entry 5535 (class 0 OID 0)
-- Dependencies: 230
-- Name: restaurant_tables_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.restaurant_tables_id_seq OWNED BY public.restaurant_tables.id;


--
-- TOC entry 227 (class 1259 OID 18238)
-- Name: restaurants; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.restaurants (
    restaurant_id integer NOT NULL,
    owner_auth_user_id integer,
    user_id uuid,
    name character varying(255) NOT NULL,
    owner_name character varying(255),
    slug character varying(255),
    type character varying(100),
    category character varying(100),
    gst_number character varying(50),
    fssai_number character varying(50),
    logo_url character varying(500),
    background_url character varying(500),
    gst_certificate_url character varying(500),
    fssai_license_url character varying(500),
    aadhaar_card_url character varying(500),
    pan_card_url character varying(500),
    street_address character varying(255),
    city character varying(100),
    state character varying(100),
    postal_code character varying(20),
    latitude numeric(10,8),
    longitude numeric(11,8),
    status character varying(50),
    tags text[],
    metadata jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    is_qrunch_purchased boolean DEFAULT false,
    is_qrunch_requested boolean DEFAULT false,
    is_restaurant_registered boolean DEFAULT false,
    upi_vpa character varying(255),
    wallet_amount numeric(12,2) DEFAULT 0.00,
    organization_id uuid,
    outlet_code character varying(50),
    outlet_type character varying(30) DEFAULT 'company_owned'::character varying,
    supports_dine_in boolean DEFAULT true,
    supports_takeaway boolean DEFAULT true,
    supports_delivery boolean DEFAULT true,
    supports_qr_ordering boolean DEFAULT true,
    manager_user_id uuid,
    tax_rate numeric(5,2),
    service_charge_rate numeric(5,2),
    outlet_settings jsonb DEFAULT '{}'::jsonb,
    CONSTRAINT restaurants_outlet_type_check CHECK (((outlet_type)::text = ANY ((ARRAY['company_owned'::character varying, 'franchise'::character varying, 'virtual'::character varying])::text[])))
);


ALTER TABLE public.restaurants OWNER TO postgres;

--
-- TOC entry 226 (class 1259 OID 18237)
-- Name: restaurants_restaurant_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.restaurants_restaurant_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.restaurants_restaurant_id_seq OWNER TO postgres;

--
-- TOC entry 5536 (class 0 OID 0)
-- Dependencies: 226
-- Name: restaurants_restaurant_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.restaurants_restaurant_id_seq OWNED BY public.restaurants.restaurant_id;


--
-- TOC entry 223 (class 1259 OID 17915)
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.role_permissions (
    role_id bigint NOT NULL,
    permission_id bigint NOT NULL
);


ALTER TABLE public.role_permissions OWNER TO postgres;

--
-- TOC entry 220 (class 1259 OID 17890)
-- Name: roles; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.roles (
    id bigint NOT NULL,
    name character varying(50) NOT NULL,
    description text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.roles OWNER TO postgres;

--
-- TOC entry 219 (class 1259 OID 17889)
-- Name: roles_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.roles_id_seq OWNER TO postgres;

--
-- TOC entry 5537 (class 0 OID 0)
-- Dependencies: 219
-- Name: roles_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.roles_id_seq OWNED BY public.roles.id;


--
-- TOC entry 263 (class 1259 OID 18706)
-- Name: stock_alerts; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.stock_alerts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    ingredient_id bigint,
    severity text NOT NULL,
    status text NOT NULL,
    current_qty numeric(18,6) DEFAULT 0 NOT NULL,
    threshold_qty numeric(18,6) DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.stock_alerts OWNER TO postgres;

--
-- TOC entry 246 (class 1259 OID 18438)
-- Name: unit_conversions; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.unit_conversions (
    conversion_id integer NOT NULL,
    from_unit_id integer NOT NULL,
    to_unit_id integer NOT NULL,
    factor numeric(15,6) NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unit_conversions_check CHECK ((from_unit_id <> to_unit_id)),
    CONSTRAINT unit_conversions_factor_check CHECK ((factor > (0)::numeric))
);


ALTER TABLE public.unit_conversions OWNER TO postgres;

--
-- TOC entry 245 (class 1259 OID 18437)
-- Name: unit_conversions_conversion_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.unit_conversions_conversion_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.unit_conversions_conversion_id_seq OWNER TO postgres;

--
-- TOC entry 5538 (class 0 OID 0)
-- Dependencies: 245
-- Name: unit_conversions_conversion_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.unit_conversions_conversion_id_seq OWNED BY public.unit_conversions.conversion_id;


--
-- TOC entry 244 (class 1259 OID 18423)
-- Name: units; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.units (
    unit_id integer NOT NULL,
    name character varying(50) NOT NULL,
    symbol character varying(10) NOT NULL,
    unit_type character varying(20) NOT NULL,
    description text,
    is_active boolean DEFAULT true,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT units_unit_type_check CHECK (((unit_type)::text = ANY ((ARRAY['weight'::character varying, 'volume'::character varying, 'count'::character varying])::text[])))
);


ALTER TABLE public.units OWNER TO postgres;

--
-- TOC entry 243 (class 1259 OID 18422)
-- Name: units_unit_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.units_unit_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.units_unit_id_seq OWNER TO postgres;

--
-- TOC entry 5539 (class 0 OID 0)
-- Dependencies: 243
-- Name: units_unit_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.units_unit_id_seq OWNED BY public.units.unit_id;


--
-- TOC entry 289 (class 1259 OID 19131)
-- Name: user_addresses; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_addresses (
    user_id uuid NOT NULL,
    label character varying(100),
    address_line1 character varying(255),
    address_line2 character varying(255),
    city character varying(100),
    state character varying(100),
    pincode character varying(20),
    latitude numeric(10,8),
    longitude numeric(11,8),
    is_default boolean DEFAULT false,
    metadata jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    id uuid DEFAULT gen_random_uuid() NOT NULL
);


ALTER TABLE public.user_addresses OWNER TO postgres;

--
-- TOC entry 5540 (class 0 OID 0)
-- Dependencies: 289
-- Name: TABLE user_addresses; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.user_addresses IS 'Stores user delivery addresses with geolocation support';


--
-- TOC entry 286 (class 1259 OID 18981)
-- Name: user_payment_methods; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.user_payment_methods (
    method_id character varying(64) NOT NULL,
    user_id text NOT NULL,
    method_type character varying(20) NOT NULL,
    card_brand character varying(32),
    card_last4 character varying(4),
    expiry_month smallint,
    expiry_year smallint,
    upi_id character varying(255),
    is_default boolean DEFAULT false NOT NULL,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_payment_methods_check CHECK (((((method_type)::text = 'CARD'::text) AND (card_last4 IS NOT NULL)) OR (((method_type)::text = 'UPI'::text) AND (upi_id IS NOT NULL)) OR ((method_type)::text = ANY ((ARRAY['WALLET'::character varying, 'NETBANKING'::character varying])::text[])))),
    CONSTRAINT user_payment_methods_method_type_check CHECK (((method_type)::text = ANY ((ARRAY['CARD'::character varying, 'UPI'::character varying, 'WALLET'::character varying, 'NETBANKING'::character varying])::text[])))
);


ALTER TABLE public.user_payment_methods OWNER TO postgres;

--
-- TOC entry 225 (class 1259 OID 18228)
-- Name: users; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.users (
    user_id integer NOT NULL,
    first_name character varying(100),
    last_name character varying(100),
    email character varying(255),
    phone character varying(20),
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    phone_country_code character varying(10),
    display_name character varying(255),
    password_hash character varying(255),
    auth_provider character varying(255),
    primary_role character varying(255),
    username character varying(100),
    password_salt character varying(255),
    oauth_id character varying(255),
    roles jsonb DEFAULT '[]'::jsonb,
    avatar_url text,
    dob date,
    gender character varying(20),
    bio text,
    addresses jsonb DEFAULT '[]'::jsonb,
    default_address_id uuid,
    loyalty_points integer DEFAULT 0,
    favorite_restaurants jsonb DEFAULT '[]'::jsonb,
    default_payment_method_id character varying(100),
    order_history_summary jsonb,
    business_name character varying(255),
    business_legal_name character varying(255),
    business_phone character varying(20),
    business_registration_number character varying(100),
    gstin character varying(20),
    business_address text,
    restaurant_ids jsonb DEFAULT '[]'::jsonb,
    license_number character varying(100),
    license_expiry date,
    vehicle_type character varying(50),
    vehicle_registration_number character varying(50),
    vehicle_details jsonb,
    insurance_details jsonb,
    max_carry_capacity_kg numeric(10,2),
    is_available boolean DEFAULT false,
    on_trip boolean DEFAULT false,
    current_lat numeric(10,8),
    current_lng numeric(11,8),
    last_location_update timestamp without time zone,
    email_verified boolean DEFAULT false,
    phone_verified boolean DEFAULT false,
    kyc_verified boolean DEFAULT false,
    kyc_data jsonb,
    verification_docs jsonb,
    bank_details jsonb,
    payout_methods jsonb,
    two_factor_enabled boolean DEFAULT false,
    two_factor_method character varying(20),
    totp_secret character varying(255),
    failed_login_attempts integer DEFAULT 0,
    locked_until timestamp without time zone,
    rating_avg numeric(3,2),
    rating_count integer DEFAULT 0,
    total_deliveries integer DEFAULT 0,
    total_orders integer DEFAULT 0,
    earnings numeric(12,2) DEFAULT 0.00,
    device_info jsonb,
    preferences jsonb,
    settings jsonb,
    metadata jsonb,
    last_login_at timestamp without time zone,
    last_seen_at timestamp without time zone,
    last_known_ip character varying(50),
    timezone character varying(50),
    locale character varying(20),
    status character varying(20) DEFAULT 'active'::character varying,
    created_by uuid,
    updated_by uuid,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    deleted_at timestamp without time zone,
    referral_code character varying(50),
    referred_by uuid,
    search_vector tsvector,
    id uuid DEFAULT gen_random_uuid() NOT NULL
);


ALTER TABLE public.users OWNER TO postgres;

--
-- TOC entry 5541 (class 0 OID 0)
-- Dependencies: 225
-- Name: TABLE users; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON TABLE public.users IS 'Main users table supporting customers, restaurant owners, and delivery drivers';


--
-- TOC entry 5542 (class 0 OID 0)
-- Dependencies: 225
-- Name: COLUMN users.primary_role; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.users.primary_role IS 'Values: customer, restaurant_owner, delivery_driver, admin, restaurant_staff';


--
-- TOC entry 5543 (class 0 OID 0)
-- Dependencies: 225
-- Name: COLUMN users.status; Type: COMMENT; Schema: public; Owner: postgres
--

COMMENT ON COLUMN public.users.status IS 'Values: active, inactive, suspended, deleted';


--
-- TOC entry 224 (class 1259 OID 18227)
-- Name: users_user_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.users_user_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.users_user_id_seq OWNER TO postgres;

--
-- TOC entry 5544 (class 0 OID 0)
-- Dependencies: 224
-- Name: users_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.users_user_id_seq OWNED BY public.users.user_id;


--
-- TOC entry 248 (class 1259 OID 18463)
-- Name: vendors; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.vendors (
    vendor_id integer NOT NULL,
    tenant_id integer DEFAULT 1,
    name character varying(255) NOT NULL,
    contact_person character varying(255),
    email character varying(255),
    phone character varying(50),
    address text,
    city character varying(100),
    state character varying(100),
    postal_code character varying(20),
    gst_number character varying(50),
    payment_terms character varying(100),
    is_active boolean DEFAULT true,
    notes text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


ALTER TABLE public.vendors OWNER TO postgres;

--
-- TOC entry 247 (class 1259 OID 18462)
-- Name: vendors_vendor_id_seq; Type: SEQUENCE; Schema: public; Owner: postgres
--

CREATE SEQUENCE public.vendors_vendor_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER SEQUENCE public.vendors_vendor_id_seq OWNER TO postgres;

--
-- TOC entry 5545 (class 0 OID 0)
-- Dependencies: 247
-- Name: vendors_vendor_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: postgres
--

ALTER SEQUENCE public.vendors_vendor_id_seq OWNED BY public.vendors.vendor_id;


--
-- TOC entry 276 (class 1259 OID 18870)
-- Name: wallet_ledger; Type: TABLE; Schema: public; Owner: postgres
--

CREATE TABLE public.wallet_ledger (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    tenant_id text NOT NULL,
    outlet_id text NOT NULL,
    customer_id text NOT NULL,
    direction text NOT NULL,
    amount numeric(18,6) NOT NULL,
    ref_type text NOT NULL,
    ref_id text NOT NULL,
    meta jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.wallet_ledger OWNER TO postgres;

--
-- TOC entry 4608 (class 2604 OID 18376)
-- Name: bank_verifications id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bank_verifications ALTER COLUMN id SET DEFAULT nextval('public.bank_verifications_id_seq'::regclass);


--
-- TOC entry 4773 (class 2604 OID 19276)
-- Name: incentive_cycles id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.incentive_cycles ALTER COLUMN id SET DEFAULT nextval('public.incentive_cycles_id_seq'::regclass);


--
-- TOC entry 4778 (class 2604 OID 19292)
-- Name: incentive_milestones id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.incentive_milestones ALTER COLUMN id SET DEFAULT nextval('public.incentive_milestones_id_seq'::regclass);


--
-- TOC entry 4627 (class 2604 OID 18481)
-- Name: ingredients ingredient_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ingredients ALTER COLUMN ingredient_id SET DEFAULT nextval('public.ingredients_ingredient_id_seq'::regclass);


--
-- TOC entry 4642 (class 2604 OID 18558)
-- Name: inventory_recipe_lines recipe_line_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipe_lines ALTER COLUMN recipe_line_id SET DEFAULT nextval('public.inventory_recipe_lines_recipe_line_id_seq'::regclass);


--
-- TOC entry 4637 (class 2604 OID 18538)
-- Name: inventory_recipes recipe_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipes ALTER COLUMN recipe_id SET DEFAULT nextval('public.inventory_recipes_recipe_id_seq'::regclass);


--
-- TOC entry 4646 (class 2604 OID 18580)
-- Name: inventory_stock_ledger ledger_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_stock_ledger ALTER COLUMN ledger_id SET DEFAULT nextval('public.inventory_stock_ledger_ledger_id_seq'::regclass);


--
-- TOC entry 4562 (class 2604 OID 18283)
-- Name: menu_categories category_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_categories ALTER COLUMN category_id SET DEFAULT nextval('public.menu_categories_category_id_seq'::regclass);


--
-- TOC entry 4759 (class 2604 OID 19206)
-- Name: menu_item_addons addon_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_addons ALTER COLUMN addon_id SET DEFAULT nextval('public.menu_item_addons_addon_id_seq'::regclass);


--
-- TOC entry 4782 (class 2604 OID 19307)
-- Name: menu_item_combos combo_item_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_combos ALTER COLUMN combo_item_id SET DEFAULT nextval('public.menu_item_combos_combo_item_id_seq'::regclass);


--
-- TOC entry 4751 (class 2604 OID 19180)
-- Name: menu_item_variants variant_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_variants ALTER COLUMN variant_id SET DEFAULT nextval('public.menu_item_variants_variant_id_seq'::regclass);


--
-- TOC entry 4567 (class 2604 OID 18300)
-- Name: menu_items item_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_items ALTER COLUMN item_id SET DEFAULT nextval('public.menu_items_item_id_seq'::regclass);


--
-- TOC entry 4770 (class 2604 OID 19256)
-- Name: order_item_addons id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_addons ALTER COLUMN id SET DEFAULT nextval('public.order_item_addons_id_seq'::regclass);


--
-- TOC entry 4768 (class 2604 OID 19237)
-- Name: order_item_variants id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_variants ALTER COLUMN id SET DEFAULT nextval('public.order_item_variants_id_seq'::regclass);


--
-- TOC entry 4604 (class 2604 OID 18351)
-- Name: order_items order_item_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_items ALTER COLUMN order_item_id SET DEFAULT nextval('public.order_items_order_item_id_seq'::regclass);


--
-- TOC entry 4583 (class 2604 OID 18327)
-- Name: orders order_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.orders ALTER COLUMN order_id SET DEFAULT nextval('public.orders_order_id_seq'::regclass);


--
-- TOC entry 4736 (class 2604 OID 18963)
-- Name: payment_sessions id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_sessions ALTER COLUMN id SET DEFAULT nextval('public.payment_sessions_id_seq'::regclass);


--
-- TOC entry 4519 (class 2604 OID 17906)
-- Name: permissions id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions ALTER COLUMN id SET DEFAULT nextval('public.permissions_id_seq'::regclass);


--
-- TOC entry 4731 (class 2604 OID 18944)
-- Name: qr_sessions session_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.qr_sessions ALTER COLUMN session_id SET DEFAULT nextval('public.qr_sessions_session_id_seq'::regclass);


--
-- TOC entry 4556 (class 2604 OID 18252)
-- Name: restaurant_hours id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_hours ALTER COLUMN id SET DEFAULT nextval('public.restaurant_hours_id_seq'::regclass);


--
-- TOC entry 4559 (class 2604 OID 18267)
-- Name: restaurant_tables id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_tables ALTER COLUMN id SET DEFAULT nextval('public.restaurant_tables_id_seq'::regclass);


--
-- TOC entry 4543 (class 2604 OID 18241)
-- Name: restaurants restaurant_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurants ALTER COLUMN restaurant_id SET DEFAULT nextval('public.restaurants_restaurant_id_seq'::regclass);


--
-- TOC entry 4516 (class 2604 OID 17893)
-- Name: roles id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles ALTER COLUMN id SET DEFAULT nextval('public.roles_id_seq'::regclass);


--
-- TOC entry 4619 (class 2604 OID 18441)
-- Name: unit_conversions conversion_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit_conversions ALTER COLUMN conversion_id SET DEFAULT nextval('public.unit_conversions_conversion_id_seq'::regclass);


--
-- TOC entry 4615 (class 2604 OID 18426)
-- Name: units unit_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.units ALTER COLUMN unit_id SET DEFAULT nextval('public.units_unit_id_seq'::regclass);


--
-- TOC entry 4522 (class 2604 OID 18231)
-- Name: users user_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users ALTER COLUMN user_id SET DEFAULT nextval('public.users_user_id_seq'::regclass);


--
-- TOC entry 4622 (class 2604 OID 18466)
-- Name: vendors vendor_id; Type: DEFAULT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.vendors ALTER COLUMN vendor_id SET DEFAULT nextval('public.vendors_vendor_id_seq'::regclass);


--
-- TOC entry 5453 (class 0 OID 18739)
-- Dependencies: 265
-- Data for Name: api_idempotency_keys; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.api_idempotency_keys (tenant_id, endpoint, idempotency_key, request_hash, response_body, status_code, created_at) FROM stdin;
\.


--
-- TOC entry 5429 (class 0 OID 18373)
-- Dependencies: 241
-- Data for Name: bank_verifications; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.bank_verifications (id, restaurant_id, contact_id, fund_account_id, payout_id, expected_amount_paise, confirmed, created_at, confirmed_at) FROM stdin;
\.


--
-- TOC entry 5500 (class 0 OID 19661)
-- Dependencies: 312
-- Data for Name: customer_devices; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.customer_devices (id, customer_id, device_id, restaurant_id, user_agent, created_at, updated_at) FROM stdin;
cedcbf30-9461-4fe5-9fa9-583983b506fd	ad14032c-9561-496b-869f-27e3d92076eb	b48d5c3f-9a71-47a0-a0f6-a3ae5d136efa	1		2026-03-19 09:31:37.276441+00	2026-03-19 09:31:37.276441+00
13c35738-e0ec-4bc8-a250-28d11d67a476	ad14032c-9561-496b-869f-27e3d92076eb	4e7f9b8a-94f7-49e3-8e25-092cdce95730	1		2026-03-19 13:08:37.448794+00	2026-03-19 13:08:37.460908+00
6af7624a-add3-4d0e-98a4-d649f814159e	ad14032c-9561-496b-869f-27e3d92076eb	a4c6099f-dc01-4405-8e07-a8dbf6cbd1da	1		2026-03-19 17:36:44.542932+00	2026-03-19 17:36:44.556034+00
099cddcf-c02e-499c-8e60-9dc967b5cecb	ad14032c-9561-496b-869f-27e3d92076eb	72ff468e-565f-4992-82b5-547f17d39d09	1		2026-03-20 10:11:52.295141+00	2026-03-20 10:11:52.308477+00
\.


--
-- TOC entry 5502 (class 0 OID 19709)
-- Dependencies: 314
-- Data for Name: customer_favorite_items; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.customer_favorite_items (id, customer_id, menu_item_id, item_name, order_count, last_ordered_at) FROM stdin;
\.


--
-- TOC entry 5455 (class 0 OID 18761)
-- Dependencies: 267
-- Data for Name: customer_feedback; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.customer_feedback (id, tenant_id, outlet_id, order_id, rating, comment, source, customer_name, customer_phone, customer_email, tags, feedback_status, internal_note, created_at, ip_hash, user_agent) FROM stdin;
\.


--
-- TOC entry 5501 (class 0 OID 19687)
-- Dependencies: 313
-- Data for Name: customer_visits; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.customer_visits (id, customer_id, restaurant_id, visit_type, table_no, order_id, started_at, ended_at, created_at) FROM stdin;
e33c2403-1360-4510-8832-6576d75665b4	ad14032c-9561-496b-869f-27e3d92076eb	1	qrunch	\N	\N	2026-03-19 09:31:35.927999+00	\N	2026-03-19 09:31:37.276441+00
544e61d1-0e3d-4eb9-b7f6-5921317eef64	ad14032c-9561-496b-869f-27e3d92076eb	1	qrunch	\N	\N	2026-03-19 13:08:37.44898+00	\N	2026-03-19 13:08:37.448794+00
ce817103-78b9-45e0-8f2a-b615a41b9407	ad14032c-9561-496b-869f-27e3d92076eb	1	qrunch	\N	\N	2026-03-19 17:36:44.543829+00	\N	2026-03-19 17:36:44.542932+00
1f024b50-c448-4305-b851-03a75af4e2b4	ad14032c-9561-496b-869f-27e3d92076eb	1	qrunch	\N	\N	2026-03-20 10:11:52.295537+00	\N	2026-03-20 10:11:52.295141+00
\.


--
-- TOC entry 5463 (class 0 OID 18861)
-- Dependencies: 275
-- Data for Name: customer_wallet; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.customer_wallet (tenant_id, outlet_id, customer_id, balance, updated_at) FROM stdin;
\.


--
-- TOC entry 5499 (class 0 OID 19630)
-- Dependencies: 311
-- Data for Name: customers; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.customers (id, restaurant_id, phone_number, name, total_visits, total_orders, avg_order_value, total_spent, last_visit_at, loyalty_status, reward_eligibility, customer_segment, tags, metadata, created_at, updated_at) FROM stdin;
ad14032c-9561-496b-869f-27e3d92076eb	1	7037772781		1	0	0.00	0.00	2026-03-19 09:31:35.927999+00	NEW	f	first_time	{}	{}	2026-03-19 09:31:37.276441+00	2026-03-19 09:32:26.311154+00
\.


--
-- TOC entry 5447 (class 0 OID 18653)
-- Dependencies: 259
-- Data for Name: daily_outlet_metrics; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.daily_outlet_metrics (tenant_id, outlet_id, day, revenue, orders_count, avg_ticket, discount_total, refund_total, void_count, cash_sales, online_sales, cogs_estimate, gross_profit_estimate, wastage_estimate, variance_estimate, health_score, updated_at) FROM stdin;
\.


--
-- TOC entry 5456 (class 0 OID 18776)
-- Dependencies: 268
-- Data for Name: feedback_events; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.feedback_events (id, tenant_id, outlet_id, order_id, token, event_type, created_at) FROM stdin;
\.


--
-- TOC entry 5476 (class 0 OID 19015)
-- Dependencies: 288
-- Data for Name: foodshare_participants; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.foodshare_participants (session_id, user_id, joined_at) FROM stdin;
\.


--
-- TOC entry 5475 (class 0 OID 18995)
-- Dependencies: 287
-- Data for Name: foodshare_sessions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.foodshare_sessions (session_id, restaurant_id, host_user_id, group_name, max_participants, split_type, status, invite_link, expires_at, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5487 (class 0 OID 19273)
-- Dependencies: 299
-- Data for Name: incentive_cycles; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.incentive_cycles (id, restaurant_id, cycle_number, start_date, end_date, current_sales, status, created_at, updated_at) FROM stdin;
1	1	1	2026-03-18 09:16:20.150314	2026-04-18 09:16:20.150314	199.51	active	2026-03-18 09:16:20.145756	2026-03-18 10:59:56.49698
\.


--
-- TOC entry 5489 (class 0 OID 19289)
-- Dependencies: 301
-- Data for Name: incentive_milestones; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.incentive_milestones (id, cycle_id, type, target_amount, reward_amount, status, created_at, updated_at) FROM stdin;
1	1	30k	30000.00	399.00	pending	2026-03-18 09:16:20.147707	2026-03-18 09:16:20.147707
2	1	60k	60000.00	599.00	pending	2026-03-18 09:16:20.147707	2026-03-18 09:16:20.147707
3	1	100k	100000.00	999.00	pending	2026-03-18 09:16:20.147707	2026-03-18 09:16:20.147707
\.


--
-- TOC entry 5438 (class 0 OID 18478)
-- Dependencies: 250
-- Data for Name: ingredients; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.ingredients (ingredient_id, tenant_id, restaurant_id, name, sku, base_unit_id, category, subcategory, description, is_perishable, track_batch, track_serial, average_cost, last_purchase_cost, preferred_vendor_id, is_active, created_at, updated_at, created_by, reorder_level) FROM stdin;
\.


--
-- TOC entry 5445 (class 0 OID 18592)
-- Dependencies: 257
-- Data for Name: inventory_idempotency_keys; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.inventory_idempotency_keys (tenant_id, endpoint, idempotency_key, request_hash, response_body, status_code, created_at) FROM stdin;
\.


--
-- TOC entry 5442 (class 0 OID 18555)
-- Dependencies: 254
-- Data for Name: inventory_recipe_lines; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.inventory_recipe_lines (recipe_line_id, recipe_id, ingredient_id, qty, unit, wastage_pct, rounding, created_at) FROM stdin;
\.


--
-- TOC entry 5440 (class 0 OID 18535)
-- Dependencies: 252
-- Data for Name: inventory_recipes; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.inventory_recipes (recipe_id, tenant_id, outlet_id, menu_item_id, variant, version, version_note, is_active, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5444 (class 0 OID 18577)
-- Dependencies: 256
-- Data for Name: inventory_stock_ledger; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.inventory_stock_ledger (ledger_id, tenant_id, outlet_id, ingredient_id, entry_type, qty_base_unit, ref_type, ref_id, base_unit_symbol, meta, created_at) FROM stdin;
\.


--
-- TOC entry 5452 (class 0 OID 18724)
-- Dependencies: 264
-- Data for Name: inventory_stock_snapshots; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.inventory_stock_snapshots (tenant_id, outlet_id, ingredient_id, on_hand, updated_at) FROM stdin;
\.


--
-- TOC entry 5503 (class 0 OID 19730)
-- Dependencies: 315
-- Data for Name: item_pairings; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.item_pairings (id, restaurant_id, source_item_id, target_item_id, pairing_type, reason, price_impact, co_order_count, priority, is_active, created_at) FROM stdin;
\.


--
-- TOC entry 5465 (class 0 OID 18880)
-- Dependencies: 277
-- Data for Name: loyalty_points; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.loyalty_points (tenant_id, outlet_id, customer_id, points, updated_at) FROM stdin;
\.


--
-- TOC entry 5421 (class 0 OID 18280)
-- Dependencies: 233
-- Data for Name: menu_categories; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.menu_categories (category_id, restaurant_id, name, description, display_order, is_active, created_at, category_type) FROM stdin;
1	1	Desert	\N	0	t	2026-03-18 09:17:30.988739	regular
2	1	Fast food	\N	0	t	2026-03-18 09:18:44.112232	regular
3	1	Pizza's	\N	0	t	2026-03-18 09:26:12.3688	regular
4	1	Burger's	\N	0	t	2026-03-18 09:26:48.946701	regular
5	1	Drink	\N	0	t	2026-03-18 09:27:25.063633	regular
6	1	Combo	\N	0	t	2026-03-18 09:27:34.464307	combo
7	1	Offers	\N	0	t	2026-03-18 09:27:43.503621	offer
8	1	Fries	\N	0	t	2026-03-18 09:29:34.272209	regular
\.


--
-- TOC entry 5481 (class 0 OID 19203)
-- Dependencies: 293
-- Data for Name: menu_item_addons; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.menu_item_addons (addon_id, item_id, category_id, restaurant_id, addon_name, addon_type, price, description, display_order, is_available, is_multiple_selection, max_quantity, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5491 (class 0 OID 19304)
-- Dependencies: 303
-- Data for Name: menu_item_combos; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.menu_item_combos (combo_item_id, combo_menu_item_id, item_id, variant_id, quantity, display_order, created_at, updated_at) FROM stdin;
1	8	4	\N	1	0	2026-03-18 09:31:37.150294	2026-03-18 09:31:37.150294
2	8	6	\N	1	1	2026-03-18 09:31:37.150294	2026-03-18 09:31:37.150294
3	8	7	\N	1	2	2026-03-18 09:31:37.150294	2026-03-18 09:31:37.150294
4	9	3	5	1	0	2026-03-18 09:32:16.220593	2026-03-18 09:32:16.220593
5	9	3	4	1	1	2026-03-18 09:32:16.220593	2026-03-18 09:32:16.220593
\.


--
-- TOC entry 5448 (class 0 OID 18674)
-- Dependencies: 260
-- Data for Name: menu_item_daily_metrics; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.menu_item_daily_metrics (tenant_id, outlet_id, day, menu_item_id, qty_sold, revenue, cogs_estimate, gross_profit_estimate, margin_pct) FROM stdin;
\.


--
-- TOC entry 5479 (class 0 OID 19177)
-- Dependencies: 291
-- Data for Name: menu_item_variants; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.menu_item_variants (variant_id, menu_item_id, variant_name, variant_type, price, measurement_unit, measurement_value, description, display_order, is_available, is_default, sku, created_at, updated_at) FROM stdin;
1	1	500g	weight	500.00	g	500.00		1	t	f	\N	2026-03-18 09:17:34.292364	2026-03-18 09:17:34.292364
2	1	1kg	weight	1000.00	kg	1.00		1	t	f	\N	2026-03-18 09:17:34.292364	2026-03-18 09:17:34.292364
3	3	Regular 	size	250.00	S	\N		1	t	f	\N	2026-03-18 09:26:19.504405	2026-03-18 09:26:19.504405
4	3	Medium 	size	350.00	M	\N		1	t	f	\N	2026-03-18 09:26:19.504405	2026-03-18 09:26:19.504405
5	3	Large	size	450.00	L	\N		1	t	f	\N	2026-03-18 09:26:19.504405	2026-03-18 09:26:19.504405
\.


--
-- TOC entry 5423 (class 0 OID 18297)
-- Dependencies: 235
-- Data for Name: menu_items; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.menu_items (item_id, restaurant_id, category_id, name, description, price, image_url, is_vegetarian, is_vegan, is_gluten_free, calories, preparation_time, is_available, display_order, created_at, updated_at, is_qrunch, promo_eligible, promo_cost, is_taxable, measurement_unit, has_variants, has_addons) FROM stdin;
1	1	1	kaju katli	\N	0.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/ef1b4b47-55fe-4597-975d-8ed9c5cbc1ad.png	t	f	f	\N	\N	t	1	2026-03-18 09:17:34.288829	2026-03-18 09:17:34.288829	t	f	0.000000	f	piece	t	f
3	1	3	Pizza	\N	0.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/60cc6317-c071-4610-98e4-5a3c4ae38849.jpg	t	f	f	\N	\N	t	1	2026-03-18 09:26:19.501981	2026-03-18 09:26:19.501981	t	f	0.000000	t	piece	t	f
4	1	4	Burger	\N	50.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/db30fe76-fd0b-4bcc-bd37-b1695fab010b.png	t	f	f	\N	\N	t	1	2026-03-18 09:27:00.129682	2026-03-18 09:27:00.129682	t	f	0.000000	t	piece	f	f
6	1	8	French Fries 	\N	60.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/d158e003-1b56-4818-afcc-c9829efa3317.jpg	t	f	f	\N	\N	t	1	2026-03-18 09:29:43.702114	2026-03-18 09:29:43.702114	t	f	0.000000	t	piece	f	f
7	1	5	Coke	\N	25.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/e692ef97-c61a-4f79-a44a-22a39bf41b3c.jpg	t	f	f	\N	\N	t	1	2026-03-18 09:31:00.758306	2026-03-18 09:31:00.758306	t	f	0.000000	t	piece	f	f
8	1	6	Burger Combo	Includes: Burger x1, French Fries  x1, Coke x1	120.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/9a5c3203-a8ae-4403-945f-ac7d66b5f63c.jpg	t	f	f	\N	15	t	1	2026-03-18 09:31:37.138323	2026-03-18 09:31:37.138323	t	f	0.000000	t	piece	f	f
9	1	7	Sunday Special Offer	Includes: Pizza (Large) x1, Pizza (Medium ) x1	450.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/97eedde8-54dd-4155-99df-5898a14e3bc7.jpg	t	f	f	\N	15	t	1	2026-03-18 09:32:16.206542	2026-03-18 09:32:16.206542	t	f	0.000000	t	piece	f	f
2	1	2	samosa		10.00	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/0d88b48e-8ab6-4ae7-a63d-da76d4d39635.webp	t	f	f	\N	15	t	1	2026-03-18 09:18:46.993215	2026-03-18 09:32:44.890013	t	f	0.000000	f	piece	f	f
\.


--
-- TOC entry 5450 (class 0 OID 18697)
-- Dependencies: 262
-- Data for Name: notification_devices; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.notification_devices (id, tenant_id, outlet_id, user_id, platform, push_token, created_at) FROM stdin;
\.


--
-- TOC entry 5462 (class 0 OID 18848)
-- Dependencies: 274
-- Data for Name: offer_applications; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.offer_applications (id, tenant_id, outlet_id, order_id, offer_id, applied_at, computed_json, idempotency_key) FROM stdin;
\.


--
-- TOC entry 5461 (class 0 OID 18835)
-- Dependencies: 273
-- Data for Name: offer_benefits; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.offer_benefits (id, offer_id, benefit_json) FROM stdin;
\.


--
-- TOC entry 5460 (class 0 OID 18822)
-- Dependencies: 272
-- Data for Name: offer_rules; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.offer_rules (id, offer_id, rule_json) FROM stdin;
\.


--
-- TOC entry 5505 (class 0 OID 19801)
-- Dependencies: 317
-- Data for Name: offer_suggested_items; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.offer_suggested_items (id, offer_id, menu_item_id, item_type, reason, quantity, display_order, created_at) FROM stdin;
\.


--
-- TOC entry 5459 (class 0 OID 18809)
-- Dependencies: 271
-- Data for Name: offers; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.offers (id, tenant_id, outlet_id, name, offer_type, status, start_at, end_at, priority, stackable, created_by, created_at, updated_at, trigger_context, display_style, recommendation_reason, visit_min_count, customer_segment_in, anchor_price, final_price, saving_text, badge_text, subtitle, personalization_eligible, visit_based, eligible_item_ids, visibility) FROM stdin;
\.


--
-- TOC entry 5485 (class 0 OID 19253)
-- Dependencies: 297
-- Data for Name: order_item_addons; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.order_item_addons (id, order_item_id, addon_id, addon_name, addon_price, quantity, created_at) FROM stdin;
\.


--
-- TOC entry 5483 (class 0 OID 19234)
-- Dependencies: 295
-- Data for Name: order_item_variants; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.order_item_variants (id, order_item_id, variant_id, variant_name, variant_price, created_at) FROM stdin;
1	4	2	1kg	1000.00	2026-03-18 10:47:16.327842
2	18	3	Regular	250.00	2026-03-19 09:33:06.406797
3	22	1	500g	500.00	2026-03-19 11:12:00.954459
4	41	3	Regular	262.50	2026-03-20 09:48:01.027526
\.


--
-- TOC entry 5427 (class 0 OID 18348)
-- Dependencies: 239
-- Data for Name: order_items; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.order_items (order_item_id, order_id, menu_item_id, quantity, is_taxable, unit_price, total_price, special_instructions, created_at, name, options) FROM stdin;
1	1	2	1	f	10.00	10.00	\N	2026-03-18 09:32:58.482915	\N	\N
2	1	4	1	f	52.50	52.50	\N	2026-03-18 09:32:58.482915	\N	\N
3	2	2	1	f	11.00	11.00	\N	2026-03-18 10:12:26.325969	\N	\N
4	3	1	1	f	1000.00	1000.00	\N	2026-03-18 10:47:16.327842	\N	\N
5	3	6	1	f	63.00	63.00	\N	2026-03-18 10:47:16.327842	\N	\N
6	3	2	5	f	10.00	50.00	\N	2026-03-18 10:47:16.327842	\N	\N
7	4	4	2	f	52.50	105.00	\N	2026-03-18 10:48:30.109743	\N	\N
8	4	7	2	f	26.25	52.50	\N	2026-03-18 10:48:30.109743	\N	\N
9	5	9	1	f	473.00	473.00	\N	2026-03-19 05:52:07.239566	\N	\N
10	6	6	1	f	63.00	63.00	\N	2026-03-19 09:08:32.928974	\N	\N
11	7	6	1	f	63.00	63.00	\N	2026-03-19 09:08:48.677533	\N	\N
12	8	7	1	f	26.00	26.00	\N	2026-03-19 09:13:11.741541	\N	\N
13	9	2	2	f	11.00	22.00	\N	2026-03-19 09:19:49.351916	\N	\N
14	10	7	1	f	26.00	26.00	\N	2026-03-19 09:20:19.306725	\N	\N
15	10	6	1	f	63.00	63.00	\N	2026-03-19 09:20:19.306725	\N	\N
16	11	4	1	f	53.00	53.00	\N	2026-03-19 09:28:27.027063	\N	\N
17	12	4	2	f	53.00	106.00	\N	2026-03-19 09:32:03.962207	\N	\N
18	13	3	8	f	263.00	2104.00	\N	2026-03-19 09:33:06.406797	\N	\N
19	14	4	4	f	53.00	212.00	\N	2026-03-19 09:40:22.354976	\N	\N
20	15	4	1	f	53.00	53.00	\N	2026-03-19 09:44:55.79826	\N	\N
21	16	2	2	f	11.00	22.00	\N	2026-03-19 09:50:00.135041	\N	\N
22	17	1	7	f	500.00	3500.00	\N	2026-03-19 11:12:00.954459	\N	\N
23	18	7	1	f	26.00	26.00	\N	2026-03-19 13:08:22.504518	\N	\N
33	28	2	1	f	10.00	10.00	\N	2026-03-19 17:36:29.937728	\N	\N
35	30	6	1	f	63.00	63.00	\N	2026-03-19 17:39:39.633779	\N	\N
36	30	2	1	f	11.00	11.00	\N	2026-03-19 17:39:39.633779	\N	\N
39	33	2	1	f	10.00	10.00	\N	2026-03-19 17:49:42.683573	\N	\N
40	34	2	1	f	10.00	10.00	\N	2026-03-20 09:48:01.027526	\N	\N
41	34	3	1	f	262.50	262.50	\N	2026-03-20 09:48:01.027526	\N	\N
42	34	6	1	f	63.00	63.00	\N	2026-03-20 09:48:01.027526	\N	\N
43	35	2	1	f	11.00	11.00	\N	2026-03-20 10:08:03.00136	\N	\N
44	35	7	1	f	26.00	26.00	\N	2026-03-20 10:08:03.00136	\N	\N
47	38	7	1	f	26.00	26.00	\N	2026-03-20 10:11:40.018776	\N	\N
48	39	2	1	f	11.00	11.00	\N	2026-03-20 10:12:29.426453	\N	\N
\.


--
-- TOC entry 5425 (class 0 OID 18324)
-- Dependencies: 237
-- Data for Name: orders; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.orders (order_id, customer_id, restaurant_id, delivery_partner_id, order_status, payment_status, subtotal, tax_amount, delivery_fee, tip_amount, discount_amount, total_amount, delivery_address, delivery_latitude, delivery_longitude, special_instructions, estimated_delivery_time, actual_delivery_time, created_at, updated_at, is_qrunch, order_number, order_type, dining_session_id, metadata, qrunch_customer_name, table_no, pay_by, cgst, sgst, delivery_address_id, kds_status, kds_priority, kds_station_ids, kds_notes, kds_chef, kds_sla_minutes, user_id, customer_uuid, discount_breakdown) FROM stdin;
1	\N	1	\N	completed	paid	52.50	0.00	0.00	0.00	1.05	64.03	\N	\N	\N	\N	\N	\N	2026-03-18 09:32:58.482915	2026-03-18 09:33:23.864078	t	ORD-000001	PICKUP	\N	\N	\N	\N	upi	1.29	1.29	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
35	\N	1	\N	cancelled	pending	37.00	0.00	0.00	0.00	0.74	38.08	\N	\N	\N		\N	\N	2026-03-20 10:08:03.00136	2026-03-20 10:11:47.928373	t	ORD-000035	DINE_IN	\N	\N	H	2	cash	0.91	0.91	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
16	\N	1	\N	pending	pending	22.00	0.00	0.00	0.00	0.44	22.64	\N	\N	\N		\N	\N	2026-03-19 09:50:00.135041	2026-03-19 09:50:00.135041	t	ORD-000016	DINE_IN	\N	\N	Gursevak s	34	cash	0.54	0.54	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
4	\N	1	\N	completed	paid	157.50	0.00	0.00	0.00	3.15	162.07	\N	\N	\N	\N	\N	\N	2026-03-18 10:48:30.109743	2026-03-18 10:59:17.607889	t	ORD-000004	DINE_IN	\N	\N	robb	\N	upi	3.86	3.86	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
3	\N	1	\N	cancelled	pending	63.00	0.00	0.00	0.00	1.26	1114.82	\N	\N	\N	\N	\N	\N	2026-03-18 10:47:16.327842	2026-03-18 19:32:58.502028	t	ORD-000003	DINE_IN	\N	\N	robin	\N	cash	1.54	1.54	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
2	\N	1	\N	cancelled	pending	11.00	0.00	0.00	0.00	0.22	11.32	\N	\N	\N		\N	\N	2026-03-18 10:12:26.325969	2026-03-18 20:06:03.26113	t	ORD-000002	DINE_IN	\N	\N	H	1	cash	0.27	0.27	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
5	\N	1	\N	pending	pending	473.00	0.00	0.00	0.00	9.46	486.72	\N	\N	\N		\N	\N	2026-03-19 05:52:07.239566	2026-03-19 05:52:07.239566	t	ORD-000005	DINE_IN	\N	\N	Gursevak	87	cash	11.59	11.59	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
39	\N	1	\N	confirmed	pending	11.00	0.54	0.00	0.00	0.22	22.32	\N	\N	\N		\N	\N	2026-03-20 10:12:29.426453	2026-03-20 17:02:50.831616	t	ORD-000039	DINE_IN	\N	\N	H	2	cash	0.27	0.27	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
6	\N	1	\N	pending	pending	63.00	0.00	0.00	0.00	1.26	64.82	\N	\N	\N		\N	\N	2026-03-19 09:08:32.928974	2026-03-19 09:08:32.928974	t	ORD-000006	DINE_IN	\N	\N	Gursevak s	34	cash	1.54	1.54	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
38	\N	1	\N	cancelled	pending	26.00	0.00	0.00	0.00	0.52	26.76	\N	\N	\N		\N	\N	2026-03-20 10:11:40.018776	2026-03-20 17:03:06.502297	t	ORD-000038	DINE_IN	\N	\N	Gursevak	12	cash	0.64	0.64	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
7	\N	1	\N	pending	pending	63.00	0.00	0.00	0.00	1.26	64.82	\N	\N	\N		\N	\N	2026-03-19 09:08:48.677533	2026-03-19 09:08:48.677533	t	ORD-000007	DINE_IN	\N	\N	Gursevak s	34	cash	1.54	1.54	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
8	\N	1	\N	pending	pending	26.00	0.00	0.00	0.00	0.52	26.76	\N	\N	\N		\N	\N	2026-03-19 09:13:11.741541	2026-03-19 09:13:11.741541	t	ORD-000008	DINE_IN	\N	\N	Gursevak s	34	cash	0.64	0.64	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
9	\N	1	\N	pending	pending	22.00	0.00	0.00	0.00	0.44	22.64	\N	\N	\N		\N	\N	2026-03-19 09:19:49.351916	2026-03-19 09:19:49.351916	t	ORD-000009	DINE_IN	\N	\N	Gursevak s	34	cash	0.54	0.54	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
10	\N	1	\N	pending	pending	89.00	0.00	0.00	0.00	1.78	91.58	\N	\N	\N		\N	\N	2026-03-19 09:20:19.306725	2026-03-19 09:20:19.306725	t	ORD-000010	DINE_IN	\N	\N	Gursevak s	34	cash	2.18	2.18	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
11	\N	1	\N	pending	pending	53.00	0.00	0.00	0.00	1.06	54.54	\N	\N	\N		\N	\N	2026-03-19 09:28:27.027063	2026-03-19 09:28:27.027063	t	ORD-000011	DINE_IN	\N	\N	Gursevak s	34	cash	1.30	1.30	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
12	\N	1	\N	pending	pending	106.00	0.00	0.00	0.00	2.12	109.08	\N	\N	\N		\N	\N	2026-03-19 09:32:03.962207	2026-03-19 09:32:03.962207	t	ORD-000012	DINE_IN	\N	\N	Gursevak s	34	cash	2.60	2.60	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
13	\N	1	\N	pending	pending	2104.00	0.00	0.00	0.00	42.08	2165.02	\N	\N	\N		\N	\N	2026-03-19 09:33:06.406797	2026-03-19 09:33:06.406797	t	ORD-000013	DINE_IN	\N	\N	Gursevak s	34	cash	51.55	51.55	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
14	\N	1	\N	pending	pending	212.00	0.00	0.00	0.00	4.24	218.14	\N	\N	\N		\N	\N	2026-03-19 09:40:22.354976	2026-03-19 09:40:22.354976	t	ORD-000014	DINE_IN	\N	\N	Gursevak s	34	cash	5.19	5.19	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
15	\N	1	\N	pending	pending	53.00	0.00	0.00	0.00	1.06	54.54	\N	\N	\N		\N	\N	2026-03-19 09:44:55.79826	2026-03-19 09:44:55.79826	t	ORD-000015	DINE_IN	\N	\N	Gursevak s	34	cash	1.30	1.30	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
17	\N	1	\N	completed	paid	0.00	0.00	0.00	0.00	0.00	3500.00	\N	\N	\N	\N	\N	\N	2026-03-19 11:12:00.954459	2026-03-19 11:12:21.061948	t	ORD-000017	PICKUP	\N	\N	\N	\N	cash	0.00	0.00	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
28	\N	1	\N	cancelled	pending	0.00	0.00	0.00	0.00	0.00	10.00	\N	\N	\N		\N	\N	2026-03-19 17:36:29.937728	2026-03-19 17:39:43.138107	t	ORD-000028	DINE_IN	\N	\N	Gursevak Singh Gill	56	cash	0.00	0.00	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
30	\N	1	\N	cancelled	pending	74.00	0.00	0.00	0.00	1.48	76.14	\N	\N	\N		\N	\N	2026-03-19 17:39:39.633779	2026-03-19 17:47:52.649809	t	ORD-000030	DINE_IN	\N	\N	Gursevak	34	cash	1.81	1.81	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
18	\N	1	\N	cancelled	pending	26.00	0.00	0.00	0.00	0.52	26.76	\N	\N	\N		\N	\N	2026-03-19 13:08:22.504518	2026-03-19 17:48:15.171745	t	ORD-000018	DINE_IN	\N	\N	Gursevak	23	cash	0.64	0.64	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
33	\N	1	\N	cancelled	pending	0.00	0.00	0.00	0.00	0.00	10.00	\N	\N	\N	\N	\N	\N	2026-03-19 17:49:42.683573	2026-03-20 09:46:21.663479	t	ORD-000033	PICKUP	\N	\N	\N	\N	cash	0.00	0.00	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
34	\N	1	\N	pending	pending	325.50	0.00	0.00	0.00	6.51	344.93	\N	\N	\N	\N	\N	\N	2026-03-20 09:48:01.027526	2026-03-20 09:48:01.027526	t	ORD-000034	PICKUP	\N	\N	\N	\N	cash	7.97	7.97	\N	NEW	NORMAL	{}	{}	\N	15	\N	\N	\N
\.


--
-- TOC entry 5495 (class 0 OID 19536)
-- Dependencies: 307
-- Data for Name: organization_users; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.organization_users (id, organization_id, user_id, role, is_active, created_at, updated_at) FROM stdin;
34d757fd-73f1-48ec-9e38-27d0cec47273	a75a6815-6787-46f3-bd93-ab1b56181cec	5953969b-3354-49a4-be82-a9dcf19e91a9	org_owner	t	2026-03-19 05:18:14.891105+00	2026-03-19 05:18:14.891105+00
\.


--
-- TOC entry 5494 (class 0 OID 19501)
-- Dependencies: 306
-- Data for Name: organizations; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.organizations (id, name, slug, legal_name, gst_number, pan_number, logo_url, website, timezone, currency, default_tax_rate, default_service_charge, metadata, status, created_by, created_at, updated_at) FROM stdin;
a75a6815-6787-46f3-bd93-ab1b56181cec	Pizza Hut	grand-bella-italia	\N	\N	\N	\N	\N	Asia/Kolkata	INR	0.00	0.00	{"auto_created": true, "source_restaurant_id": 1}	active	5953969b-3354-49a4-be82-a9dcf19e91a9	2026-03-19 05:18:14.891105+00	2026-03-19 05:18:14.891105+00
\.


--
-- TOC entry 5497 (class 0 OID 19582)
-- Dependencies: 309
-- Data for Name: outlet_menu_overrides; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.outlet_menu_overrides (id, restaurant_id, menu_item_id, price_override, is_available, is_hidden, display_name_override, metadata, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5458 (class 0 OID 18796)
-- Dependencies: 270
-- Data for Name: outlet_offer_constraints; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.outlet_offer_constraints (tenant_id, outlet_id, min_gross_margin_pct, max_next_visit_credit, max_daily_credit_per_customer, updated_at) FROM stdin;
\.


--
-- TOC entry 5457 (class 0 OID 18787)
-- Dependencies: 269
-- Data for Name: outlet_review_settings; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.outlet_review_settings (tenant_id, outlet_id, google_review_url, rating_redirect_threshold, thank_you_message, offer_message, updated_at) FROM stdin;
\.


--
-- TOC entry 5498 (class 0 OID 19607)
-- Dependencies: 310
-- Data for Name: outlet_settings_overrides; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.outlet_settings_overrides (id, restaurant_id, setting_key, setting_value, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5496 (class 0 OID 19556)
-- Dependencies: 308
-- Data for Name: outlet_staff_assignments; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.outlet_staff_assignments (id, organization_id, user_id, restaurant_id, role, is_active, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5449 (class 0 OID 18687)
-- Dependencies: 261
-- Data for Name: owner_insights; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.owner_insights (id, tenant_id, outlet_id, severity, category, title, detail, data, status, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5466 (class 0 OID 18889)
-- Dependencies: 278
-- Data for Name: owner_referral_codes; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.owner_referral_codes (id, tenant_id, owner_user_id, code, created_at) FROM stdin;
\.


--
-- TOC entry 5467 (class 0 OID 18900)
-- Dependencies: 279
-- Data for Name: owner_referrals; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.owner_referrals (id, referrer_tenant_id, referrer_owner_user_id, referred_business_name, referred_phone, referred_email, referral_code, status, referred_tenant_id, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5473 (class 0 OID 18960)
-- Dependencies: 285
-- Data for Name: payment_sessions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.payment_sessions (id, order_id, token_hash, amount, status, expires_at, paid_at, psp_txn_id, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5410 (class 0 OID 17903)
-- Dependencies: 222
-- Data for Name: permissions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.permissions (id, name, description, created_at, updated_at) FROM stdin;
1	manage_roles	Allows managing roles and permissions	2026-03-18 07:15:57.851023	2026-03-18 07:15:57.851023
9	view_dashboard	Can view dashboard	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
11	manage_restaurant	Can manage restaurant settings, menu, and tables	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
12	view_orders	Can view list of orders and order details	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
13	manage_orders	Can update order status and details	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
22	manage_organization	Can manage organization settings and users	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
23	manage_outlets	Can create/update/deactivate outlets	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
24	view_org_dashboard	Can view organization-wide analytics	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
25	manage_outlet_staff	Can assign/remove staff at outlets	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
26	manage_menu_overrides	Can override menu pricing and availability per outlet	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
27	manage_outlet_settings	Can manage outlet-specific settings	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
\.


--
-- TOC entry 5446 (class 0 OID 18642)
-- Dependencies: 258
-- Data for Name: pos_events; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.pos_events (id, tenant_id, outlet_id, event_type, event_time, actor_user_id, actor_role, ref_type, ref_id, amount, meta) FROM stdin;
fbc1509c-fa48-4eac-8981-09c959979a41	1	1	ORDER_PAID	2026-03-19 11:12:21.068321+00	\N	\N	ORDER	17	3500.000000	{"payment_mode": "CASH"}
\.


--
-- TOC entry 5471 (class 0 OID 18941)
-- Dependencies: 283
-- Data for Name: qr_sessions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.qr_sessions (session_id, restaurant_id, table_no, qr_token, status, started_at, expires_at, expired_at, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5492 (class 0 OID 19474)
-- Dependencies: 304
-- Data for Name: qr_tokens; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.qr_tokens (token, restaurant_id, table_number, active, created_at) FROM stdin;
6d2194423de44b548c3225f6e29936d3	1	\N	t	2026-03-18 10:10:24.307969
34c29456d7314407ba61c1bf106835b3	1	\N	t	2026-03-18 10:12:33.755424
1043b6980bb94f329dceb09431dbd50a	1	\N	t	2026-03-18 10:41:09.276713
78a70d7fcb314213b067aa17448e8d8e	1	\N	t	2026-03-18 10:45:13.331759
f804fc5fe0d24c3889ab93dd51c8d507	1	\N	t	2026-03-18 10:45:26.532304
6003047debd84b50998e03b918bb4331	1	\N	t	2026-03-18 10:58:19.755422
6c07f1a8888e435da7b312aedb592e59	1	\N	t	2026-03-18 11:00:36.380318
a566bb3e4f4840a58a317e8cae7eee0a	1	\N	t	2026-03-18 18:39:33.467243
3139651cab8b40dca2943f52af7d0899	1	\N	t	2026-03-18 19:03:18.7088
3083fbcfe2ff461cb7283736cb960218	1	\N	t	2026-03-18 19:22:57.462032
c33c27e51cb5400d87083fb7980c755c	1	\N	t	2026-03-18 19:32:44.67121
73b865ac0c4b4703819060fd8eb1565a	1	\N	t	2026-03-18 19:53:23.023007
a4db692106db443fac902181b3a759f9	1	\N	t	2026-03-18 20:02:50.882837
8c75b79b0ba043bfb91446717a3bd2cc	1	\N	t	2026-03-19 05:50:24.460652
326604ea021f4f19a7f531423998e963	1	\N	t	2026-03-19 06:05:25.542666
a11e26900bb541bf9a1668a3a94bb4f3	1	\N	t	2026-03-19 06:16:29.652409
c96d336a142e45e7b888035747d4e268	1	\N	t	2026-03-19 11:11:48.91139
9075b75ef700400f823f262dfe51fdbf	1	\N	t	2026-03-19 17:48:36.286984
2a5c6dabd1cc416b8f52447a206dc893	1	\N	t	2026-03-20 09:46:50.12796
894ba3a6c03f49fb8a9db19b4e2d33d8	1	\N	t	2026-03-20 10:07:31.04508
d8b7259387fb4dadb9128900c0ed211a	1	\N	t	2026-03-20 10:08:37.341203
c17564e137c54f43bf00ea23af36b9e1	1	\N	t	2026-03-20 16:52:30.74921
322c6e7acee84768bb2e8b3f9d33c8df	1	\N	t	2026-03-20 16:52:41.255747
c92b5fbc10b840ac9830033147d1ad92	1	\N	t	2026-03-20 16:52:47.904929
24bc228e6687436395ec551621fd85ec	1	\N	t	2026-03-20 16:53:11.657561
67988b86f88b4997ac3d5d43c894e79e	1	\N	t	2026-03-20 16:53:30.606032
179ec19f05a8476b917012dfdeab90fa	1	\N	t	2026-03-20 16:55:21.945855
23fb7e25e29b4645b89a5f12dfe489af	1	\N	t	2026-03-20 16:55:29.42754
ebcc43ecbd194623a8a985aca667a020	1	\N	t	2026-03-20 16:57:34.68162
8144d33acd9b4357b84479ef3a3756ed	1	\N	t	2026-03-20 16:59:26.66671
91519baa7ef543d98913b61a235887a0	1	\N	t	2026-03-20 17:00:57.810031
43dd61c1b2134cd2a9c2b54404969670	1	\N	t	2026-03-20 17:05:08.229849
e47b6e7814ff4a87b14957afcf259521	1	\N	t	2026-03-20 17:08:45.742773
3c4425d63c004687a020c4e4e5157f5c	1	\N	t	2026-03-20 17:09:03.030121
72741551c526415cb785c0e64547ee5d	1	\N	t	2026-03-20 17:09:38.764103
\.


--
-- TOC entry 5454 (class 0 OID 18747)
-- Dependencies: 266
-- Data for Name: rating_links; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.rating_links (id, tenant_id, outlet_id, order_id, token, expires_at, max_uses, used_count, created_at) FROM stdin;
\.


--
-- TOC entry 5504 (class 0 OID 19764)
-- Dependencies: 316
-- Data for Name: recommendation_events; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.recommendation_events (id, restaurant_id, customer_id, event_type, source_type, source_id, context, item_ids, metadata, created_at) FROM stdin;
\.


--
-- TOC entry 5469 (class 0 OID 18926)
-- Dependencies: 281
-- Data for Name: referral_attribution_events; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.referral_attribution_events (id, referral_id, event_type, created_at, meta) FROM stdin;
\.


--
-- TOC entry 5468 (class 0 OID 18913)
-- Dependencies: 280
-- Data for Name: referral_rewards; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.referral_rewards (id, referral_id, reward_type, amount, months, issued_at, status, meta) FROM stdin;
\.


--
-- TOC entry 5417 (class 0 OID 18249)
-- Dependencies: 229
-- Data for Name: restaurant_hours; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.restaurant_hours (id, restaurant_id, weekday, open_time, close_time, is_closed, created_at) FROM stdin;
\.


--
-- TOC entry 5493 (class 0 OID 19487)
-- Dependencies: 305
-- Data for Name: restaurant_sessions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.restaurant_sessions (session_id, restaurant_id, table_number, customer_name, created_at, expires_at, updated_at) FROM stdin;
92307f8d-b6ea-4c05-80a9-50e2b5001ed0	1	1	H	2026-03-18 10:10:58.34321	2026-03-18 10:30:58.404415	2026-03-18 10:10:58.403446
918b9efd-d606-4a97-9eaf-400cea262330	1	2	Harsh	2026-03-18 11:00:58.581975	2026-03-18 11:20:58.662838	2026-03-18 11:00:58.660283
5a7764f7-1bed-4ef2-bc88-02e2ddc0eb78	1	87	Gursevak	2026-03-19 05:51:39.864452	2026-03-19 06:11:40.068368	2026-03-19 05:51:40.069821
0293995a-1b81-4529-bf1c-f84396db15d4	1	2	H	2026-03-19 05:54:33.515788	2026-03-19 06:14:33.586141	2026-03-19 05:54:33.587243
0a7bb641-a250-4938-ae72-897a1553483c	1	34	Gursevak s	2026-03-19 08:58:58.624397	2026-03-19 09:18:58.931444	2026-03-19 08:59:00.400586
26378b0a-d73e-4816-8e59-2b99db6ab5ad	1	23	Gursevak	2026-03-19 13:08:06.770201	2026-03-19 13:28:06.838397	2026-03-19 13:08:06.840119
6d256b19-bcbf-4f08-86b8-490b7cf76d37	1	2	H	2026-03-19 16:28:01.332078	2026-03-19 16:48:01.391066	2026-03-19 16:28:01.392426
4aa29832-7440-48bf-89e1-afd65ba4201c	1	21	Shubham sharma	2026-03-19 16:28:48.101991	2026-03-19 16:48:48.231604	2026-03-19 16:28:48.233035
0dce6166-bf7d-4385-88a8-a2ba33be1db5	1	56	Gursevak Singh Gill	2026-03-19 17:35:22.387855	2026-03-19 17:55:22.458075	2026-03-19 17:35:22.459099
7339f079-9c61-4548-bf76-db0b0f219d0f	1	34	Gursevak	2026-03-19 17:39:31.273721	2026-03-19 17:59:31.373061	2026-03-19 17:39:31.374187
04e9fe2d-1070-49e9-b23f-9d576e6c36d3	1	12	Gursevak	2026-03-19 17:40:56.012292	2026-03-19 18:00:56.17888	2026-03-19 17:40:56.180214
69c1e7a7-15e8-4df0-a12c-56703e1e022f	1	2	H	2026-03-20 10:07:45.025728	2026-03-20 10:27:45.122267	2026-03-20 10:07:45.124545
a8a75b22-8f0c-4a99-9765-678712e8221c	1	12	Gursevak	2026-03-20 10:11:06.776606	2026-03-20 10:31:06.870514	2026-03-20 10:11:06.872081
2596e8dd-95d4-4abd-aafe-4864bf064d33	1	2	H	2026-03-20 16:56:53.735254	2026-03-20 17:16:53.816502	2026-03-20 16:56:53.817928
\.


--
-- TOC entry 5430 (class 0 OID 18390)
-- Dependencies: 242
-- Data for Name: restaurant_staff; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.restaurant_staff (user_id, restaurant_id, role_id, is_active, created_at, updated_at, created_by_user_id, id) FROM stdin;
5d993a8c-fa88-4196-b5be-842d9bf21563	1	5	t	2026-03-18 10:42:21.914559	2026-03-18 10:42:21.914559	5953969b-3354-49a4-be82-a9dcf19e91a9	157a0bbd-965c-48ad-9a16-2a236ca99bd5
b6d8143b-4676-4f0f-9a40-c25798abb81d	1	5	t	2026-03-18 10:43:22.269687	2026-03-18 10:43:22.269687	5953969b-3354-49a4-be82-a9dcf19e91a9	6a63d3d7-b9d0-443c-80bf-a91a60cac00d
\.


--
-- TOC entry 5419 (class 0 OID 18264)
-- Dependencies: 231
-- Data for Name: restaurant_tables; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.restaurant_tables (id, restaurant_id, table_identifier, seats, qr_token, qr_url, is_active, created_at) FROM stdin;
\.


--
-- TOC entry 5415 (class 0 OID 18238)
-- Dependencies: 227
-- Data for Name: restaurants; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.restaurants (restaurant_id, owner_auth_user_id, user_id, name, owner_name, slug, type, category, gst_number, fssai_number, logo_url, background_url, gst_certificate_url, fssai_license_url, aadhaar_card_url, pan_card_url, street_address, city, state, postal_code, latitude, longitude, status, tags, metadata, created_at, updated_at, is_qrunch_purchased, is_qrunch_requested, is_restaurant_registered, upi_vpa, wallet_amount, organization_id, outlet_code, outlet_type, supports_dine_in, supports_takeaway, supports_delivery, supports_qr_ordering, manager_user_id, tax_rate, service_charge_rate, outlet_settings) FROM stdin;
1	\N	5953969b-3354-49a4-be82-a9dcf19e91a9	Pizza Hut	Giovanni Rossi	grand-bella-italia	Italian	Fine Dining	27AAAAA0000A1Z5	10012022000000	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/fbadd6a4-5d8d-4767-8044-f97382abb8e1.jpg	https://mangaale-prod.s3.ap-south-1.amazonaws.com/uploads/0981f49c-6f3b-450c-b3d6-12ced9afb9f6.jpg					123 Culinary Avenue	Mumbai	Maharashtra	400001	19.07600000	72.87770000	OPEN	{pasta,pizza,wine}	\N	2026-03-18 09:09:41.310438	2026-03-19 11:12:21.065334	t	t	t	zenzobgmi@ibl	-6.31	a75a6815-6787-46f3-bd93-ab1b56181cec	DEFAULT	company_owned	t	t	t	t	\N	\N	\N	{}
\.


--
-- TOC entry 5411 (class 0 OID 17915)
-- Dependencies: 223
-- Data for Name: role_permissions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.role_permissions (role_id, permission_id) FROM stdin;
1	1
3	1
3	9
3	11
3	12
3	13
5	12
5	13
30	1
30	9
30	11
30	12
30	13
30	22
30	23
30	24
30	25
30	26
30	27
31	9
31	11
31	12
31	13
31	23
31	24
31	25
31	26
31	27
32	9
32	12
32	24
33	9
33	12
33	13
33	23
33	24
33	25
34	9
34	11
34	12
34	13
34	25
34	26
34	27
35	12
35	13
36	12
37	12
37	13
\.


--
-- TOC entry 5408 (class 0 OID 17890)
-- Dependencies: 220
-- Data for Name: roles; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.roles (id, name, description, created_at, updated_at) FROM stdin;
1	admin	Default role	2026-03-18 07:15:57.835045	2026-03-18 07:15:57.835045
2	customer	Default role	2026-03-18 07:15:57.838694	2026-03-18 07:15:57.838694
3	restaurant_owner	Default role	2026-03-18 07:15:57.841916	2026-03-18 07:15:57.841916
4	rider	Default role	2026-03-18 07:15:57.844069	2026-03-18 07:15:57.844069
5	restaurant_staff	Default role	2026-03-18 07:15:57.846135	2026-03-18 07:15:57.846135
30	org_owner	Organization owner with full access	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
31	org_admin	Organization administrator	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
32	finance_admin	Finance and billing administrator	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
33	operations_admin	Operations and logistics administrator	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
34	outlet_manager	Manager of a specific outlet	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
35	cashier	POS cashier staff	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
36	kitchen_staff	Kitchen / KDS staff	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
37	waiter	Front-of-house waiter	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
38	delivery_staff	Delivery personnel	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
39	support_staff	Customer support staff	2026-03-19 05:18:14.891105	2026-03-19 05:18:14.891105
\.


--
-- TOC entry 5451 (class 0 OID 18706)
-- Dependencies: 263
-- Data for Name: stock_alerts; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.stock_alerts (id, tenant_id, outlet_id, ingredient_id, severity, status, current_qty, threshold_qty, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5434 (class 0 OID 18438)
-- Dependencies: 246
-- Data for Name: unit_conversions; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.unit_conversions (conversion_id, from_unit_id, to_unit_id, factor, is_active, created_at) FROM stdin;
1	1	2	1000.000000	t	2026-03-18 07:28:12.437195
2	1	3	1000000.000000	t	2026-03-18 07:28:12.437195
3	1	4	2.204620	t	2026-03-18 07:28:12.437195
4	2	1	0.001000	t	2026-03-18 07:28:12.437195
5	2	3	1000.000000	t	2026-03-18 07:28:12.437195
6	2	5	0.035274	t	2026-03-18 07:28:12.437195
7	3	2	0.001000	t	2026-03-18 07:28:12.437195
8	3	1	0.000001	t	2026-03-18 07:28:12.437195
9	4	1	0.453592	t	2026-03-18 07:28:12.437195
10	4	5	16.000000	t	2026-03-18 07:28:12.437195
11	5	2	28.349500	t	2026-03-18 07:28:12.437195
12	5	4	0.062500	t	2026-03-18 07:28:12.437195
13	6	7	1000.000000	t	2026-03-18 07:28:12.437195
14	6	8	0.264172	t	2026-03-18 07:28:12.437195
15	7	6	0.001000	t	2026-03-18 07:28:12.437195
16	7	10	0.004227	t	2026-03-18 07:28:12.437195
17	7	11	0.067628	t	2026-03-18 07:28:12.437195
18	7	12	0.202884	t	2026-03-18 07:28:12.437195
19	8	6	3.785410	t	2026-03-18 07:28:12.437195
20	8	9	4.000000	t	2026-03-18 07:28:12.437195
21	10	7	236.588000	t	2026-03-18 07:28:12.437195
22	10	11	16.000000	t	2026-03-18 07:28:12.437195
23	11	7	14.786800	t	2026-03-18 07:28:12.437195
24	11	12	3.000000	t	2026-03-18 07:28:12.437195
25	12	7	4.928920	t	2026-03-18 07:28:12.437195
26	14	13	12.000000	t	2026-03-18 07:28:12.437195
27	13	14	0.083333	t	2026-03-18 07:28:12.437195
\.


--
-- TOC entry 5432 (class 0 OID 18423)
-- Dependencies: 244
-- Data for Name: units; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.units (unit_id, name, symbol, unit_type, description, is_active, created_at, updated_at) FROM stdin;
1	Kilogram	kg	weight	Standard metric unit of mass	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
2	Gram	g	weight	Metric unit of mass	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
3	Milligram	mg	weight	Metric unit of mass	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
4	Pound	lb	weight	Imperial unit of mass	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
5	Ounce	oz	weight	Imperial unit of mass	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
6	Liter	L	volume	Standard metric unit of volume	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
7	Milliliter	ml	volume	Metric unit of volume	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
8	Gallon	gal	volume	Imperial unit of volume	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
9	Quart	qt	volume	Imperial unit of volume	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
10	Cup	cup	volume	Cooking measurement unit	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
11	Tablespoon	tbsp	volume	Cooking measurement unit	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
12	Teaspoon	tsp	volume	Cooking measurement unit	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
13	Piece	pc	count	Individual item	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
14	Dozen	doz	count	12 pieces	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
15	Plate	plate	count	Serving plate	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
16	Portion	portion	count	Single serving portion	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
17	Box	box	count	Boxed items	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
18	Packet	pkt	count	Packaged items	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
19	Bottle	btl	count	Bottled items	t	2026-03-18 07:28:12.437195	2026-03-18 07:28:12.437195
\.


--
-- TOC entry 5477 (class 0 OID 19131)
-- Dependencies: 289
-- Data for Name: user_addresses; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.user_addresses (user_id, label, address_line1, address_line2, city, state, pincode, latitude, longitude, is_default, metadata, created_at, updated_at, id) FROM stdin;
\.


--
-- TOC entry 5474 (class 0 OID 18981)
-- Dependencies: 286
-- Data for Name: user_payment_methods; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.user_payment_methods (method_id, user_id, method_type, card_brand, card_last4, expiry_month, expiry_year, upi_id, is_default, metadata, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5413 (class 0 OID 18228)
-- Dependencies: 225
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.users (user_id, first_name, last_name, email, phone, created_at, phone_country_code, display_name, password_hash, auth_provider, primary_role, username, password_salt, oauth_id, roles, avatar_url, dob, gender, bio, addresses, default_address_id, loyalty_points, favorite_restaurants, default_payment_method_id, order_history_summary, business_name, business_legal_name, business_phone, business_registration_number, gstin, business_address, restaurant_ids, license_number, license_expiry, vehicle_type, vehicle_registration_number, vehicle_details, insurance_details, max_carry_capacity_kg, is_available, on_trip, current_lat, current_lng, last_location_update, email_verified, phone_verified, kyc_verified, kyc_data, verification_docs, bank_details, payout_methods, two_factor_enabled, two_factor_method, totp_secret, failed_login_attempts, locked_until, rating_avg, rating_count, total_deliveries, total_orders, earnings, device_info, preferences, settings, metadata, last_login_at, last_seen_at, last_known_ip, timezone, locale, status, created_by, updated_by, updated_at, deleted_at, referral_code, referred_by, search_vector, id) FROM stdin;
1	Gursevak	Singh	Admon@gmail.com	+918937294270	2026-03-18 08:30:24.23989	+91	Gursevak	$2a$10$mdtUmykzVDVId8DrZoLngOT88Drq7mezK3LAEJ9Znmc/ViBvL.2gy	local	customer	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 08:30:24.23989	\N	\N	\N	'+918937294270':4 'admon@gmail.com':3 'gursevak':1,5 'singh':2	7961de3b-9628-4a9a-b572-81b8a6311bee
2	Gursevak	Singh	Admon@gmail.com	+918937294270	2026-03-18 08:39:25.652544	+91	Gursevak	$2a$10$fIZpUGAhRb9nMmbcXZZ8SuaWuG05Ch5sMbiEP.fgAASlmm17N5emy	local	customer	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 08:39:25.652544	\N	\N	\N	'+918937294270':4 'admon@gmail.com':3 'gursevak':1,5 'singh':2	150908cd-a964-4679-b998-03094e5a214e
3	Gursevak	Singh	Admon@gmail.com	+918937294270	2026-03-18 08:48:15.970177	+91	Gursevak	$2a$10$OZxTDE.k1Cl7cAklI2DV3.TITXarovtmYvjZk5r2E1aIwyglZuBo2	local	customer	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 08:48:15.970177	\N	\N	\N	'+918937294270':4 'admon@gmail.com':3 'gursevak':1,5 'singh':2	6e3f2ee8-41f5-46cc-a435-0aeff4a8c9b5
4	Gursevak	Singh	Admon@gmail.com	+918937294270	2026-03-18 08:55:48.667148	+91	Gursevak	$2a$10$d3olOgCeg5mY4eUb7tyug..7/g7lpl4le/xvFZhW3n/Ma0aRb5HJy	local	customer	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 08:55:48.667148	\N	\N	\N	'+918937294270':4 'admon@gmail.com':3 'gursevak':1,5 'singh':2	3037ce11-c255-4c66-b9e9-e6ac6d30741e
5	Gursevak	Singh	Admon@gmail.com	+918937294270	2026-03-18 09:00:43.356186	+91	Gursevak	$2a$10$IYKV3z2WbcnSWcbNcpc7K.7IYUrfI.m0rAEe9q5T1dWtXLU0v3lZ2	local	customer	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 09:00:43.356186	\N	\N	\N	'+918937294270':4 'admon@gmail.com':3 'gursevak':1,5 'singh':2	dddb6b1e-56ec-449a-b404-521d11ebb0eb
6	Gursevak	Singh	Admon@gmail.com	+918937294270	2026-03-18 09:05:46.997504	+91	Gursevak	$2a$10$CuooOs3qtyYatlCjJ9pnd.O2NP2lP3wj/zxIBtq/Wj.Ft0ivyNSf2	local	customer	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 09:05:46.997504	\N	\N	\N	'+918937294270':4 'admon@gmail.com':3 'gursevak':1,5 'singh':2	891cfa9f-c88a-41fe-8399-d5b0a9d5ae8b
7	Gursevak	Singh	Admon@gmail.com	+918937294270	2026-03-18 09:06:45.456809	+91	Gursevak	$2a$10$SetYRWqqzuQH5yWo2if/SuDIl/fgTPXNOYGJuV4ZQMPk2uS3dbUBe	local	customer	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 09:06:45.456809	\N	\N	\N	'+918937294270':4 'admon@gmail.com':3 'gursevak':1,5 'singh':2	e4717ffa-5fd9-4670-88c7-10451c77991a
8	Harsh	Singh	harsh@gmail.com	+91703772781	2026-03-18 09:07:48.297606	+91	Gursevak	$2a$10$2sB5JXMwmFrhhGjxcwp6vu5BbaXg.DlXoz/Q4/EUWzYgNZ4RoL.iK	local	restaurant_owner	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 09:07:48.297606	\N	\N	\N	'+91703772781':4 'gursevak':5 'harsh':1 'harsh@gmail.com':3 'singh':2	5953969b-3354-49a4-be82-a9dcf19e91a9
9	gursevak		gursevak@gmail.com	+917037772781	2026-03-18 10:42:21.910771	+91	gursevak	$2a$10$QoPeUzoBJvhtuJn1yLFeJebqsi6ZUOGllIOY7vyWGTbHkAF4fgKmK	local	restaurant_staff	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 10:42:21.910771	\N	\N	\N	'+917037772781':3 'gursevak':1,4 'gursevak@gmail.com':2	5d993a8c-fa88-4196-b5be-842d9bf21563
10	Mohit		mohit1@gmail.com	+919085858685	2026-03-18 10:43:22.26553	+91	Mohit	$2a$10$URmjmjDHV66LrxI3Wr.EiuJgFO45xWxFrfLMB5VTo07TEn/iC/d1G	local	restaurant_staff	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 10:43:22.26553	\N	\N	\N	'+919085858685':3 'mohit':1,4 'mohit1@gmail.com':2	b6d8143b-4676-4f0f-9a40-c25798abb81d
11	Harsh@gmail.com		gursevaksinghgill21@gmail.com	+917037772781	2026-03-18 13:55:35.867045	+91	Harsh@gmail.com	$2a$10$JYTy2oCF6D2rGmwiy09rg.u/OTjO9Rc3y7OpD.4umN8io6GE5BERK	local	restaurant_owner	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	Taruna sweets	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-18 13:55:35.867045	\N	\N	\N	'+917037772781':3 'gursevaksinghgill21@gmail.com':2 'harsh@gmail.com':1,4 'sweet':6 'taruna':5	26ec8d15-a535-48d2-b9f9-3e19e4fc156f
12	Admin	singh	admin@gmail.com	+91703772782	2026-03-19 13:33:12.346454	+91	Gursevak	$2a$10$.0b7lXWmk.6SubGXN8M59.wi1DHlnG7ErGcj4jE2wn0RY0DOTkGSW	local	admin	\N	\N	\N	[]	\N	\N	\N	\N	[]	\N	0	[]	\N	\N	\N	\N	\N	\N	\N	\N	[]	\N	\N	\N	\N	\N	\N	\N	f	f	\N	\N	\N	f	f	f	\N	\N	\N	\N	f	\N	\N	0	\N	\N	0	0	0	0.00	\N	\N	\N	\N	\N	\N	\N	\N	\N	active	\N	\N	2026-03-19 13:33:12.346454	\N	\N	\N	'+91703772782':4 'admin':1 'admin@gmail.com':3 'gursevak':5 'singh':2	ec968f62-ca87-4498-899f-661e2c4881c4
\.


--
-- TOC entry 5436 (class 0 OID 18463)
-- Dependencies: 248
-- Data for Name: vendors; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.vendors (vendor_id, tenant_id, name, contact_person, email, phone, address, city, state, postal_code, gst_number, payment_terms, is_active, notes, created_at, updated_at) FROM stdin;
\.


--
-- TOC entry 5464 (class 0 OID 18870)
-- Dependencies: 276
-- Data for Name: wallet_ledger; Type: TABLE DATA; Schema: public; Owner: postgres
--

COPY public.wallet_ledger (id, tenant_id, outlet_id, customer_id, direction, amount, ref_type, ref_id, meta, created_at) FROM stdin;
\.


--
-- TOC entry 5546 (class 0 OID 0)
-- Dependencies: 240
-- Name: bank_verifications_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.bank_verifications_id_seq', 1, false);


--
-- TOC entry 5547 (class 0 OID 0)
-- Dependencies: 298
-- Name: incentive_cycles_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.incentive_cycles_id_seq', 1, true);


--
-- TOC entry 5548 (class 0 OID 0)
-- Dependencies: 300
-- Name: incentive_milestones_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.incentive_milestones_id_seq', 3, true);


--
-- TOC entry 5549 (class 0 OID 0)
-- Dependencies: 249
-- Name: ingredients_ingredient_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.ingredients_ingredient_id_seq', 1, false);


--
-- TOC entry 5550 (class 0 OID 0)
-- Dependencies: 253
-- Name: inventory_recipe_lines_recipe_line_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.inventory_recipe_lines_recipe_line_id_seq', 1, false);


--
-- TOC entry 5551 (class 0 OID 0)
-- Dependencies: 251
-- Name: inventory_recipes_recipe_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.inventory_recipes_recipe_id_seq', 1, false);


--
-- TOC entry 5552 (class 0 OID 0)
-- Dependencies: 255
-- Name: inventory_stock_ledger_ledger_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.inventory_stock_ledger_ledger_id_seq', 1, false);


--
-- TOC entry 5553 (class 0 OID 0)
-- Dependencies: 232
-- Name: menu_categories_category_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.menu_categories_category_id_seq', 8, true);


--
-- TOC entry 5554 (class 0 OID 0)
-- Dependencies: 292
-- Name: menu_item_addons_addon_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.menu_item_addons_addon_id_seq', 1, false);


--
-- TOC entry 5555 (class 0 OID 0)
-- Dependencies: 302
-- Name: menu_item_combos_combo_item_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.menu_item_combos_combo_item_id_seq', 5, true);


--
-- TOC entry 5556 (class 0 OID 0)
-- Dependencies: 290
-- Name: menu_item_variants_variant_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.menu_item_variants_variant_id_seq', 5, true);


--
-- TOC entry 5557 (class 0 OID 0)
-- Dependencies: 234
-- Name: menu_items_item_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.menu_items_item_id_seq', 9, true);


--
-- TOC entry 5558 (class 0 OID 0)
-- Dependencies: 296
-- Name: order_item_addons_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.order_item_addons_id_seq', 1, false);


--
-- TOC entry 5559 (class 0 OID 0)
-- Dependencies: 294
-- Name: order_item_variants_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.order_item_variants_id_seq', 4, true);


--
-- TOC entry 5560 (class 0 OID 0)
-- Dependencies: 238
-- Name: order_items_order_item_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.order_items_order_item_id_seq', 48, true);


--
-- TOC entry 5561 (class 0 OID 0)
-- Dependencies: 236
-- Name: orders_order_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.orders_order_id_seq', 39, true);


--
-- TOC entry 5562 (class 0 OID 0)
-- Dependencies: 284
-- Name: payment_sessions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.payment_sessions_id_seq', 1, false);


--
-- TOC entry 5563 (class 0 OID 0)
-- Dependencies: 221
-- Name: permissions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.permissions_id_seq', 27, true);


--
-- TOC entry 5564 (class 0 OID 0)
-- Dependencies: 282
-- Name: qr_sessions_session_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.qr_sessions_session_id_seq', 1, false);


--
-- TOC entry 5565 (class 0 OID 0)
-- Dependencies: 228
-- Name: restaurant_hours_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.restaurant_hours_id_seq', 1, false);


--
-- TOC entry 5566 (class 0 OID 0)
-- Dependencies: 230
-- Name: restaurant_tables_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.restaurant_tables_id_seq', 1, false);


--
-- TOC entry 5567 (class 0 OID 0)
-- Dependencies: 226
-- Name: restaurants_restaurant_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.restaurants_restaurant_id_seq', 1, true);


--
-- TOC entry 5568 (class 0 OID 0)
-- Dependencies: 219
-- Name: roles_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.roles_id_seq', 39, true);


--
-- TOC entry 5569 (class 0 OID 0)
-- Dependencies: 245
-- Name: unit_conversions_conversion_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.unit_conversions_conversion_id_seq', 54, true);


--
-- TOC entry 5570 (class 0 OID 0)
-- Dependencies: 243
-- Name: units_unit_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.units_unit_id_seq', 38, true);


--
-- TOC entry 5571 (class 0 OID 0)
-- Dependencies: 224
-- Name: users_user_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.users_user_id_seq', 12, true);


--
-- TOC entry 5572 (class 0 OID 0)
-- Dependencies: 247
-- Name: vendors_vendor_id_seq; Type: SEQUENCE SET; Schema: public; Owner: postgres
--

SELECT pg_catalog.setval('public.vendors_vendor_id_seq', 1, false);


--
-- TOC entry 5006 (class 2606 OID 18746)
-- Name: api_idempotency_keys api_idempotency_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.api_idempotency_keys
    ADD CONSTRAINT api_idempotency_keys_pkey PRIMARY KEY (tenant_id, endpoint, idempotency_key);


--
-- TOC entry 4935 (class 2606 OID 18382)
-- Name: bank_verifications bank_verifications_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bank_verifications
    ADD CONSTRAINT bank_verifications_pkey PRIMARY KEY (id);


--
-- TOC entry 5156 (class 2606 OID 19673)
-- Name: customer_devices customer_devices_device_id_restaurant_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_devices
    ADD CONSTRAINT customer_devices_device_id_restaurant_id_key UNIQUE (device_id, restaurant_id);


--
-- TOC entry 5158 (class 2606 OID 19671)
-- Name: customer_devices customer_devices_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_devices
    ADD CONSTRAINT customer_devices_pkey PRIMARY KEY (id);


--
-- TOC entry 5167 (class 2606 OID 19718)
-- Name: customer_favorite_items customer_favorite_items_customer_id_menu_item_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_favorite_items
    ADD CONSTRAINT customer_favorite_items_customer_id_menu_item_id_key UNIQUE (customer_id, menu_item_id);


--
-- TOC entry 5169 (class 2606 OID 19716)
-- Name: customer_favorite_items customer_favorite_items_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_favorite_items
    ADD CONSTRAINT customer_favorite_items_pkey PRIMARY KEY (id);


--
-- TOC entry 5013 (class 2606 OID 18772)
-- Name: customer_feedback customer_feedback_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_feedback
    ADD CONSTRAINT customer_feedback_pkey PRIMARY KEY (id);


--
-- TOC entry 5163 (class 2606 OID 19696)
-- Name: customer_visits customer_visits_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_visits
    ADD CONSTRAINT customer_visits_pkey PRIMARY KEY (id);


--
-- TOC entry 5040 (class 2606 OID 18869)
-- Name: customer_wallet customer_wallet_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_wallet
    ADD CONSTRAINT customer_wallet_pkey PRIMARY KEY (tenant_id, outlet_id, customer_id);


--
-- TOC entry 5150 (class 2606 OID 19651)
-- Name: customers customers_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_pkey PRIMARY KEY (id);


--
-- TOC entry 4987 (class 2606 OID 18672)
-- Name: daily_outlet_metrics daily_outlet_metrics_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.daily_outlet_metrics
    ADD CONSTRAINT daily_outlet_metrics_pkey PRIMARY KEY (tenant_id, outlet_id, day);


--
-- TOC entry 5018 (class 2606 OID 18784)
-- Name: feedback_events feedback_events_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.feedback_events
    ADD CONSTRAINT feedback_events_pkey PRIMARY KEY (id);


--
-- TOC entry 5078 (class 2606 OID 19022)
-- Name: foodshare_participants foodshare_participants_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.foodshare_participants
    ADD CONSTRAINT foodshare_participants_pkey PRIMARY KEY (session_id, user_id);


--
-- TOC entry 5074 (class 2606 OID 19007)
-- Name: foodshare_sessions foodshare_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.foodshare_sessions
    ADD CONSTRAINT foodshare_sessions_pkey PRIMARY KEY (session_id);


--
-- TOC entry 5105 (class 2606 OID 19282)
-- Name: incentive_cycles incentive_cycles_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.incentive_cycles
    ADD CONSTRAINT incentive_cycles_pkey PRIMARY KEY (id);


--
-- TOC entry 5107 (class 2606 OID 19297)
-- Name: incentive_milestones incentive_milestones_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.incentive_milestones
    ADD CONSTRAINT incentive_milestones_pkey PRIMARY KEY (id);


--
-- TOC entry 4964 (class 2606 OID 18493)
-- Name: ingredients ingredients_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ingredients
    ADD CONSTRAINT ingredients_pkey PRIMARY KEY (ingredient_id);


--
-- TOC entry 4966 (class 2606 OID 18495)
-- Name: ingredients ingredients_tenant_id_restaurant_id_sku_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ingredients
    ADD CONSTRAINT ingredients_tenant_id_restaurant_id_sku_key UNIQUE (tenant_id, restaurant_id, sku);


--
-- TOC entry 4979 (class 2606 OID 18600)
-- Name: inventory_idempotency_keys inventory_idempotency_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_idempotency_keys
    ADD CONSTRAINT inventory_idempotency_keys_pkey PRIMARY KEY (tenant_id, endpoint, idempotency_key);


--
-- TOC entry 4973 (class 2606 OID 18565)
-- Name: inventory_recipe_lines inventory_recipe_lines_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipe_lines
    ADD CONSTRAINT inventory_recipe_lines_pkey PRIMARY KEY (recipe_line_id);


--
-- TOC entry 4969 (class 2606 OID 18546)
-- Name: inventory_recipes inventory_recipes_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipes
    ADD CONSTRAINT inventory_recipes_pkey PRIMARY KEY (recipe_id);


--
-- TOC entry 4971 (class 2606 OID 18548)
-- Name: inventory_recipes inventory_recipes_tenant_id_outlet_id_menu_item_id_variant__key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipes
    ADD CONSTRAINT inventory_recipes_tenant_id_outlet_id_menu_item_id_variant__key UNIQUE (tenant_id, outlet_id, menu_item_id, variant, version);


--
-- TOC entry 4977 (class 2606 OID 18586)
-- Name: inventory_stock_ledger inventory_stock_ledger_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_stock_ledger
    ADD CONSTRAINT inventory_stock_ledger_pkey PRIMARY KEY (ledger_id);


--
-- TOC entry 5004 (class 2606 OID 18732)
-- Name: inventory_stock_snapshots inventory_stock_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_stock_snapshots
    ADD CONSTRAINT inventory_stock_snapshots_pkey PRIMARY KEY (tenant_id, outlet_id, ingredient_id);


--
-- TOC entry 5173 (class 2606 OID 19745)
-- Name: item_pairings item_pairings_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.item_pairings
    ADD CONSTRAINT item_pairings_pkey PRIMARY KEY (id);


--
-- TOC entry 5175 (class 2606 OID 19747)
-- Name: item_pairings item_pairings_restaurant_id_source_item_id_target_item_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.item_pairings
    ADD CONSTRAINT item_pairings_restaurant_id_source_item_id_target_item_id_key UNIQUE (restaurant_id, source_item_id, target_item_id);


--
-- TOC entry 5045 (class 2606 OID 18888)
-- Name: loyalty_points loyalty_points_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.loyalty_points
    ADD CONSTRAINT loyalty_points_pkey PRIMARY KEY (tenant_id, outlet_id, customer_id);


--
-- TOC entry 4917 (class 2606 OID 18290)
-- Name: menu_categories menu_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_categories
    ADD CONSTRAINT menu_categories_pkey PRIMARY KEY (category_id);


--
-- TOC entry 5097 (class 2606 OID 19218)
-- Name: menu_item_addons menu_item_addons_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_addons
    ADD CONSTRAINT menu_item_addons_pkey PRIMARY KEY (addon_id);


--
-- TOC entry 5112 (class 2606 OID 19314)
-- Name: menu_item_combos menu_item_combos_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_combos
    ADD CONSTRAINT menu_item_combos_pkey PRIMARY KEY (combo_item_id);


--
-- TOC entry 4991 (class 2606 OID 18685)
-- Name: menu_item_daily_metrics menu_item_daily_metrics_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_daily_metrics
    ADD CONSTRAINT menu_item_daily_metrics_pkey PRIMARY KEY (tenant_id, outlet_id, day, menu_item_id);


--
-- TOC entry 5089 (class 2606 OID 19193)
-- Name: menu_item_variants menu_item_variants_menu_item_id_variant_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_variants
    ADD CONSTRAINT menu_item_variants_menu_item_id_variant_name_key UNIQUE (menu_item_id, variant_name);


--
-- TOC entry 5091 (class 2606 OID 19191)
-- Name: menu_item_variants menu_item_variants_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_variants
    ADD CONSTRAINT menu_item_variants_pkey PRIMARY KEY (variant_id);


--
-- TOC entry 4921 (class 2606 OID 18312)
-- Name: menu_items menu_items_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_items
    ADD CONSTRAINT menu_items_pkey PRIMARY KEY (item_id);


--
-- TOC entry 4997 (class 2606 OID 18704)
-- Name: notification_devices notification_devices_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.notification_devices
    ADD CONSTRAINT notification_devices_pkey PRIMARY KEY (id);


--
-- TOC entry 5034 (class 2606 OID 18856)
-- Name: offer_applications offer_applications_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_applications
    ADD CONSTRAINT offer_applications_pkey PRIMARY KEY (id);


--
-- TOC entry 5036 (class 2606 OID 18860)
-- Name: offer_applications offer_applications_tenant_id_outlet_id_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_applications
    ADD CONSTRAINT offer_applications_tenant_id_outlet_id_idempotency_key_key UNIQUE (tenant_id, outlet_id, idempotency_key);


--
-- TOC entry 5038 (class 2606 OID 18858)
-- Name: offer_applications offer_applications_tenant_id_outlet_id_order_id_offer_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_applications
    ADD CONSTRAINT offer_applications_tenant_id_outlet_id_order_id_offer_id_key UNIQUE (tenant_id, outlet_id, order_id, offer_id);


--
-- TOC entry 5032 (class 2606 OID 18842)
-- Name: offer_benefits offer_benefits_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_benefits
    ADD CONSTRAINT offer_benefits_pkey PRIMARY KEY (id);


--
-- TOC entry 5030 (class 2606 OID 18829)
-- Name: offer_rules offer_rules_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_rules
    ADD CONSTRAINT offer_rules_pkey PRIMARY KEY (id);


--
-- TOC entry 5182 (class 2606 OID 19814)
-- Name: offer_suggested_items offer_suggested_items_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_suggested_items
    ADD CONSTRAINT offer_suggested_items_pkey PRIMARY KEY (id);


--
-- TOC entry 5028 (class 2606 OID 18820)
-- Name: offers offers_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offers
    ADD CONSTRAINT offers_pkey PRIMARY KEY (id);


--
-- TOC entry 5103 (class 2606 OID 19260)
-- Name: order_item_addons order_item_addons_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_addons
    ADD CONSTRAINT order_item_addons_pkey PRIMARY KEY (id);


--
-- TOC entry 5100 (class 2606 OID 19240)
-- Name: order_item_variants order_item_variants_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_variants
    ADD CONSTRAINT order_item_variants_pkey PRIMARY KEY (id);


--
-- TOC entry 4933 (class 2606 OID 18358)
-- Name: order_items order_items_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT order_items_pkey PRIMARY KEY (order_item_id);


--
-- TOC entry 4931 (class 2606 OID 18341)
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (order_id);


--
-- TOC entry 5129 (class 2606 OID 19548)
-- Name: organization_users organization_users_organization_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.organization_users
    ADD CONSTRAINT organization_users_organization_id_user_id_key UNIQUE (organization_id, user_id);


--
-- TOC entry 5131 (class 2606 OID 19546)
-- Name: organization_users organization_users_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.organization_users
    ADD CONSTRAINT organization_users_pkey PRIMARY KEY (id);


--
-- TOC entry 5123 (class 2606 OID 19517)
-- Name: organizations organizations_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);


--
-- TOC entry 5125 (class 2606 OID 19519)
-- Name: organizations organizations_slug_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.organizations
    ADD CONSTRAINT organizations_slug_key UNIQUE (slug);


--
-- TOC entry 5141 (class 2606 OID 19593)
-- Name: outlet_menu_overrides outlet_menu_overrides_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_menu_overrides
    ADD CONSTRAINT outlet_menu_overrides_pkey PRIMARY KEY (id);


--
-- TOC entry 5143 (class 2606 OID 19595)
-- Name: outlet_menu_overrides outlet_menu_overrides_restaurant_id_menu_item_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_menu_overrides
    ADD CONSTRAINT outlet_menu_overrides_restaurant_id_menu_item_id_key UNIQUE (restaurant_id, menu_item_id);


--
-- TOC entry 5024 (class 2606 OID 18806)
-- Name: outlet_offer_constraints outlet_offer_constraints_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_offer_constraints
    ADD CONSTRAINT outlet_offer_constraints_pkey PRIMARY KEY (tenant_id, outlet_id);


--
-- TOC entry 5022 (class 2606 OID 18795)
-- Name: outlet_review_settings outlet_review_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_review_settings
    ADD CONSTRAINT outlet_review_settings_pkey PRIMARY KEY (tenant_id, outlet_id);


--
-- TOC entry 5146 (class 2606 OID 19616)
-- Name: outlet_settings_overrides outlet_settings_overrides_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_settings_overrides
    ADD CONSTRAINT outlet_settings_overrides_pkey PRIMARY KEY (id);


--
-- TOC entry 5148 (class 2606 OID 19618)
-- Name: outlet_settings_overrides outlet_settings_overrides_restaurant_id_setting_key_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_settings_overrides
    ADD CONSTRAINT outlet_settings_overrides_restaurant_id_setting_key_key UNIQUE (restaurant_id, setting_key);


--
-- TOC entry 5136 (class 2606 OID 19566)
-- Name: outlet_staff_assignments outlet_staff_assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_staff_assignments
    ADD CONSTRAINT outlet_staff_assignments_pkey PRIMARY KEY (id);


--
-- TOC entry 5138 (class 2606 OID 19568)
-- Name: outlet_staff_assignments outlet_staff_assignments_user_id_restaurant_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_staff_assignments
    ADD CONSTRAINT outlet_staff_assignments_user_id_restaurant_id_key UNIQUE (user_id, restaurant_id);


--
-- TOC entry 4995 (class 2606 OID 18694)
-- Name: owner_insights owner_insights_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.owner_insights
    ADD CONSTRAINT owner_insights_pkey PRIMARY KEY (id);


--
-- TOC entry 5047 (class 2606 OID 18899)
-- Name: owner_referral_codes owner_referral_codes_code_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.owner_referral_codes
    ADD CONSTRAINT owner_referral_codes_code_key UNIQUE (code);


--
-- TOC entry 5049 (class 2606 OID 18897)
-- Name: owner_referral_codes owner_referral_codes_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.owner_referral_codes
    ADD CONSTRAINT owner_referral_codes_pkey PRIMARY KEY (id);


--
-- TOC entry 5053 (class 2606 OID 18909)
-- Name: owner_referrals owner_referrals_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.owner_referrals
    ADD CONSTRAINT owner_referrals_pkey PRIMARY KEY (id);


--
-- TOC entry 5065 (class 2606 OID 18969)
-- Name: payment_sessions payment_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_sessions
    ADD CONSTRAINT payment_sessions_pkey PRIMARY KEY (id);


--
-- TOC entry 4894 (class 2606 OID 17914)
-- Name: permissions permissions_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_name_key UNIQUE (name);


--
-- TOC entry 4896 (class 2606 OID 17912)
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- TOC entry 4985 (class 2606 OID 18649)
-- Name: pos_events pos_events_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.pos_events
    ADD CONSTRAINT pos_events_pkey PRIMARY KEY (id);


--
-- TOC entry 5062 (class 2606 OID 18950)
-- Name: qr_sessions qr_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.qr_sessions
    ADD CONSTRAINT qr_sessions_pkey PRIMARY KEY (session_id);


--
-- TOC entry 5115 (class 2606 OID 19480)
-- Name: qr_tokens qr_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.qr_tokens
    ADD CONSTRAINT qr_tokens_pkey PRIMARY KEY (token);


--
-- TOC entry 5009 (class 2606 OID 18757)
-- Name: rating_links rating_links_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rating_links
    ADD CONSTRAINT rating_links_pkey PRIMARY KEY (id);


--
-- TOC entry 5011 (class 2606 OID 18759)
-- Name: rating_links rating_links_token_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.rating_links
    ADD CONSTRAINT rating_links_token_key UNIQUE (token);


--
-- TOC entry 5179 (class 2606 OID 19779)
-- Name: recommendation_events recommendation_events_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.recommendation_events
    ADD CONSTRAINT recommendation_events_pkey PRIMARY KEY (id);


--
-- TOC entry 5058 (class 2606 OID 18934)
-- Name: referral_attribution_events referral_attribution_events_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.referral_attribution_events
    ADD CONSTRAINT referral_attribution_events_pkey PRIMARY KEY (id);


--
-- TOC entry 5056 (class 2606 OID 18920)
-- Name: referral_rewards referral_rewards_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.referral_rewards
    ADD CONSTRAINT referral_rewards_pkey PRIMARY KEY (id);


--
-- TOC entry 4913 (class 2606 OID 18257)
-- Name: restaurant_hours restaurant_hours_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_hours
    ADD CONSTRAINT restaurant_hours_pkey PRIMARY KEY (id);


--
-- TOC entry 5119 (class 2606 OID 19493)
-- Name: restaurant_sessions restaurant_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_sessions
    ADD CONSTRAINT restaurant_sessions_pkey PRIMARY KEY (session_id);


--
-- TOC entry 4940 (class 2606 OID 19472)
-- Name: restaurant_staff restaurant_staff_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_staff
    ADD CONSTRAINT restaurant_staff_pkey PRIMARY KEY (id);


--
-- TOC entry 4942 (class 2606 OID 18400)
-- Name: restaurant_staff restaurant_staff_user_id_restaurant_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_staff
    ADD CONSTRAINT restaurant_staff_user_id_restaurant_id_key UNIQUE (user_id, restaurant_id);


--
-- TOC entry 4915 (class 2606 OID 18273)
-- Name: restaurant_tables restaurant_tables_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_tables
    ADD CONSTRAINT restaurant_tables_pkey PRIMARY KEY (id);


--
-- TOC entry 4911 (class 2606 OID 18247)
-- Name: restaurants restaurants_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurants
    ADD CONSTRAINT restaurants_pkey PRIMARY KEY (restaurant_id);


--
-- TOC entry 4898 (class 2606 OID 17919)
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- TOC entry 4890 (class 2606 OID 17901)
-- Name: roles roles_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_name_key UNIQUE (name);


--
-- TOC entry 4892 (class 2606 OID 17899)
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- TOC entry 5001 (class 2606 OID 18717)
-- Name: stock_alerts stock_alerts_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_alerts
    ADD CONSTRAINT stock_alerts_pkey PRIMARY KEY (id);


--
-- TOC entry 4950 (class 2606 OID 18449)
-- Name: unit_conversions unit_conversions_from_unit_id_to_unit_id_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit_conversions
    ADD CONSTRAINT unit_conversions_from_unit_id_to_unit_id_key UNIQUE (from_unit_id, to_unit_id);


--
-- TOC entry 4952 (class 2606 OID 18447)
-- Name: unit_conversions unit_conversions_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit_conversions
    ADD CONSTRAINT unit_conversions_pkey PRIMARY KEY (conversion_id);


--
-- TOC entry 4944 (class 2606 OID 18436)
-- Name: units units_name_key; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.units
    ADD CONSTRAINT units_name_key UNIQUE (name);


--
-- TOC entry 4946 (class 2606 OID 18434)
-- Name: units units_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.units
    ADD CONSTRAINT units_pkey PRIMARY KEY (unit_id);


--
-- TOC entry 5084 (class 2606 OID 19461)
-- Name: user_addresses user_addresses_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_addresses
    ADD CONSTRAINT user_addresses_pkey PRIMARY KEY (id);


--
-- TOC entry 5072 (class 2606 OID 18992)
-- Name: user_payment_methods user_payment_methods_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.user_payment_methods
    ADD CONSTRAINT user_payment_methods_pkey PRIMARY KEY (method_id);


--
-- TOC entry 4907 (class 2606 OID 19447)
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- TOC entry 4956 (class 2606 OID 18474)
-- Name: vendors vendors_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.vendors
    ADD CONSTRAINT vendors_pkey PRIMARY KEY (vendor_id);


--
-- TOC entry 5043 (class 2606 OID 18878)
-- Name: wallet_ledger wallet_ledger_pkey; Type: CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.wallet_ledger
    ADD CONSTRAINT wallet_ledger_pkey PRIMARY KEY (id);


--
-- TOC entry 5092 (class 1259 OID 19232)
-- Name: idx_addons_available; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_addons_available ON public.menu_item_addons USING btree (is_available);


--
-- TOC entry 5093 (class 1259 OID 19230)
-- Name: idx_addons_category_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_addons_category_id ON public.menu_item_addons USING btree (category_id);


--
-- TOC entry 5094 (class 1259 OID 19229)
-- Name: idx_addons_item_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_addons_item_id ON public.menu_item_addons USING btree (item_id);


--
-- TOC entry 5095 (class 1259 OID 19231)
-- Name: idx_addons_restaurant_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_addons_restaurant_id ON public.menu_item_addons USING btree (restaurant_id);


--
-- TOC entry 4936 (class 1259 OID 18388)
-- Name: idx_bank_verifications_payout_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_bank_verifications_payout_id ON public.bank_verifications USING btree (payout_id);


--
-- TOC entry 5159 (class 1259 OID 19685)
-- Name: idx_customer_devices_customer; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_devices_customer ON public.customer_devices USING btree (customer_id);


--
-- TOC entry 5160 (class 1259 OID 19684)
-- Name: idx_customer_devices_device; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_devices_device ON public.customer_devices USING btree (device_id);


--
-- TOC entry 5161 (class 1259 OID 19686)
-- Name: idx_customer_devices_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_devices_restaurant ON public.customer_devices USING btree (restaurant_id);


--
-- TOC entry 5170 (class 1259 OID 19729)
-- Name: idx_customer_fav_customer; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_fav_customer ON public.customer_favorite_items USING btree (customer_id, order_count DESC);


--
-- TOC entry 5014 (class 1259 OID 18775)
-- Name: idx_customer_feedback_tenant_order; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_feedback_tenant_order ON public.customer_feedback USING btree (tenant_id, order_id) WHERE (order_id IS NOT NULL);


--
-- TOC entry 5015 (class 1259 OID 18773)
-- Name: idx_customer_feedback_tenant_outlet_created; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_feedback_tenant_outlet_created ON public.customer_feedback USING btree (tenant_id, outlet_id, created_at DESC);


--
-- TOC entry 5016 (class 1259 OID 18774)
-- Name: idx_customer_feedback_tenant_outlet_rating_created; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_feedback_tenant_outlet_rating_created ON public.customer_feedback USING btree (tenant_id, outlet_id, rating, created_at DESC);


--
-- TOC entry 5164 (class 1259 OID 19707)
-- Name: idx_customer_visits_customer; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_visits_customer ON public.customer_visits USING btree (customer_id, started_at DESC);


--
-- TOC entry 5165 (class 1259 OID 19708)
-- Name: idx_customer_visits_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customer_visits_restaurant ON public.customer_visits USING btree (restaurant_id, started_at DESC);


--
-- TOC entry 5151 (class 1259 OID 19660)
-- Name: idx_customers_last_visit; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customers_last_visit ON public.customers USING btree (restaurant_id, last_visit_at DESC);


--
-- TOC entry 5152 (class 1259 OID 19659)
-- Name: idx_customers_phone; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customers_phone ON public.customers USING btree (phone_number) WHERE (phone_number IS NOT NULL);


--
-- TOC entry 5153 (class 1259 OID 19657)
-- Name: idx_customers_phone_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_customers_phone_restaurant ON public.customers USING btree (restaurant_id, phone_number) WHERE ((phone_number IS NOT NULL) AND ((phone_number)::text <> ''::text));


--
-- TOC entry 5154 (class 1259 OID 19658)
-- Name: idx_customers_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_customers_restaurant ON public.customers USING btree (restaurant_id);


--
-- TOC entry 4988 (class 1259 OID 18673)
-- Name: idx_daily_outlet_metrics_tenant_outlet_day; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_daily_outlet_metrics_tenant_outlet_day ON public.daily_outlet_metrics USING btree (tenant_id, outlet_id, day DESC);


--
-- TOC entry 5019 (class 1259 OID 18785)
-- Name: idx_feedback_events_tenant_outlet_created; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_feedback_events_tenant_outlet_created ON public.feedback_events USING btree (tenant_id, outlet_id, created_at DESC);


--
-- TOC entry 5020 (class 1259 OID 18786)
-- Name: idx_feedback_events_tenant_outlet_type_created; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_feedback_events_tenant_outlet_type_created ON public.feedback_events USING btree (tenant_id, outlet_id, event_type, created_at DESC);


--
-- TOC entry 5079 (class 1259 OID 19028)
-- Name: idx_foodshare_participants_user_joined; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_foodshare_participants_user_joined ON public.foodshare_participants USING btree (user_id, joined_at DESC);


--
-- TOC entry 5075 (class 1259 OID 19014)
-- Name: idx_foodshare_sessions_restaurant_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_foodshare_sessions_restaurant_status ON public.foodshare_sessions USING btree (restaurant_id, status, created_at DESC);


--
-- TOC entry 5076 (class 1259 OID 19013)
-- Name: idx_foodshare_sessions_status_expires; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_foodshare_sessions_status_expires ON public.foodshare_sessions USING btree (status, expires_at);


--
-- TOC entry 4957 (class 1259 OID 18516)
-- Name: idx_ingredients_base_unit; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ingredients_base_unit ON public.ingredients USING btree (base_unit_id);


--
-- TOC entry 4958 (class 1259 OID 18513)
-- Name: idx_ingredients_category; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ingredients_category ON public.ingredients USING btree (category) WHERE (is_active = true);


--
-- TOC entry 4959 (class 1259 OID 18514)
-- Name: idx_ingredients_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ingredients_name ON public.ingredients USING btree (name);


--
-- TOC entry 4960 (class 1259 OID 18512)
-- Name: idx_ingredients_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ingredients_restaurant ON public.ingredients USING btree (restaurant_id) WHERE (is_active = true);


--
-- TOC entry 4961 (class 1259 OID 18515)
-- Name: idx_ingredients_sku; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ingredients_sku ON public.ingredients USING btree (sku) WHERE (sku IS NOT NULL);


--
-- TOC entry 4962 (class 1259 OID 18511)
-- Name: idx_ingredients_tenant_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_ingredients_tenant_restaurant ON public.ingredients USING btree (tenant_id, restaurant_id) WHERE (is_active = true);


--
-- TOC entry 4967 (class 1259 OID 18601)
-- Name: idx_inventory_recipes_active_lookup; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inventory_recipes_active_lookup ON public.inventory_recipes USING btree (tenant_id, menu_item_id, variant, is_active);


--
-- TOC entry 4974 (class 1259 OID 18602)
-- Name: idx_inventory_stock_ledger_query; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inventory_stock_ledger_query ON public.inventory_stock_ledger USING btree (tenant_id, outlet_id, ingredient_id, created_at);


--
-- TOC entry 4975 (class 1259 OID 18603)
-- Name: idx_inventory_stock_ledger_ref; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inventory_stock_ledger_ref ON public.inventory_stock_ledger USING btree (ref_type, ref_id);


--
-- TOC entry 5002 (class 1259 OID 18738)
-- Name: idx_inventory_stock_snapshots_outlet; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_inventory_stock_snapshots_outlet ON public.inventory_stock_snapshots USING btree (tenant_id, outlet_id, updated_at DESC);


--
-- TOC entry 5171 (class 1259 OID 19763)
-- Name: idx_item_pairings_source; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_item_pairings_source ON public.item_pairings USING btree (restaurant_id, source_item_id) WHERE (is_active = true);


--
-- TOC entry 5108 (class 1259 OID 19330)
-- Name: idx_menu_item_combos_combo_menu_item_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_menu_item_combos_combo_menu_item_id ON public.menu_item_combos USING btree (combo_menu_item_id);


--
-- TOC entry 5109 (class 1259 OID 19331)
-- Name: idx_menu_item_combos_item_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_menu_item_combos_item_id ON public.menu_item_combos USING btree (item_id);


--
-- TOC entry 5110 (class 1259 OID 19332)
-- Name: idx_menu_item_combos_variant_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_menu_item_combos_variant_id ON public.menu_item_combos USING btree (variant_id);


--
-- TOC entry 4989 (class 1259 OID 18686)
-- Name: idx_menu_item_daily_metrics_tenant_outlet_day; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_menu_item_daily_metrics_tenant_outlet_day ON public.menu_item_daily_metrics USING btree (tenant_id, outlet_id, day DESC);


--
-- TOC entry 4918 (class 1259 OID 18419)
-- Name: idx_menu_items_is_qrunch; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_menu_items_is_qrunch ON public.menu_items USING btree (is_qrunch) WHERE (is_qrunch = true);


--
-- TOC entry 4919 (class 1259 OID 19175)
-- Name: idx_menu_items_measurement_unit; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_menu_items_measurement_unit ON public.menu_items USING btree (measurement_unit);


--
-- TOC entry 5180 (class 1259 OID 19825)
-- Name: idx_offer_suggested_items_offer; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_offer_suggested_items_offer ON public.offer_suggested_items USING btree (offer_id);


--
-- TOC entry 5025 (class 1259 OID 18821)
-- Name: idx_offers_lookup; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_offers_lookup ON public.offers USING btree (tenant_id, outlet_id, status, start_at, end_at, priority);


--
-- TOC entry 5026 (class 1259 OID 19800)
-- Name: idx_offers_trigger_context; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_offers_trigger_context ON public.offers USING btree (tenant_id, outlet_id, trigger_context, status) WHERE (status = 'ACTIVE'::text);


--
-- TOC entry 5101 (class 1259 OID 19271)
-- Name: idx_order_item_addons_order_item; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_order_item_addons_order_item ON public.order_item_addons USING btree (order_item_id);


--
-- TOC entry 5098 (class 1259 OID 19251)
-- Name: idx_order_item_variants_order_item; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_order_item_variants_order_item ON public.order_item_variants USING btree (order_item_id);


--
-- TOC entry 4922 (class 1259 OID 18421)
-- Name: idx_orders_created_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_created_at ON public.orders USING btree (created_at DESC);


--
-- TOC entry 4923 (class 1259 OID 18979)
-- Name: idx_orders_customer_created_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_customer_created_at ON public.orders USING btree (customer_id, created_at DESC);


--
-- TOC entry 4924 (class 1259 OID 18958)
-- Name: idx_orders_dining_session_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_dining_session_id ON public.orders USING btree (dining_session_id) WHERE (dining_session_id IS NOT NULL);


--
-- TOC entry 4925 (class 1259 OID 19174)
-- Name: idx_orders_kds_active; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_kds_active ON public.orders USING btree (restaurant_id, kds_status) WHERE ((kds_status)::text <> 'SERVED'::text);


--
-- TOC entry 4926 (class 1259 OID 18980)
-- Name: idx_orders_order_number; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_order_number ON public.orders USING btree (order_number) WHERE (order_number IS NOT NULL);


--
-- TOC entry 4927 (class 1259 OID 18417)
-- Name: idx_orders_order_type; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_order_type ON public.orders USING btree (order_type);


--
-- TOC entry 4928 (class 1259 OID 18420)
-- Name: idx_orders_restaurant_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_restaurant_id ON public.orders USING btree (restaurant_id);


--
-- TOC entry 4929 (class 1259 OID 18418)
-- Name: idx_orders_table_no; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_orders_table_no ON public.orders USING btree (table_no) WHERE (table_no IS NOT NULL);


--
-- TOC entry 5126 (class 1259 OID 19555)
-- Name: idx_org_users_org_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_org_users_org_id ON public.organization_users USING btree (organization_id);


--
-- TOC entry 5127 (class 1259 OID 19554)
-- Name: idx_org_users_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_org_users_user_id ON public.organization_users USING btree (user_id);


--
-- TOC entry 5120 (class 1259 OID 19520)
-- Name: idx_organizations_slug; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_organizations_slug ON public.organizations USING btree (slug);


--
-- TOC entry 5121 (class 1259 OID 19521)
-- Name: idx_organizations_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_organizations_status ON public.organizations USING btree (status);


--
-- TOC entry 5139 (class 1259 OID 19606)
-- Name: idx_outlet_menu_overrides_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_outlet_menu_overrides_restaurant ON public.outlet_menu_overrides USING btree (restaurant_id);


--
-- TOC entry 5144 (class 1259 OID 19624)
-- Name: idx_outlet_settings_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_outlet_settings_restaurant ON public.outlet_settings_overrides USING btree (restaurant_id);


--
-- TOC entry 5132 (class 1259 OID 19581)
-- Name: idx_outlet_staff_org_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_outlet_staff_org_id ON public.outlet_staff_assignments USING btree (organization_id);


--
-- TOC entry 5133 (class 1259 OID 19580)
-- Name: idx_outlet_staff_restaurant_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_outlet_staff_restaurant_id ON public.outlet_staff_assignments USING btree (restaurant_id);


--
-- TOC entry 5134 (class 1259 OID 19579)
-- Name: idx_outlet_staff_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_outlet_staff_user_id ON public.outlet_staff_assignments USING btree (user_id);


--
-- TOC entry 4992 (class 1259 OID 18696)
-- Name: idx_owner_insights_category_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_owner_insights_category_time ON public.owner_insights USING btree (tenant_id, outlet_id, category, created_at DESC);


--
-- TOC entry 4993 (class 1259 OID 18695)
-- Name: idx_owner_insights_status_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_owner_insights_status_time ON public.owner_insights USING btree (tenant_id, outlet_id, status, created_at DESC);


--
-- TOC entry 5050 (class 1259 OID 18910)
-- Name: idx_owner_referrals_referrer_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_owner_referrals_referrer_time ON public.owner_referrals USING btree (referrer_owner_user_id, created_at DESC);


--
-- TOC entry 5051 (class 1259 OID 18911)
-- Name: idx_owner_referrals_status_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_owner_referrals_status_time ON public.owner_referrals USING btree (status, updated_at DESC);


--
-- TOC entry 5063 (class 1259 OID 18978)
-- Name: idx_payment_sessions_status_expires; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_payment_sessions_status_expires ON public.payment_sessions USING btree (status, expires_at) WHERE ((status)::text = 'PENDING'::text);


--
-- TOC entry 4981 (class 1259 OID 18652)
-- Name: idx_pos_events_ref; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pos_events_ref ON public.pos_events USING btree (tenant_id, ref_type, ref_id);


--
-- TOC entry 4982 (class 1259 OID 18651)
-- Name: idx_pos_events_tenant_outlet_event_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pos_events_tenant_outlet_event_time ON public.pos_events USING btree (tenant_id, outlet_id, event_type, event_time DESC);


--
-- TOC entry 4983 (class 1259 OID 18650)
-- Name: idx_pos_events_tenant_outlet_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_pos_events_tenant_outlet_time ON public.pos_events USING btree (tenant_id, outlet_id, event_time DESC);


--
-- TOC entry 5059 (class 1259 OID 18956)
-- Name: idx_qr_sessions_restaurant_status_expires; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_qr_sessions_restaurant_status_expires ON public.qr_sessions USING btree (restaurant_id, status, expires_at);


--
-- TOC entry 5060 (class 1259 OID 18957)
-- Name: idx_qr_sessions_token; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_qr_sessions_token ON public.qr_sessions USING btree (qr_token) WHERE (qr_token IS NOT NULL);


--
-- TOC entry 5113 (class 1259 OID 19486)
-- Name: idx_qr_tokens_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_qr_tokens_restaurant ON public.qr_tokens USING btree (restaurant_id) WHERE (active = true);


--
-- TOC entry 5007 (class 1259 OID 18760)
-- Name: idx_rating_links_tenant_outlet_created; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_rating_links_tenant_outlet_created ON public.rating_links USING btree (tenant_id, outlet_id, created_at DESC);


--
-- TOC entry 5176 (class 1259 OID 19786)
-- Name: idx_rec_events_customer; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_rec_events_customer ON public.recommendation_events USING btree (customer_id, created_at DESC) WHERE (customer_id IS NOT NULL);


--
-- TOC entry 5177 (class 1259 OID 19785)
-- Name: idx_rec_events_restaurant_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_rec_events_restaurant_time ON public.recommendation_events USING btree (restaurant_id, created_at DESC);


--
-- TOC entry 5116 (class 1259 OID 19500)
-- Name: idx_restaurant_sessions_expires_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_restaurant_sessions_expires_at ON public.restaurant_sessions USING btree (expires_at);


--
-- TOC entry 5117 (class 1259 OID 19499)
-- Name: idx_restaurant_sessions_restaurant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_restaurant_sessions_restaurant ON public.restaurant_sessions USING btree (restaurant_id);


--
-- TOC entry 4937 (class 1259 OID 18412)
-- Name: idx_restaurant_staff_restaurant_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_restaurant_staff_restaurant_id ON public.restaurant_staff USING btree (restaurant_id);


--
-- TOC entry 4938 (class 1259 OID 18411)
-- Name: idx_restaurant_staff_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_restaurant_staff_user_id ON public.restaurant_staff USING btree (user_id);


--
-- TOC entry 4908 (class 1259 OID 19534)
-- Name: idx_restaurants_org_outlet_code; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_restaurants_org_outlet_code ON public.restaurants USING btree (organization_id, outlet_code) WHERE ((organization_id IS NOT NULL) AND (outlet_code IS NOT NULL));


--
-- TOC entry 4909 (class 1259 OID 19535)
-- Name: idx_restaurants_organization_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_restaurants_organization_id ON public.restaurants USING btree (organization_id);


--
-- TOC entry 4999 (class 1259 OID 18723)
-- Name: idx_stock_alerts_lookup; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_stock_alerts_lookup ON public.stock_alerts USING btree (tenant_id, outlet_id, status, severity, created_at DESC);


--
-- TOC entry 4947 (class 1259 OID 18460)
-- Name: idx_unit_conversions_from; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_unit_conversions_from ON public.unit_conversions USING btree (from_unit_id) WHERE (is_active = true);


--
-- TOC entry 4948 (class 1259 OID 18461)
-- Name: idx_unit_conversions_to; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_unit_conversions_to ON public.unit_conversions USING btree (to_unit_id) WHERE (is_active = true);


--
-- TOC entry 5080 (class 1259 OID 19148)
-- Name: idx_user_addresses_is_default; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_addresses_is_default ON public.user_addresses USING btree (user_id, is_default);


--
-- TOC entry 5081 (class 1259 OID 19149)
-- Name: idx_user_addresses_location; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_addresses_location ON public.user_addresses USING btree (latitude, longitude);


--
-- TOC entry 5082 (class 1259 OID 19147)
-- Name: idx_user_addresses_user_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_addresses_user_id ON public.user_addresses USING btree (user_id);


--
-- TOC entry 5069 (class 1259 OID 18994)
-- Name: idx_user_payment_methods_default_per_user; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX idx_user_payment_methods_default_per_user ON public.user_payment_methods USING btree (user_id) WHERE (is_default = true);


--
-- TOC entry 5070 (class 1259 OID 18993)
-- Name: idx_user_payment_methods_user_updated; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_user_payment_methods_user_updated ON public.user_payment_methods USING btree (user_id, updated_at DESC, created_at DESC);


--
-- TOC entry 4899 (class 1259 OID 19125)
-- Name: idx_users_created_at; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_created_at ON public.users USING btree (created_at DESC);


--
-- TOC entry 4900 (class 1259 OID 19121)
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_email ON public.users USING btree (email) WHERE (deleted_at IS NULL);


--
-- TOC entry 4901 (class 1259 OID 19122)
-- Name: idx_users_phone; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_phone ON public.users USING btree (phone) WHERE (deleted_at IS NULL);


--
-- TOC entry 4902 (class 1259 OID 19127)
-- Name: idx_users_phone_role; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_phone_role ON public.users USING btree (phone, primary_role) WHERE (deleted_at IS NULL);


--
-- TOC entry 4903 (class 1259 OID 19123)
-- Name: idx_users_primary_role; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_primary_role ON public.users USING btree (primary_role) WHERE (deleted_at IS NULL);


--
-- TOC entry 4904 (class 1259 OID 19126)
-- Name: idx_users_search_vector; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_search_vector ON public.users USING gin (search_vector);


--
-- TOC entry 4905 (class 1259 OID 19124)
-- Name: idx_users_status; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_users_status ON public.users USING btree (status) WHERE (deleted_at IS NULL);


--
-- TOC entry 5085 (class 1259 OID 19200)
-- Name: idx_variants_available; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_variants_available ON public.menu_item_variants USING btree (is_available);


--
-- TOC entry 5086 (class 1259 OID 19201)
-- Name: idx_variants_default; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_variants_default ON public.menu_item_variants USING btree (menu_item_id, is_default);


--
-- TOC entry 5087 (class 1259 OID 19199)
-- Name: idx_variants_item_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_variants_item_id ON public.menu_item_variants USING btree (menu_item_id);


--
-- TOC entry 4953 (class 1259 OID 18476)
-- Name: idx_vendors_name; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_vendors_name ON public.vendors USING btree (name) WHERE (is_active = true);


--
-- TOC entry 4954 (class 1259 OID 18475)
-- Name: idx_vendors_tenant; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_vendors_tenant ON public.vendors USING btree (tenant_id) WHERE (is_active = true);


--
-- TOC entry 5041 (class 1259 OID 18879)
-- Name: idx_wallet_ledger_customer_time; Type: INDEX; Schema: public; Owner: postgres
--

CREATE INDEX idx_wallet_ledger_customer_time ON public.wallet_ledger USING btree (tenant_id, outlet_id, customer_id, created_at DESC);


--
-- TOC entry 5066 (class 1259 OID 18976)
-- Name: uidx_payment_sessions_order_pending; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX uidx_payment_sessions_order_pending ON public.payment_sessions USING btree (order_id) WHERE ((status)::text = 'PENDING'::text);


--
-- TOC entry 5067 (class 1259 OID 18977)
-- Name: uidx_payment_sessions_psp_txn_id; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX uidx_payment_sessions_psp_txn_id ON public.payment_sessions USING btree (psp_txn_id) WHERE (psp_txn_id IS NOT NULL);


--
-- TOC entry 5068 (class 1259 OID 18975)
-- Name: uidx_payment_sessions_token_hash; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX uidx_payment_sessions_token_hash ON public.payment_sessions USING btree (token_hash);


--
-- TOC entry 4980 (class 1259 OID 18604)
-- Name: uq_inventory_idempotency; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX uq_inventory_idempotency ON public.inventory_idempotency_keys USING btree (tenant_id, endpoint, idempotency_key);


--
-- TOC entry 4998 (class 1259 OID 18705)
-- Name: uq_notification_devices_token; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX uq_notification_devices_token ON public.notification_devices USING btree (tenant_id, user_id, push_token);


--
-- TOC entry 5054 (class 1259 OID 18912)
-- Name: uq_owner_referrals_code_phone; Type: INDEX; Schema: public; Owner: postgres
--

CREATE UNIQUE INDEX uq_owner_referrals_code_phone ON public.owner_referrals USING btree (referral_code, referred_phone) WHERE (referred_phone IS NOT NULL);


--
-- TOC entry 5261 (class 2620 OID 19827)
-- Name: customer_devices trg_customer_devices_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_customer_devices_updated_at BEFORE UPDATE ON public.customer_devices FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5260 (class 2620 OID 19826)
-- Name: customers trg_customers_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_customers_updated_at BEFORE UPDATE ON public.customers FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5256 (class 2620 OID 19626)
-- Name: organization_users trg_organization_users_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_organization_users_updated_at BEFORE UPDATE ON public.organization_users FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5255 (class 2620 OID 19625)
-- Name: organizations trg_organizations_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_organizations_updated_at BEFORE UPDATE ON public.organizations FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5258 (class 2620 OID 19628)
-- Name: outlet_menu_overrides trg_outlet_menu_overrides_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_outlet_menu_overrides_updated_at BEFORE UPDATE ON public.outlet_menu_overrides FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5259 (class 2620 OID 19629)
-- Name: outlet_settings_overrides trg_outlet_settings_overrides_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_outlet_settings_overrides_updated_at BEFORE UPDATE ON public.outlet_settings_overrides FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5257 (class 2620 OID 19627)
-- Name: outlet_staff_assignments trg_outlet_staff_assignments_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER trg_outlet_staff_assignments_updated_at BEFORE UPDATE ON public.outlet_staff_assignments FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5253 (class 2620 OID 19335)
-- Name: ingredients update_ingredients_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER update_ingredients_updated_at BEFORE UPDATE ON public.ingredients FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5251 (class 2620 OID 19333)
-- Name: units update_units_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER update_units_updated_at BEFORE UPDATE ON public.units FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5254 (class 2620 OID 19462)
-- Name: user_addresses update_user_addresses_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER update_user_addresses_updated_at BEFORE UPDATE ON public.user_addresses FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5249 (class 2620 OID 19448)
-- Name: users update_users_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5252 (class 2620 OID 19334)
-- Name: vendors update_vendors_updated_at; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER update_vendors_updated_at BEFORE UPDATE ON public.vendors FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- TOC entry 5250 (class 2620 OID 19449)
-- Name: users users_search_vector_trigger; Type: TRIGGER; Schema: public; Owner: postgres
--

CREATE TRIGGER users_search_vector_trigger BEFORE INSERT OR UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.users_search_vector_update();


--
-- TOC entry 5194 (class 2606 OID 18383)
-- Name: bank_verifications bank_verifications_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.bank_verifications
    ADD CONSTRAINT bank_verifications_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5237 (class 2606 OID 19674)
-- Name: customer_devices customer_devices_customer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_devices
    ADD CONSTRAINT customer_devices_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id) ON DELETE CASCADE;


--
-- TOC entry 5238 (class 2606 OID 19679)
-- Name: customer_devices customer_devices_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_devices
    ADD CONSTRAINT customer_devices_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5241 (class 2606 OID 19719)
-- Name: customer_favorite_items customer_favorite_items_customer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_favorite_items
    ADD CONSTRAINT customer_favorite_items_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id) ON DELETE CASCADE;


--
-- TOC entry 5242 (class 2606 OID 19724)
-- Name: customer_favorite_items customer_favorite_items_menu_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_favorite_items
    ADD CONSTRAINT customer_favorite_items_menu_item_id_fkey FOREIGN KEY (menu_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5239 (class 2606 OID 19697)
-- Name: customer_visits customer_visits_customer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_visits
    ADD CONSTRAINT customer_visits_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id) ON DELETE CASCADE;


--
-- TOC entry 5240 (class 2606 OID 19702)
-- Name: customer_visits customer_visits_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customer_visits
    ADD CONSTRAINT customer_visits_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5236 (class 2606 OID 19652)
-- Name: customers customers_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5215 (class 2606 OID 19023)
-- Name: foodshare_participants foodshare_participants_session_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.foodshare_participants
    ADD CONSTRAINT foodshare_participants_session_id_fkey FOREIGN KEY (session_id) REFERENCES public.foodshare_sessions(session_id) ON DELETE CASCADE;


--
-- TOC entry 5214 (class 2606 OID 19008)
-- Name: foodshare_sessions foodshare_sessions_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.foodshare_sessions
    ADD CONSTRAINT foodshare_sessions_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5223 (class 2606 OID 19283)
-- Name: incentive_cycles incentive_cycles_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.incentive_cycles
    ADD CONSTRAINT incentive_cycles_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5224 (class 2606 OID 19298)
-- Name: incentive_milestones incentive_milestones_cycle_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.incentive_milestones
    ADD CONSTRAINT incentive_milestones_cycle_id_fkey FOREIGN KEY (cycle_id) REFERENCES public.incentive_cycles(id) ON DELETE CASCADE;


--
-- TOC entry 5199 (class 2606 OID 18501)
-- Name: ingredients ingredients_base_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ingredients
    ADD CONSTRAINT ingredients_base_unit_id_fkey FOREIGN KEY (base_unit_id) REFERENCES public.units(unit_id);


--
-- TOC entry 5200 (class 2606 OID 18506)
-- Name: ingredients ingredients_preferred_vendor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ingredients
    ADD CONSTRAINT ingredients_preferred_vendor_id_fkey FOREIGN KEY (preferred_vendor_id) REFERENCES public.vendors(vendor_id) ON DELETE SET NULL;


--
-- TOC entry 5201 (class 2606 OID 18496)
-- Name: ingredients ingredients_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.ingredients
    ADD CONSTRAINT ingredients_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5203 (class 2606 OID 18571)
-- Name: inventory_recipe_lines inventory_recipe_lines_ingredient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipe_lines
    ADD CONSTRAINT inventory_recipe_lines_ingredient_id_fkey FOREIGN KEY (ingredient_id) REFERENCES public.ingredients(ingredient_id) ON DELETE RESTRICT;


--
-- TOC entry 5204 (class 2606 OID 18566)
-- Name: inventory_recipe_lines inventory_recipe_lines_recipe_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipe_lines
    ADD CONSTRAINT inventory_recipe_lines_recipe_id_fkey FOREIGN KEY (recipe_id) REFERENCES public.inventory_recipes(recipe_id) ON DELETE CASCADE;


--
-- TOC entry 5202 (class 2606 OID 18549)
-- Name: inventory_recipes inventory_recipes_menu_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_recipes
    ADD CONSTRAINT inventory_recipes_menu_item_id_fkey FOREIGN KEY (menu_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5205 (class 2606 OID 18587)
-- Name: inventory_stock_ledger inventory_stock_ledger_ingredient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_stock_ledger
    ADD CONSTRAINT inventory_stock_ledger_ingredient_id_fkey FOREIGN KEY (ingredient_id) REFERENCES public.ingredients(ingredient_id) ON DELETE RESTRICT;


--
-- TOC entry 5207 (class 2606 OID 18733)
-- Name: inventory_stock_snapshots inventory_stock_snapshots_ingredient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.inventory_stock_snapshots
    ADD CONSTRAINT inventory_stock_snapshots_ingredient_id_fkey FOREIGN KEY (ingredient_id) REFERENCES public.ingredients(ingredient_id) ON DELETE CASCADE;


--
-- TOC entry 5243 (class 2606 OID 19748)
-- Name: item_pairings item_pairings_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.item_pairings
    ADD CONSTRAINT item_pairings_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5244 (class 2606 OID 19753)
-- Name: item_pairings item_pairings_source_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.item_pairings
    ADD CONSTRAINT item_pairings_source_item_id_fkey FOREIGN KEY (source_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5245 (class 2606 OID 19758)
-- Name: item_pairings item_pairings_target_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.item_pairings
    ADD CONSTRAINT item_pairings_target_item_id_fkey FOREIGN KEY (target_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5188 (class 2606 OID 18291)
-- Name: menu_categories menu_categories_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_categories
    ADD CONSTRAINT menu_categories_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5217 (class 2606 OID 19219)
-- Name: menu_item_addons menu_item_addons_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_addons
    ADD CONSTRAINT menu_item_addons_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5218 (class 2606 OID 19224)
-- Name: menu_item_addons menu_item_addons_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_addons
    ADD CONSTRAINT menu_item_addons_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5225 (class 2606 OID 19315)
-- Name: menu_item_combos menu_item_combos_combo_menu_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_combos
    ADD CONSTRAINT menu_item_combos_combo_menu_item_id_fkey FOREIGN KEY (combo_menu_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5226 (class 2606 OID 19320)
-- Name: menu_item_combos menu_item_combos_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_combos
    ADD CONSTRAINT menu_item_combos_item_id_fkey FOREIGN KEY (item_id) REFERENCES public.menu_items(item_id) ON DELETE RESTRICT;


--
-- TOC entry 5227 (class 2606 OID 19325)
-- Name: menu_item_combos menu_item_combos_variant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_combos
    ADD CONSTRAINT menu_item_combos_variant_id_fkey FOREIGN KEY (variant_id) REFERENCES public.menu_item_variants(variant_id) ON DELETE SET NULL;


--
-- TOC entry 5216 (class 2606 OID 19194)
-- Name: menu_item_variants menu_item_variants_menu_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_item_variants
    ADD CONSTRAINT menu_item_variants_menu_item_id_fkey FOREIGN KEY (menu_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5189 (class 2606 OID 18318)
-- Name: menu_items menu_items_category_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_items
    ADD CONSTRAINT menu_items_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.menu_categories(category_id) ON DELETE SET NULL;


--
-- TOC entry 5190 (class 2606 OID 18313)
-- Name: menu_items menu_items_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.menu_items
    ADD CONSTRAINT menu_items_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5209 (class 2606 OID 18843)
-- Name: offer_benefits offer_benefits_offer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_benefits
    ADD CONSTRAINT offer_benefits_offer_id_fkey FOREIGN KEY (offer_id) REFERENCES public.offers(id) ON DELETE CASCADE;


--
-- TOC entry 5208 (class 2606 OID 18830)
-- Name: offer_rules offer_rules_offer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_rules
    ADD CONSTRAINT offer_rules_offer_id_fkey FOREIGN KEY (offer_id) REFERENCES public.offers(id) ON DELETE CASCADE;


--
-- TOC entry 5247 (class 2606 OID 19820)
-- Name: offer_suggested_items offer_suggested_items_menu_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_suggested_items
    ADD CONSTRAINT offer_suggested_items_menu_item_id_fkey FOREIGN KEY (menu_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5248 (class 2606 OID 19815)
-- Name: offer_suggested_items offer_suggested_items_offer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.offer_suggested_items
    ADD CONSTRAINT offer_suggested_items_offer_id_fkey FOREIGN KEY (offer_id) REFERENCES public.offers(id) ON DELETE CASCADE;


--
-- TOC entry 5221 (class 2606 OID 19266)
-- Name: order_item_addons order_item_addons_addon_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_addons
    ADD CONSTRAINT order_item_addons_addon_id_fkey FOREIGN KEY (addon_id) REFERENCES public.menu_item_addons(addon_id) ON DELETE SET NULL;


--
-- TOC entry 5222 (class 2606 OID 19261)
-- Name: order_item_addons order_item_addons_order_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_addons
    ADD CONSTRAINT order_item_addons_order_item_id_fkey FOREIGN KEY (order_item_id) REFERENCES public.order_items(order_item_id) ON DELETE CASCADE;


--
-- TOC entry 5219 (class 2606 OID 19241)
-- Name: order_item_variants order_item_variants_order_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_variants
    ADD CONSTRAINT order_item_variants_order_item_id_fkey FOREIGN KEY (order_item_id) REFERENCES public.order_items(order_item_id) ON DELETE CASCADE;


--
-- TOC entry 5220 (class 2606 OID 19246)
-- Name: order_item_variants order_item_variants_variant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_item_variants
    ADD CONSTRAINT order_item_variants_variant_id_fkey FOREIGN KEY (variant_id) REFERENCES public.menu_item_variants(variant_id) ON DELETE SET NULL;


--
-- TOC entry 5192 (class 2606 OID 18364)
-- Name: order_items order_items_menu_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT order_items_menu_item_id_fkey FOREIGN KEY (menu_item_id) REFERENCES public.menu_items(item_id) ON DELETE SET NULL;


--
-- TOC entry 5193 (class 2606 OID 18359)
-- Name: order_items order_items_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.order_items
    ADD CONSTRAINT order_items_order_id_fkey FOREIGN KEY (order_id) REFERENCES public.orders(order_id) ON DELETE CASCADE;


--
-- TOC entry 5191 (class 2606 OID 18342)
-- Name: orders orders_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5230 (class 2606 OID 19549)
-- Name: organization_users organization_users_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.organization_users
    ADD CONSTRAINT organization_users_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- TOC entry 5233 (class 2606 OID 19601)
-- Name: outlet_menu_overrides outlet_menu_overrides_menu_item_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_menu_overrides
    ADD CONSTRAINT outlet_menu_overrides_menu_item_id_fkey FOREIGN KEY (menu_item_id) REFERENCES public.menu_items(item_id) ON DELETE CASCADE;


--
-- TOC entry 5234 (class 2606 OID 19596)
-- Name: outlet_menu_overrides outlet_menu_overrides_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_menu_overrides
    ADD CONSTRAINT outlet_menu_overrides_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5235 (class 2606 OID 19619)
-- Name: outlet_settings_overrides outlet_settings_overrides_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_settings_overrides
    ADD CONSTRAINT outlet_settings_overrides_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5231 (class 2606 OID 19569)
-- Name: outlet_staff_assignments outlet_staff_assignments_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_staff_assignments
    ADD CONSTRAINT outlet_staff_assignments_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE CASCADE;


--
-- TOC entry 5232 (class 2606 OID 19574)
-- Name: outlet_staff_assignments outlet_staff_assignments_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.outlet_staff_assignments
    ADD CONSTRAINT outlet_staff_assignments_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5213 (class 2606 OID 18970)
-- Name: payment_sessions payment_sessions_order_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.payment_sessions
    ADD CONSTRAINT payment_sessions_order_id_fkey FOREIGN KEY (order_id) REFERENCES public.orders(order_id) ON DELETE CASCADE;


--
-- TOC entry 5212 (class 2606 OID 18951)
-- Name: qr_sessions qr_sessions_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.qr_sessions
    ADD CONSTRAINT qr_sessions_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5228 (class 2606 OID 19481)
-- Name: qr_tokens qr_tokens_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.qr_tokens
    ADD CONSTRAINT qr_tokens_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5246 (class 2606 OID 19780)
-- Name: recommendation_events recommendation_events_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.recommendation_events
    ADD CONSTRAINT recommendation_events_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5211 (class 2606 OID 18935)
-- Name: referral_attribution_events referral_attribution_events_referral_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.referral_attribution_events
    ADD CONSTRAINT referral_attribution_events_referral_id_fkey FOREIGN KEY (referral_id) REFERENCES public.owner_referrals(id) ON DELETE CASCADE;


--
-- TOC entry 5210 (class 2606 OID 18921)
-- Name: referral_rewards referral_rewards_referral_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.referral_rewards
    ADD CONSTRAINT referral_rewards_referral_id_fkey FOREIGN KEY (referral_id) REFERENCES public.owner_referrals(id) ON DELETE CASCADE;


--
-- TOC entry 5186 (class 2606 OID 18258)
-- Name: restaurant_hours restaurant_hours_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_hours
    ADD CONSTRAINT restaurant_hours_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5229 (class 2606 OID 19494)
-- Name: restaurant_sessions restaurant_sessions_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_sessions
    ADD CONSTRAINT restaurant_sessions_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5195 (class 2606 OID 18401)
-- Name: restaurant_staff restaurant_staff_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_staff
    ADD CONSTRAINT restaurant_staff_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5196 (class 2606 OID 18406)
-- Name: restaurant_staff restaurant_staff_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_staff
    ADD CONSTRAINT restaurant_staff_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE RESTRICT;


--
-- TOC entry 5187 (class 2606 OID 18274)
-- Name: restaurant_tables restaurant_tables_restaurant_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurant_tables
    ADD CONSTRAINT restaurant_tables_restaurant_id_fkey FOREIGN KEY (restaurant_id) REFERENCES public.restaurants(restaurant_id) ON DELETE CASCADE;


--
-- TOC entry 5185 (class 2606 OID 19522)
-- Name: restaurants restaurants_organization_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.restaurants
    ADD CONSTRAINT restaurants_organization_id_fkey FOREIGN KEY (organization_id) REFERENCES public.organizations(id) ON DELETE SET NULL;


--
-- TOC entry 5183 (class 2606 OID 17925)
-- Name: role_permissions role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES public.permissions(id) ON DELETE CASCADE;


--
-- TOC entry 5184 (class 2606 OID 17920)
-- Name: role_permissions role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES public.roles(id) ON DELETE CASCADE;


--
-- TOC entry 5206 (class 2606 OID 18718)
-- Name: stock_alerts stock_alerts_ingredient_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.stock_alerts
    ADD CONSTRAINT stock_alerts_ingredient_id_fkey FOREIGN KEY (ingredient_id) REFERENCES public.ingredients(ingredient_id) ON DELETE CASCADE;


--
-- TOC entry 5197 (class 2606 OID 18450)
-- Name: unit_conversions unit_conversions_from_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit_conversions
    ADD CONSTRAINT unit_conversions_from_unit_id_fkey FOREIGN KEY (from_unit_id) REFERENCES public.units(unit_id) ON DELETE CASCADE;


--
-- TOC entry 5198 (class 2606 OID 18455)
-- Name: unit_conversions unit_conversions_to_unit_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: postgres
--

ALTER TABLE ONLY public.unit_conversions
    ADD CONSTRAINT unit_conversions_to_unit_id_fkey FOREIGN KEY (to_unit_id) REFERENCES public.units(unit_id) ON DELETE CASCADE;


-- Completed on 2026-03-20 23:05:04

--
-- PostgreSQL database dump complete
--

