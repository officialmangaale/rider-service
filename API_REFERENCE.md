# 🍽️ Restaurant Service — Complete API Reference

> **Base URL:** `https://api.mangaale.com` (production) | `http://localhost:8080` (development)
>
> **Auth:** Most endpoints require a JWT Bearer token via `Authorization: Bearer <token>` header.
> Permissions are enforced via RBAC middleware (e.g. `manage_restaurant`, `manage_orders`, `view_dashboard`, etc.)

---

## Table of Contents

1. [Public / Unauthenticated](#1-public--unauthenticated)
2. [QRunch Session (Cookie-based)](#2-qrunch-session-cookie-based)
3. [Restaurant Management](#3-restaurant-management)
4. [Menu Management](#4-menu-management)
5. [Order Management](#5-order-management)
6. [Payment](#6-payment)
7. [Analytics & Dashboard](#7-analytics--dashboard)
8. [Finance & GST Reports](#8-finance--gst-reports)
9. [Staff Management](#9-staff-management)
10. [RBAC (Role-Based Access Control)](#10-rbac-role-based-access-control)
11. [Notifications](#11-notifications)
12. [Kitchen Display System (KDS)](#12-kitchen-display-system-kds)
13. [Owner Intelligence & Copilot](#13-owner-intelligence--copilot)
14. [Incentives](#14-incentives)
15. [Ratings & Reviews](#15-ratings--reviews)
16. [Offers & Smart Discounts](#16-offers--smart-discounts)
17. [Customer Identity System](#17-customer-identity-system)
18. [Smart Recommendations](#18-smart-recommendations)
19. [Referrals](#19-referrals)
20. [Inventory & Recipes](#20-inventory--recipes)
21. [Multi-Outlet Management](#21-multi-outlet-management)
22. [Wallet](#22-wallet)
23. [Consumer App (Discovery)](#23-consumer-app-discovery)
24. [Consumer Orders (Online)](#24-consumer-orders-online)
25. [FoodShare](#25-foodshare)
26. [Demo Requests](#26-demo-requests)

---

## 1. Public / Unauthenticated

### List All Restaurants
```
GET /restaurants
```
| Query Param | Type | Description |
|---|---|---|
| `q` | string | Search keyword |
| `city` | string | Filter by city |
| `lat` | float | Latitude for geo-search |
| `lon` | float | Longitude for geo-search |
| `radius` | float | Search radius in km |
| `tags` | string | Comma-separated tags |
| `page` | int | Page number (default: 1) |
| `limit` | int | Items per page (default: 20) |

**Response (200):**
```json
{ "success": true, "message": "restaurants fetched", "data": { "items": [...], "meta": { "total": 50, "page": 1, "limit": 20 } } }
```

---

### Get Restaurant (Public)
```
GET /restaurants/public/:id
```
**Response (200):**
```json
{ "success": true, "data": { "restaurant": { "id": 1, "name": "...", ... } } }
```

---

### Get Nearby Restaurants
```
GET /api/restaurants/nearby
```
| Query Param | Type | Description |
|---|---|---|
| `lat` | float | **Required.** Latitude |
| `lng` | float | **Required.** Longitude |
| `radius` | float | Radius in km |
| `page` | int | Page number |
| `limit` | int | Items per page |

**Response (200):**
```json
{ "success": true, "data": [{ "id": 1, "name": "...", "cuisine": "...", "rating": 4.5, "distance": "1.2 km", ... }] }
```

---

### Get Online Menu
```
GET /api/restaurants/:restaurant_id/menu/online
```
**Response (200):**
```json
{ "success": true, "data": { "restaurant": { "id": "rest_1", "name": "...", "currency": "INR" }, "categories": [{ "id": 1, "name": "Starters", "items": [...] }] } }
```

---

### Get Menu Items (Public)
```
GET /restaurants/:id/menu/items
```
| Query Param | Type | Description |
|---|---|---|
| `category_id` | int | Filter by category |
| `is_available` | bool | Availability filter |
| `is_vegetarian` | bool | Vegetarian filter |
| `is_vegan` | bool | Vegan filter |
| `is_gluten_free` | bool | Gluten-free filter |
| `is_qrunch` | bool | QRunch items filter |

**Response (200):** Array of menu items with variants, addons, and combo items.

---

### Rating Page (HTML)
```
GET /r/:token
```
Returns an HTML rating submission page. Rate-limited.

---

### Submit Rating (Public)
```
POST /api/public/ratings
```
Rate-limited. Body depends on rating form fields.

---

### Track Rating Redirect
```
POST /api/public/ratings/redirect
```
Tracks Google Maps redirect clicks from rating flow. Rate-limited.

---

### Payment Session Resolve
```
GET /payment-session/:token
```
Resolves a payment session token and returns payment details.

---

### Payment Webhook
```
POST /payments/webhook
```
Webhook endpoint for payment gateway callbacks.

---

### Referral Landing Page
```
GET /ref/:code
```
Public landing page for referral codes.

---

### Customer Order Live (SSE)
```
GET /orders/:id/live
```
**No auth required.** Server-Sent Events stream for real-time order status updates for QRunch customers.

---

## 2. QRunch Session (Cookie-based)

### Start Session
```
POST /session/start
POST /api/session/start
```
**Body:**
```json
{ "token": "qr-token-string", "name": "Customer Name", "table": "T5" }
```
**Response (200):** Sets `restaurant_session` cookie.

---

### Switch Session
```
POST /session/switch
POST /api/session/switch
```
**Body:**
```json
{ "token": "new-qr-token", "confirm": true }
```

---

### Clear Session
```
POST /session/clear
POST /api/session/clear
```
Clears the active session cookie.

---

### Get Current Menu (Session Required)
```
GET /menu/current
GET /api/menu/current
```
**Requires:** Valid `restaurant_session` cookie.

**Response (200):**
```json
{ "restaurant_id": 1, "restaurant_name": "...", "items": [...], "menu": [...] }
```

---

### Start QR Session (Order Flow)
```
POST /qr/session/start
```
**Body:**
```json
{ "restaurant_id": 1, "table_no": 5, "qr_token": "optional-token" }
```
**Response (201):**
```json
{ "data": { "session_id": 123, "restaurant_id": 1, "table_no": 5, "status": "active", "started_at": "...", "expires_at": "..." } }
```

---

## 3. Restaurant Management

> **Auth:** JWT required. **Permission:** `manage_restaurant`

### Create Restaurant
```
POST /restaurants/
```
**Content-Type:** `multipart/form-data`

| Form Field | Type | Description |
|---|---|---|
| `payload` | JSON string | Restaurant details |
| `logo` | file | Logo image |
| `background` | file | Background image |
| `gst_certificate` | file | GST certificate |
| `fssai_license` | file | FSSAI license |
| `aadhaar` | file | Aadhaar document |
| `pan` | file | PAN document |

**Response (201):** `{ "data": { "restaurantId": 1 } }`

---

### Get My Restaurant
```
GET /restaurants/byid
```
Returns restaurant associated with the authenticated user.

**Response (200):** `{ "data": { "restaurant": {...} } }`

---

### Get Restaurant Detail
```
GET /restaurants/detail/:id
```
**Response (200):** `{ "data": { "restaurant": {...} } }`

---

### Update Restaurant (Full)
```
PUT /restaurants/:id
```
**Body:** Full `Restaurant` JSON object.

---

### Patch Restaurant (Partial)
```
PATCH /restaurants/:id
```
**Content-Type:** `application/json` or `multipart/form-data`

**Body (JSON):** `{ "name": "New Name", "is_qrunch_purchased": true, ... }`

Supports file uploads via multipart: `logo`, `background`, etc.

---

### Delete Restaurant
```
DELETE /restaurants/:id
```
**Response (200):** `{ "message": "restaurant deleted" }`

---

### Generate QR Code Image
```
GET /restaurants/:id/qr?table_number=T5
```
**Response:** QR code image (`image/png`).

---

### Generate QR Session Token
```
POST /restaurants/:id/qr-token
```
**Body:** `{ "table_number": "T5" }`

**Response (201):**
```json
{ "data": { "restaurant_id": 1, "table_number": "T5", "token": "...", "qr_url": "...", "qr_image_base64": "...", "qr_image_dataurl": "data:image/png;base64,..." } }
```

---

### Operating Hours

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/restaurants/:id/hours` | Create hour |
| `GET` | `/restaurants/:id/hours` | List hours |
| `PUT` | `/restaurants/:id/hours/:hours_id` | Update hour |
| `DELETE` | `/restaurants/:id/hours/:hours_id` | Delete hour |

---

### Tables

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/restaurants/:id/tables` | Create table |
| `GET` | `/restaurants/:id/tables` | List tables |
| `PUT` | `/restaurants/:id/tables/:table_id` | Update table |
| `DELETE` | `/restaurants/:id/tables/:table_id` | Delete table |

---

### Cancel OTP
```
GET /restaurants/:id/cancel-otp
```
Returns the current cancel OTP for the restaurant.

**Response (200):** `{ "data": { "cancel_otp": "1234" } }`

---

## 4. Menu Management

> **Auth:** JWT required. **Permission:** `manage_restaurant`

### Categories

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/restaurants/:id/categories` | Create category |
| `GET` | `/restaurants/:id/categories` | List categories (auth-relaxed) |
| `GET` | `/restaurants/categories` | List all categories globally |

**Create Category Body:**
```json
{ "name": "Starters", "is_active": true }
```

---

### Menu Items

#### Create Menu Item
```
POST /restaurants/:id/menu/items
```
**Content-Type:** `application/json` or `multipart/form-data`

**Body (JSON):**
```json
{
  "name": "Butter Chicken",
  "description": "Creamy tomato-based curry",
  "price": 350.00,
  "category_id": 1,
  "is_vegetarian": false,
  "is_available": true,
  "is_taxable": true,
  "is_qrunch": true,
  "is_combo": false,
  "variants": [{ "variant_name": "Half", "price": 200 }, { "variant_name": "Full", "price": 350 }],
  "addons": [{ "name": "Extra Butter", "price": 30 }],
  "combo_items": []
}
```
**Multipart:** Use `data` field for JSON, `image` field for file.

**Response (201):** `{ "data": { "itemId": 42 } }`

---

#### Patch Menu Item
```
PATCH /restaurants/:id/menu/items/:item_id
```
**Body:** Partial update map. Supports `variants`, `addons`, `combo_items` as nested arrays.

---

#### Delete Menu Items (Bulk)
```
DELETE /restaurants/:id/menu/items
```
**Body:** `{ "menu_item_ids": [1, 2, 3] }`

**Response (200):** `{ "data": { "deleted_count": 3 } }`

---

### Variants

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/restaurants/:id/menu/items/:item_id/variants` | Add variant |
| `GET` | `/restaurants/:id/menu/items/:item_id/variants` | List variants |
| `PUT` | `/restaurants/:id/menu/items/:item_id/variants/:variant_id` | Update variant |
| `DELETE` | `/restaurants/:id/menu/items/:item_id/variants/:variant_id` | Delete variant |

---

### Addons

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/restaurants/:id/menu/items/:item_id/addons` | Add addon |
| `GET` | `/restaurants/:id/menu/items/:item_id/addons` | List addons |
| `PUT` | `/restaurants/:id/menu/items/:item_id/addons/:addon_id` | Update addon |
| `DELETE` | `/restaurants/:id/menu/items/:item_id/addons/:addon_id` | Delete addon |

---

## 5. Order Management

### Place Order
```
POST /orders
```
> **Auth:** JWT required.

**Body:**
```json
{
  "restaurantId": 1,
  "items": [{
    "menuItemId": 42,
    "name": "Butter Chicken",
    "qty": 2,
    "unitPrice": 350,
    "totalPrice": 700,
    "is_taxable": true,
    "variants": [],
    "addons": [],
    "is_combo": false,
    "combo_items": []
  }],
  "subtotal": 700,
  "tax_amount": 35,
  "deliveryFee": 0,
  "tipAmount": 0,
  "discountAmount": 14,
  "totalAmount": 721,
  "discountBreakdown": {
    "offer_name": "Welcome Offer",
    "offer_discount": 10,
    "standard_discount": 14,
    "total_savings": 24
  },
  "orderType": "DINE_IN",
  "tableNo": 5,
  "pay_by": "upi",
  "isQrunch": true,
  "diningSessionId": 123,
  "specialInstructions": "No onions",
  "cgst": 17.50,
  "sgst": 17.50,
  "customerId": "uuid-string"
}
```
**Response (201):**
```json
{ "data": { "orderId": 1001, "isQrunch": true, "items": [...], "createdAt": "..." } }
```

---

### Get All Orders (Restaurant)
```
GET /restaurants/orders
```
> **Auth:** JWT required. Admins see all orders; others see their restaurant's only.

| Query Param | Type | Description |
|---|---|---|
| `status` | string | Filter: `all`, `pending`, `completed`, `cancelled` |
| `is_qrunch` | bool | Filter QRunch orders |
| `from_date` | string | Start date (`YYYY-MM-DD` or RFC3339) |
| `to_date` | string | End date |
| `page` | int | Page (default: 1) |
| `limit` | int | Limit (default: 20, max: 100) |

**Response (200):**
```json
{ "data": { "orders": [...], "pagination": { "page": 1, "limit": 20, "total": 150, "total_pages": 8 } } }
```

---

### Get Active Orders
```
GET /restaurants/orders/active
```
Returns currently active (non-completed, non-cancelled) orders.

---

### Get Order Status
```
GET /orders/:id/status
```
> **Permission:** `manage_orders`

**Response (200):** `{ "data": { "orderId": 1001, "status": "preparing" } }`

---

### Update Order Status
```
PUT /orders/:id/status
```
> **Permission:** `manage_orders`

**Body:**
```json
{ "status": "preparing", "cancel_otp": "1234" }
```
`cancel_otp` is required only when cancelling.

---

### Update Order (Partial)
```
PATCH /orders/:id
```
> **Permission:** `manage_orders`

**Body:** Partial update map. For payment completion, send `payment_status`, `pay_by`, and `order_status` together:
```json
{ "payment_status": "paid", "pay_by": "upi", "order_status": "completed" }
```

---

### Add Items to Order
```
PATCH /orders/:id/items
```
> **Permission:** `manage_orders`

**Body:**
```json
{
  "items": [{ "menuItemId": 42, "name": "Paneer Tikka", "qty": 1, "unitPrice": 250, "is_taxable": true }],
  "tipAmount": 20,
  "discountAmount": 5,
  "createPaymentSession": true,
  "discountBreakdown": { ... }
}
```
**Response (200):** Returns updated `order`, all `items`, `payment_session`, and `payment_reopened` flag.

---

### Update Item Quantity
```
PATCH /orders/:id/items/:item_id/quantity
```
> **Permission:** `manage_orders`

**Body:** `{ "quantity": 3, "tipAmount": 20, "discountAmount": 5 }`

---

### Delete Order Item
```
DELETE /orders/:id/items/:item_id
```
> **Permission:** `manage_orders`

**Optional Body:**
```json
{ "tip_amount": 0, "discount_amount": 0, "delivery_fee": 0 }
```

---

### Admin: Get All Orders
```
GET /admin/orders
```
> **Permission:** `orders_admin`

| Query Param | Type | Description |
|---|---|---|
| `restaurant_id` | int | Filter by restaurant |
| `status` | string | Order status filter |
| `is_qrunch` | bool | QRunch filter |
| `from_date` | string | Start date |
| `to_date` | string | End date |
| `page` | int | Page |
| `limit` | int | Limit |

---

## 6. Payment

### Generate Payment QR
```
POST /orders/:id/payment/qr
```
> **Auth:** JWT required.

**Response (200):**
```json
{ "data": { "order_id": 1001, "amount": 721, "upi_uri": "upi://pay?...", "payment_status": "pending" } }
```

---

### Confirm Payment
```
POST /orders/:id/payment/confirm
```
> **Auth:** JWT required.

**Body:** `{ "payment_status": "paid" }`

---

### Create Payment Session
```
POST /orders/:id/payment-session
```
> **Permission:** `manage_orders`

---

## 7. Analytics & Dashboard

> **Auth:** JWT required. **Permission:** `view_dashboard`

### Dashboard Overview
```
GET /restaurants/analytics/dashboard
```
| Query Param | Type | Description |
|---|---|---|
| `from` | string | Start date (`YYYY-MM-DD`). Default: 30 days ago |
| `to` | string | End date. Default: today |
| `period` | string | `day`, `week`, `month` (default: `day`) |

---

### Orders Analytics
```
GET /restaurants/analytics/orders
```
Same query params as dashboard.

---

### Order Details (Analytics)
```
GET /restaurants/analytics/orders/:order_id
```

---

### Order PDF
```
GET /restaurants/analytics/orders/:order_id/pdf
```
Returns `application/pdf` download.

---

### Order Receipt
```
GET /restaurants/analytics/orders/:order_id/receipt
```
| Query Param | Type | Description |
|---|---|---|
| `width` | int | Thermal paper width in chars (default: 32) |
| `format` | string | `json` (default), `text`, or `html` |

**Response (200 for JSON):**
```json
{
  "data": {
    "order_id": 1001,
    "restaurant": "My Restaurant",
    "receipt_text": "...",
    "receipt_lines": [...],
    "receipt_html": "<html>...",
    "branding": { "logo_url": "...", "preferred_format": "html" },
    "payment_qr": { "state": "pending", "qr_url": "..." }
  }
}
```

---

### Export Orders PDF
```
GET /restaurants/analytics/orders/export-pdf
```
Returns a bulk orders PDF for the date range.

---

### Export Orders CSV
```
GET /restaurants/analytics/orders/export-csv
```
Returns a CSV download. Columns: Date, Order ID, Order Type, Table Number, Total Items, Subtotal, Discount, Tax/GST, Final Amount, Payment Method, Status, Staff Name.

---

## 8. Finance & GST Reports

> **Auth:** JWT required. **Permission:** `view_dashboard`

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/restaurants/:id/gst-summary` | GST tax summary |
| `GET` | `/restaurants/:id/gst-invoices` | GST invoice listing |
| `GET` | `/restaurants/:id/gst-hsn-summary` | HSN-wise GST summary |
| `GET` | `/restaurants/:id/receipts` | Receipt listing |
| `GET` | `/restaurants/:id/dashboard-summary` | Financial dashboard summary |

All support `from`, `to`, and `period` query params.

---

## 9. Staff Management

> **Auth:** JWT required. **Permission:** `manage_restaurant`

### Register Staff
```
POST /restaurants/:id/staff
```
**Body:**
```json
{ "user_id": "uuid-of-user", "role_id": 2 }
```
**Response (201):** `{ "data": { "staff": { ... } } }`

---

### List Staff
```
GET /restaurants/:id/staff
```
**Response (200):** `{ "data": { "staff": [...] } }`

---

### Deactivate Staff
```
DELETE /restaurants/:id/staff/:staff_id
```

---

## 10. RBAC (Role-Based Access Control)

> **Auth:** JWT required. **Permission:** `manage_roles`

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/restaurants/rbac/roles` | Create a new role |
| `POST` | `/restaurants/rbac/permissions` | Create a new permission |
| `POST` | `/restaurants/rbac/roles/:id/permissions` | Assign permission to role |

---

## 11. Notifications

> **Auth:** JWT required.

### Register Device Token
```
POST /notifications/device-token
```
**Body:** `{ "token": "fcm-token", "platform": "android" }`

`device_token` is also accepted as an alias for `token`.

---

### Remove Device Token
```
DELETE /notifications/device-token
```
**Body:** `{ "token": "fcm-token" }`

---

### List Notifications
```
GET /notifications
```
| Query Param | Type | Description |
|---|---|---|
| `page` | int | Page (default: 1) |
| `limit` | int | Items per page (default: 20) |

**Response (200):**
```json
{ "data": { "items": [...], "pagination": { "page": 1, "limit": 20, "total": 42 } } }
```

---

### Mark Notification Read
```
PATCH /notifications/:id/read
```

---

## 12. Kitchen Display System (KDS)

> **Auth:** JWT required. **Permission:** `manage_orders`

### Get Active KDS Orders
```
GET /kds/orders
```
| Query Param | Type | Description |
|---|---|---|
| `stationId` | string | Optional station filter |

**Response (200):** JSON array of KDS order objects.

---

### Update KDS Order Status
```
PATCH /kds/orders/:orderId/status
```
**Body:** `{ "status": "preparing" }`

Valid transitions: `new` → `preparing` → `ready` → `served`

---

### Add Note to KDS Order
```
POST /kds/orders/:orderId/note
```
**Body:** `{ "note": "Extra spicy, no onions" }`

**Response (201):** `{ "success": true, "message": "Note added successfully" }`

---

### KDS Events (SSE)
```
GET /kds/events?token=<jwt>
```
**No header auth** — JWT passed via `token` query param (EventSource limitation).

Events: `connected`, `order:new`, `order:status_change`

---

## 13. Owner Intelligence & Copilot

> **Auth:** JWT required.

### POS Event Tracking
```
POST /v1/events/pos
```
> **Permission:** `manage_orders`

---

### Dashboard Overview (v1)
```
GET /v1/dashboard/overview
```
> **Permission:** `view_dashboard`

---

### List Insights
```
GET /v1/dashboard/insights
```
> **Permission:** `view_dashboard`

---

### Update Insight Status
```
PATCH /v1/dashboard/insights/:id
```
> **Permission:** `view_dashboard`

---

### Run Day Rollup
```
POST /v1/dashboard/rollup
```
> **Permission:** `view_dashboard`

---

### Copilot Voice Query
```
POST /v1/copilot/voice
```
> **Permission:** `view_dashboard`

Accepts a voice/text query and returns AI-generated insights.

---

## 14. Incentives

> **Auth:** JWT required.

### Get Current Incentive
```
GET /restaurants/:id/qrunch/incentive/current
```

### Collect Rewards
```
POST /restaurants/:id/qrunch/incentive/collect-rewards
```

### Incentive History
```
GET /restaurants/:id/qrunch/incentive/history
```

---

## 15. Ratings & Reviews

> **Auth:** JWT required.

### Get Rating Summary
```
GET /v1/ratings/summary
```
> **Permission:** `view_dashboard`

---

### List Feedback
```
GET /v1/ratings/list
```
> **Permission:** `view_dashboard`

---

### Patch Feedback
```
PATCH /v1/ratings/:id
```
> **Permission:** `view_dashboard`

---

### Create Rating Link
```
POST /v1/ratings/link
```
> **Permission:** `manage_orders`

---

### Update Review Settings (Outlet)
```
PUT /v1/outlets/:outletId/review-settings
```
> **Permission:** `manage_restaurant`

---

## 16. Offers & Smart Discounts

> **Auth:** JWT required. **Permission:** `manage_orders`

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/offers` | Create offer |
| `PUT` | `/v1/offers/:id` | Update offer |
| `POST` | `/v1/offers/:id/activate` | Activate offer |
| `POST` | `/v1/offers/:id/pause` | Pause offer |
| `GET` | `/v1/offers` | List all offers |
| `GET` | `/v1/offers/:id` | Get offer details |
| `POST` | `/v1/offers/evaluate` | Evaluate offers for a cart |
| `POST` | `/v1/offers/apply` | Apply offer to an order |

### QRunch Public Offers (No Auth)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/offers` | Get available offers |
| `GET` | `/offers/recommendations` | Get recommended offers |
| `GET` | `/offers/checkout` | Get checkout-time offer |

---

## 17. Customer Identity System

### QRunch Public (Session-based, No JWT)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/customer/identity/init` | Initialize customer identity |
| `GET` | `/customer/identity` | Get current identity |
| `POST` | `/customer/identity/phone` | Attach phone number |

Uses optional restaurant session middleware.

---

### Staff Endpoints (JWT + `manage_orders`)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/v1/customers/lookup?phone=9876543210` | Lookup by phone |
| `GET` | `/v1/customers/search?q=john` | Search customers |
| `GET` | `/v1/customers/recent` | Get recent customers |

---

## 18. Smart Recommendations

### Staff Endpoints (JWT + `manage_orders`)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/recommendations/item` | Get item-level recommendations |
| `POST` | `/v1/recommendations/cart` | Get cart upsell recommendations |
| `POST` | `/v1/recommendations/review` | Get order review recommendations |

---

## 19. Referrals

> **Auth:** JWT required. **Permission:** `manage_restaurant`

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/referrals/code` | Create referral code |
| `POST` | `/v1/referrals/invite` | Send invite |
| `GET` | `/v1/referrals` | List referrals |
| `GET` | `/v1/referrals/:id` | Get referral details |

### Internal

```
POST /internal/referrals/attribution
```
> **Permission:** `manage_roles`. Handles referral attribution.

---

## 20. Inventory & Recipes

### Units (JWT required)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/units` | List all units |
| `GET` | `/api/v1/unit-conversions` | List conversions |
| `GET` | `/api/v1/units/convert` | Convert between units |
| `POST` | `/api/v1/unit-conversions` | Create conversion (**`manage_inventory`**) |

### Ingredients (`manage_inventory`)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/v1/ingredients` | Create |
| `GET` | `/api/v1/ingredients` | List |
| `GET` | `/api/v1/ingredients/:id` | Get details |
| `PATCH` | `/api/v1/ingredients/:id` | Update |
| `DELETE` | `/api/v1/ingredients/:id` | Delete |

### Vendors (`manage_inventory`)

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/api/v1/vendors` | Create |
| `GET` | `/api/v1/vendors` | List |
| `GET` | `/api/v1/vendors/:id` | Get details |
| `PATCH` | `/api/v1/vendors/:id` | Update |
| `DELETE` | `/api/v1/vendors/:id` | Delete |

### Recipes (`manage_inventory`)

| Method | Endpoint | Description |
|---|---|---|
| `PUT` | `/v1/menu-items/:menuItemId/recipes/:variant` | Upsert recipe |
| `GET` | `/v1/menu-items/:menuItemId/recipes/:variant` | Get active recipe |
| `POST` | `/v1/inventory/deduct-from-order` | Deduct inventory from order |
| `POST` | `/v1/inventory/stock/receive` | Receive stock |

---

## 21. Multi-Outlet Management

### Organizations (JWT required)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/organizations` | My organizations |
| `POST` | `/api/v1/organizations` | Create organization |
| `GET` | `/api/v1/organizations/:orgId` | Get org details |
| `PUT` | `/api/v1/organizations/:orgId` | Update org (**org_owner/org_admin**) |

### Org Users

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/organizations/:orgId/users` | List org users |
| `POST` | `/api/v1/organizations/:orgId/users` | Add org user |
| `DELETE` | `/api/v1/organizations/:orgId/users/:userId` | Remove user (**org_owner** only) |

### Outlets

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/organizations/:orgId/outlets` | List outlets |
| `POST` | `/api/v1/organizations/:orgId/outlets` | Create outlet |
| `GET` | `/api/v1/organizations/:orgId/dashboard` | Org dashboard |
| `GET` | `/api/v1/outlets/:outletId` | Get outlet |
| `PUT` | `/api/v1/outlets/:outletId` | Update outlet |
| `POST` | `/api/v1/outlets/:outletId/activate` | Activate outlet |
| `POST` | `/api/v1/outlets/:outletId/deactivate` | Deactivate outlet |

### Outlet Staff

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/outlets/:outletId/staff` | List outlet staff |
| `POST` | `/api/v1/outlets/:outletId/staff` | Assign staff |
| `PATCH` | `/api/v1/outlets/:outletId/staff/:userId` | Update staff role |
| `DELETE` | `/api/v1/outlets/:outletId/staff/:userId` | Remove staff |

### Menu Overrides

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/outlets/:outletId/menu-overrides` | Get overrides |
| `POST` | `/api/v1/outlets/:outletId/menu-overrides` | Set override |
| `DELETE` | `/api/v1/outlets/:outletId/menu-overrides/:menuItemId` | Remove override |

### Settings Overrides

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/outlets/:outletId/settings` | Get settings |
| `POST` | `/api/v1/outlets/:outletId/settings` | Set setting |
| `DELETE` | `/api/v1/outlets/:outletId/settings/:key` | Remove setting |

### User Outlets & Search

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/v1/user/outlets` | Get accessible outlets |
| `GET` | `/api/v1/users/search` | Search users (for staff assignment) |

---

## 22. Wallet

> Conditional — only active when Razorpay credentials are configured.

### Verify Bank Account
```
POST /wallet/restaurants/:id/wallet/verify-bank
```

---

## 23. Consumer App (Discovery)

> **No Auth required.**

### Home Data
```
GET /api/home
```
| Query | Type | Description |
|---|---|---|
| `lat` | float | Latitude |
| `lng` | float | Longitude |

---

### Browse Restaurants
```
GET /api/restaurants
```
| Query | Type | Description |
|---|---|---|
| `q` | string | Search query |
| `category` | string | Category filter |
| `sort` | string | `recommended`, `distance`, `rating` |
| `lat` | float | Latitude |
| `lng` | float | Longitude |
| `radius` | float | Radius in km |
| `page` | int | Page |
| `limit` | int | Limit |

---

### Get Restaurant Menu (Consumer)
```
GET /api/restaurants/:restaurant_id/menu
```

---

## 24. Consumer Orders (Online)

> **Auth:** JWT required.

### Place Online Order
```
POST /api/orders/online
```
**Body:**
```json
{
  "restaurantId": 1,
  "orderType": "DELIVERY",
  "customer": { "name": "John", "phone": "9876543210", "email": "john@example.com" },
  "deliveryAddress": { "street": "123 Main St", "city": "Delhi", "zipCode": "110001" },
  "items": [{ "name": "...", "qty": 1, "unitPrice": 200, "totalPrice": 200 }],
  "paymentMethod": "upi",
  "subtotal": 200, "taxAmount": 10, "totalAmount": 210
}
```

**Response (201):**
```json
{ "success": true, "data": { "orderId": 1001, "status": "PAYMENT_PENDING", "paymentUrl": "..." } }
```

---

### Order History
```
GET /api/orders/history
```

### Live Order Tracking (SSE)
```
GET /api/orders/:orderId/live
```

### Get Order Details (Consumer)
```
GET /api/orders/:orderId
```

---

### Saved Payment Methods
```
GET /api/users/payment-methods
```

---

## 25. FoodShare

> **Auth:** JWT required.

### Create FoodShare Session
```
POST /api/foodshare
```

### Get Active Sessions
```
GET /api/foodshare/active
```

---

## 26. Demo Requests

### Submit Demo Request
```
POST /api/v1/demo-requests
```
> **No Auth required.** Stored in MongoDB, sends email notification.

---

## Standard Response Format

All endpoints use a consistent wrapper:

### Success
```json
{
  "success": true,
  "message": "descriptive message",
  "data": { ... }
}
```

### Error
```json
{
  "success": false,
  "message": "error description",
  "error": "detailed error (if applicable)"
}
```

---

## Authentication

- **Header:** `Authorization: Bearer <JWT>`
- **JWT Claims:** `sub` (user ID), `role` (user role)
- **Session:** QRunch uses cookie-based sessions (`restaurant_session` cookie)
- **SSE:** KDS events use `?token=<jwt>` query param

## Permissions Reference

| Permission | Description |
|---|---|
| `manage_restaurant` | Create, update, delete restaurants, menus, staff, tables |
| `manage_orders` | Manage orders, KDS, offers, customer lookup |
| `view_dashboard` | Access analytics, dashboard, ratings, copilot |
| `view_orders` | View order details and receipts |
| `manage_roles` | RBAC role and permission management |
| `manage_inventory` | Ingredients, vendors, recipes, stock |
| `orders_admin` | Admin-level cross-restaurant order access |

## Org/Outlet Roles

| Role | Description |
|---|---|
| `org_owner` | Full organization access |
| `org_admin` | Organization admin |
| `finance_admin` | Financial dashboards |
| `operations_admin` | Outlet operations |
| `outlet_manager` | Per-outlet management |
