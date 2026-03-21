# User Service API Documentation

## Overview

The User Service API is built with Go (Gin framework) and provides endpoints for user authentication, user management, address management, and role-based access control (RBAC).

**Base URL:** `http://localhost:8080`

---

## Table of Contents

1. [Authentication APIs](#authentication-apis)
2. [User Management APIs](#user-management-apis)
3. [Address Management APIs](#address-management-apis)
4. [Role & Permission APIs](#role--permission-apis)
5. [Error Handling](#error-handling)
6. [Response Format](#response-format)

---

## Authentication APIs

### 1. Send OTP

**Endpoint:** `POST /auth/send-otp`

**Authentication:** Not required

**Description:** Sends an OTP to the provided phone number via SMS. Used for both user registration and login.

**Request Body:**

```json
{
  "phone": "+1234567890",
  "source": "login"
}
```

**Request Parameters:**

| Parameter | Type   | Required | Description                |
|-----------|--------|----------|----------------------------|
| phone     | string | Yes      | Phone number with country code |
| source    | string | No       | Source of the OTP request (e.g., "login", "registration") |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "otp sent",
  "data": {
    "phone": "+1234567890",
    "expiresAt": "2026-03-20T10:05:00Z"
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/auth/send-otp' \
  --header 'Content-Type: application/json' \
  --data '{
    "phone": "+1234567890",
    "source": "login"
  }'
```

---

### 2. Verify OTP

**Endpoint:** `POST /auth/verify-otp`

**Authentication:** Not required

**Description:** Verifies the OTP and returns a JWT authentication token.

**Request Body:**

```json
{
  "phone": "+1234567890",
  "otp": "123456",
  "userType": "customer"
}
```

**Request Parameters:**

| Parameter | Type   | Required | Description                            |
|-----------|--------|----------|----------------------------------------|
| phone     | string | Yes      | Phone number (must match send-otp call) |
| otp       | string | Yes      | 6-digit OTP received via SMS           |
| userType  | string | No       | User type: "customer", "restaurant_owner", "restaurant_staff" (defaults to "customer") |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "authenticated",
  "data": {
    "authToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

**Error Response (401 Unauthorized):**

```json
{
  "status": "error",
  "statusCode": 401,
  "message": "invalid otp",
  "error": "OTP has expired or is incorrect"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/auth/verify-otp' \
  --header 'Content-Type: application/json' \
  --data '{
    "phone": "+1234567890",
    "otp": "123456",
    "userType": "customer"
  }'
```

---

### 3. Send Forgot Password OTP

**Endpoint:** `POST /auth/forgot-password/send-otp`

**Authentication:** Not required

**Description:** Sends an OTP to the email address associated with the user account for password reset.

**Request Body:**

```json
{
  "email": "user@example.com"
}
```

**Request Parameters:**

| Parameter | Type   | Required | Description                |
|-----------|--------|----------|----------------------------|
| email     | string | Yes      | User's email address       |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "otp sent to email",
  "data": {
    "email": "user@example.com",
    "expiresAt": "2026-03-20T10:05:00Z"
  }
}
```

**Error Response (404 Not Found):**

```json
{
  "status": "error",
  "statusCode": 404,
  "message": "email not found"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/auth/forgot-password/send-otp' \
  --header 'Content-Type: application/json' \
  --data '{
    "email": "user@example.com"
  }'
```

---

### 4. Verify Forgot Password OTP

**Endpoint:** `POST /auth/forgot-password/verify-otp`

**Authentication:** Not required

**Description:** Verifies the OTP sent to the user's email for password reset.

**Request Body:**

```json
{
  "email": "user@example.com",
  "otp": "123456"
}
```

**Request Parameters:**

| Parameter | Type   | Required | Description                |
|-----------|--------|----------|----------------------------|
| email     | string | Yes      | User's email address       |
| otp       | string | Yes      | OTP sent to email          |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "otp verified successfully",
  "data": {
    "email": "user@example.com"
  }
}
```

**Error Response (401 Unauthorized):**

```json
{
  "status": "error",
  "statusCode": 401,
  "message": "invalid otp",
  "error": "OTP is incorrect or has expired"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/auth/forgot-password/verify-otp' \
  --header 'Content-Type: application/json' \
  --data '{
    "email": "user@example.com",
    "otp": "123456"
  }'
```

---

### 5. Reset Forgot Password

**Endpoint:** `POST /auth/forgot-password/reset`

**Authentication:** Not required (but OTP verification required)

**Description:** Resets the user's password after OTP verification.

**Request Body:**

```json
{
  "email": "user@example.com",
  "password": "NewSecurePassword123!",
  "confirm_password": "NewSecurePassword123!"
}
```

**Request Parameters:**

| Parameter        | Type   | Required | Description                         |
|------------------|--------|----------|-------------------------------------|
| email            | string | Yes      | User's email address                |
| password         | string | Yes      | New password (min 8 characters)     |
| confirm_password | string | Yes      | Confirm new password (must match)   |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "Password reset successful",
  "data": {
    "authToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "email": "user@example.com",
    "phone": "+1234567890",
    "first_name": "John",
    "last_name": "Doe",
    "display_name": "John D.",
    "primary_role": "customer",
    "user_type": "customer",
    "full_name": "John Doe"
  }
}
```

**Error Responses:**

- **400 Bad Request:** Passwords don't match or password too short
- **401 Unauthorized:** OTP verification required
- **404 Not Found:** Email not found

**cURL Example:**

```bash
curl --location 'http://localhost:8080/auth/forgot-password/reset' \
  --header 'Content-Type: application/json' \
  --data '{
    "email": "user@example.com",
    "password": "NewSecurePassword123!",
    "confirm_password": "NewSecurePassword123!"
  }'
```

---

## User Management APIs

### 1. Create User

**Endpoint:** `POST /users`

**Authentication:** Not required (but required for restaurant_staff creation)

**Description:** Creates a new user account. Supports customer, restaurant_owner, and restaurant_staff user types.

**Request Body (Customer):**

```json
{
  "email": "john.doe@example.com",
  "phone": "+19876543210",
  "phone_country_code": "+1",
  "first_name": "John",
  "last_name": "Doe",
  "display_name": "John D.",
  "primary_role": "customer",
  "password": "StrongPassword123!"
}
```

**Request Body (Restaurant Owner):**

```json
{
  "email": "owner@mybistro.com",
  "phone": "+919876543212",
  "first_name": "Rajesh",
  "last_name": "Kumar",
  "primary_role": "restaurant_owner",
  "password": "OwnerPass789!",
  "business_name": "Rajesh Bistro",
  "business_legal_name": "Rajesh Kumar Foods Pvt Ltd",
  "business_phone": "+919876543213",
  "business_registration_number": "CIN123456789",
  "gstin": "29ABCDE1234F1Z5",
  "business_address": "123 MG Road, Bangalore, Karnataka 560001"
}
```

**Request Parameters:**

| Parameter                       | Type   | Required | Description                              |
|---------------------------------|--------|----------|------------------------------------------|
| email                           | string | Yes      | User's email address                     |
| phone                           | string | Yes      | User's phone number                      |
| phone_country_code              | string | No       | Country code for phone (e.g., "+1")      |
| first_name                      | string | Yes      | User's first name                        |
| last_name                       | string | Yes      | User's last name                         |
| display_name                    | string | No       | Display name for user                    |
| full_name                       | string | No       | Full name (computed if not provided)     |
| primary_role                    | string | Yes      | "customer", "restaurant_owner", "restaurant_staff" |
| password                        | string | Yes      | Account password (min 8 chars)           |
| user_type                       | string | No       | Legacy field (same as primary_role)      |
| business_name                   | string | No       | Business name (for restaurant_owner)     |
| business_legal_name             | string | No       | Legal business name                      |
| business_phone                  | string | No       | Business phone number                    |
| business_registration_number    | string | No       | Business registration/CIN number         |
| gstin                           | string | No       | GST Identification Number                |
| business_address                | string | No       | Business address                         |

**Response (201 Created):**

```json
{
  "status": "success",
  "statusCode": 201,
  "message": "user created successfully",
  "data": {
    "user": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "email": "john.doe@example.com",
      "phone": "+19876543210",
      "phone_country_code": "+1",
      "first_name": "John",
      "last_name": "Doe",
      "display_name": "John D.",
      "full_name": "John Doe",
      "primary_role": "customer",
      "user_type": "customer",
      "status": "active",
      "created_at": "2026-03-20T16:30:00Z",
      "updated_at": "2026-03-20T16:30:00Z"
    },
    "authToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/users' \
  --header 'Content-Type: application/json' \
  --data '{
    "email": "john.doe@example.com",
    "phone": "+19876543210",
    "phone_country_code": "+1",
    "first_name": "John",
    "last_name": "Doe",
    "display_name": "John D.",
    "primary_role": "customer",
    "password": "StrongPassword123!"
  }'
```

---

### 2. Login User

**Endpoint:** `POST /users/login`

**Authentication:** Not required

**Description:** Authenticates a user with email and password, returns JWT token.

**Request Body:**

```json
{
  "email": "john.doe@example.com",
  "password": "StrongPassword123!"
}
```

**Request Parameters:**

| Parameter | Type   | Required | Description           |
|-----------|--------|----------|------------------------|
| email     | string | Yes      | User's email address   |
| password  | string | Yes      | User's password        |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "Login successful",
  "data": {
    "authToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "email": "john.doe@example.com",
    "phone": "+19876543210",
    "first_name": "John",
    "last_name": "Doe",
    "display_name": "John D.",
    "primary_role": "customer",
    "user_type": "customer",
    "full_name": "John Doe"
  }
}
```

**Error Response (401 Unauthorized):**

```json
{
  "status": "error",
  "statusCode": 401,
  "message": "Invalid credentials",
  "error": "Invalid email or password"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/users/login' \
  --header 'Content-Type: application/json' \
  --data '{
    "email": "john.doe@example.com",
    "password": "StrongPassword123!"
  }'
```

---

### 3. Get All Users (Paginated)

**Endpoint:** `GET /users`

**Authentication:** Not required

**Description:** Retrieves a paginated list of all users.

**Query Parameters:**

| Parameter | Type    | Default | Description      |
|-----------|---------|---------|------------------|
| page      | integer | 1       | Page number      |
| limit     | integer | 10      | Items per page (max 100) |

**Response (200 OK):**

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "john.doe@example.com",
    "primary_role": "customer",
    "created_at": "2026-03-20T10:00:00Z",
    "phone": "+19876543210",
    "first_name": "John",
    "last_name": "Doe",
    "full_name": "John Doe"
  },
  {
    "id": "550e8400-e29b-41d4-a716-446655440001",
    "email": "jane.smith@example.com",
    "primary_role": "customer",
    "created_at": "2026-03-20T11:00:00Z",
    "phone": "+19876543211",
    "first_name": "Jane",
    "last_name": "Smith",
    "full_name": "Jane Smith"
  }
]
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/users?page=1&limit=10'
```

---

### 4. Get User by ID

**Endpoint:** `GET /users/:id`

**Authentication:** Not required

**Description:** Retrieves detailed information about a specific user.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | User UUID   |

**Response (200 OK):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "john.doe@example.com",
  "phone": "+19876543210",
  "phone_country_code": "+1",
  "first_name": "John",
  "last_name": "Doe",
  "display_name": "John D.",
  "full_name": "John Doe",
  "primary_role": "customer",
  "user_type": "customer",
  "status": "active",
  "created_at": "2026-03-20T16:30:00Z",
  "updated_at": "2026-03-20T16:30:00Z"
}
```

**Error Response (404 Not Found):**

```json
{
  "error": "User not found"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000'
```

---

### 5. Update User

**Endpoint:** `PUT /users/:id`

**Authentication:** Not required

**Description:** Updates user profile information. Partial updates supported.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | User UUID   |

**Request Body:**

```json
{
  "first_name": "Johnny",
  "display_name": "Johnny D",
  "phone": "+11234567890"
}
```

**Request Parameters:**

All user fields are optional. Only provide fields to be updated:

| Parameter | Type   | Description      |
|-----------|--------|------------------|
| first_name | string | First name      |
| last_name  | string | Last name       |
| display_name | string | Display name  |
| phone | string | Phone number    |
| email | string | Email address   |

**Response (200 OK):**

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "john.doe@example.com",
  "phone": "+11234567890",
  "first_name": "Johnny",
  "last_name": "Doe",
  "display_name": "Johnny D",
  "full_name": "Johnny Doe",
  "primary_role": "customer",
  "updated_at": "2026-03-20T17:45:00Z"
}
```

**cURL Example:**

```bash
curl --location --request PUT 'http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000' \
  --header 'Content-Type: application/json' \
  --data '{
    "first_name": "Johnny",
    "display_name": "Johnny D"
  }'
```

---

### 6. Delete User

**Endpoint:** `DELETE /users/:id`

**Authentication:** Not required

**Description:** Deletes a user account and all associated data.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | User UUID   |

**Response (204 No Content):**

No response body returned on successful deletion.

**cURL Example:**

```bash
curl --location --request DELETE 'http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000'
```

---

## Address Management APIs

### 1. Create Address

**Endpoint:** `POST /users/:id/addresses`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Only user can create address for their own account (OwnerOnly middleware)

**Description:** Creates a new address for a user.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | User UUID   |

**Request Body:**

```json
{
  "label": "Home",
  "address_line1": "123 Main St",
  "address_line2": "Apt 4B",
  "city": "Metropolis",
  "state": "NY",
  "country": "USA",
  "pincode": "10001",
  "is_default": true,
  "latitude": 40.7128,
  "longitude": -74.0060
}
```

**Request Parameters:**

| Parameter      | Type    | Required | Description                    |
|----------------|---------|----------|--------------------------------|
| label          | string  | Yes      | Address label (e.g., "Home", "Work") |
| address_line1  | string  | Yes      | Primary address line           |
| address_line2  | string  | No       | Secondary address line         |
| city           | string  | Yes      | City name                      |
| state          | string  | Yes      | State/Province name            |
| country        | string  | No       | Country name                   |
| pincode        | string  | Yes      | Postal code/ZIP code           |
| is_default     | boolean | No       | Mark as default address        |
| latitude       | float   | No       | Latitude coordinate            |
| longitude      | float   | No       | Longitude coordinate           |

**Response (201 Created):**

```json
{
  "status": "success",
  "statusCode": 201,
  "message": "address created",
  "data": {
    "address": {
      "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "label": "Home",
      "address_line1": "123 Main St",
      "address_line2": "Apt 4B",
      "city": "Metropolis",
      "state": "NY",
      "country": "USA",
      "pincode": "10001",
      "is_default": true,
      "latitude": 40.7128,
      "longitude": -74.0060,
      "created_at": "2026-03-20T16:30:00Z",
      "updated_at": "2026-03-20T16:30:00Z"
    }
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/addresses' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "label": "Home",
    "address_line1": "123 Main St",
    "city": "Metropolis",
    "state": "NY",
    "pincode": "10001",
    "is_default": true
  }'
```

---

### 2. Get User Addresses (Paginated)

**Endpoint:** `GET /users/:id/addresses`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** User can get their own addresses or admin can get any user's addresses

**Description:** Retrieves a paginated list of addresses for a specific user.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | User UUID   |

**Query Parameters:**

| Parameter | Type    | Default | Description      |
|-----------|---------|---------|------------------|
| page      | integer | 1       | Page number      |
| limit     | integer | 10      | Items per page (max 100) |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "addresses fetched",
  "data": {
    "items": [
      {
        "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
        "user_id": "550e8400-e29b-41d4-a716-446655440000",
        "label": "Home",
        "address_line1": "123 Main St",
        "address_line2": "Apt 4B",
        "city": "Metropolis",
        "state": "NY",
        "country": "USA",
        "pincode": "10001",
        "is_default": true,
        "created_at": "2026-03-20T16:30:00Z",
        "updated_at": "2026-03-20T16:30:00Z"
      },
      {
        "id": "b2c3d4e5-f6g7-8901-2345-678901bcdefg",
        "user_id": "550e8400-e29b-41d4-a716-446655440000",
        "label": "Work",
        "address_line1": "456 Business Ave",
        "city": "Gotham",
        "state": "NJ",
        "country": "USA",
        "pincode": "07000",
        "is_default": false,
        "created_at": "2026-03-20T17:15:00Z",
        "updated_at": "2026-03-20T17:15:00Z"
      }
    ]
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/addresses?page=1&limit=5' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN'
```

---

### 3. Get Address by ID

**Endpoint:** `GET /addresses/:id`

**Authentication:** Required (JWT token in Authorization header)

**Description:** Retrieves a specific address by ID.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | Address UUID |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "address fetched",
  "data": {
    "address": {
      "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "label": "Home",
      "address_line1": "123 Main St",
      "address_line2": "Apt 4B",
      "city": "Metropolis",
      "state": "NY",
      "country": "USA",
      "pincode": "10001",
      "is_default": true,
      "latitude": 40.7128,
      "longitude": -74.0060,
      "created_at": "2026-03-20T16:30:00Z",
      "updated_at": "2026-03-20T16:30:00Z"
    }
  }
}
```

**Error Response (404 Not Found):**

```json
{
  "status": "error",
  "statusCode": 404,
  "message": "address not found"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/addresses/a1b2c3d4-e5f6-7890-1234-567890abcdef' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN'
```

---

### 4. Update Address

**Endpoint:** `PUT /addresses/:id`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Only the address owner can update their address (OwnerOnly middleware)

**Description:** Updates an existing address. Supports partial updates.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | Address UUID |

**Request Body:**

```json
{
  "address_line1": "456 Side St",
  "city": "Gotham",
  "is_default": true
}
```

**Request Parameters:**

All address fields are optional. Only provide fields to be updated.

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "address updated",
  "data": {
    "address": {
      "id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "label": "Home",
      "address_line1": "456 Side St",
      "address_line2": "Apt 4B",
      "city": "Gotham",
      "state": "NJ",
      "country": "USA",
      "pincode": "10001",
      "is_default": true,
      "updated_at": "2026-03-20T18:00:00Z"
    }
  }
}
```

**Error Responses:**

- **403 Forbidden:** User does not own this address
- **404 Not Found:** Address not found

**cURL Example:**

```bash
curl --location --request PUT 'http://localhost:8080/addresses/a1b2c3d4-e5f6-7890-1234-567890abcdef' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "address_line1": "456 Side St",
    "city": "Gotham",
    "is_default": true
  }'
```

---

### 5. Delete Address

**Endpoint:** `DELETE /addresses/:id`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Only the address owner can delete their address (OwnerOnly middleware)

**Description:** Deletes an address.

**Path Parameters:**

| Parameter | Type   | Description |
|-----------|--------|-------------|
| id        | string | Address UUID |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "address deleted"
}
```

**Error Responses:**

- **403 Forbidden:** User does not own this address
- **404 Not Found:** Address not found

**cURL Example:**

```bash
curl --location --request DELETE 'http://localhost:8080/addresses/a1b2c3d4-e5f6-7890-1234-567890abcdef' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN'
```

---

## Role & Permission APIs

### 1. Create Role

**Endpoint:** `POST /roles`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Requires "manage_roles" permission

**Description:** Creates a new role for RBAC.

**Request Body:**

```json
{
  "name": "manager",
  "description": "Store Manager with order management permissions"
}
```

**Request Parameters:**

| Parameter   | Type   | Required | Description       |
|-------------|--------|----------|-------------------|
| name        | string | Yes      | Role name         |
| description | string | No       | Role description  |

**Response (201 Created):**

```json
{
  "status": "success",
  "statusCode": 201,
  "message": "role created",
  "data": {
    "role": {
      "id": 1,
      "name": "manager",
      "description": "Store Manager with order management permissions",
      "created_at": "2026-03-20T16:30:00Z"
    }
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/roles' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "name": "manager",
    "description": "Store Manager with order management permissions"
  }'
```

---

### 2. Get All Roles

**Endpoint:** `GET /roles`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Requires "manage_roles" permission

**Description:** Retrieves all available roles.

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "roles fetched",
  "data": {
    "roles": [
      {
        "id": 1,
        "name": "manager",
        "description": "Store Manager with order management permissions",
        "created_at": "2026-03-20T16:30:00Z"
      },
      {
        "id": 2,
        "name": "staff",
        "description": "Restaurant Staff",
        "created_at": "2026-03-20T16:35:00Z"
      }
    ]
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/roles' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN'
```

---

### 3. Create Permission

**Endpoint:** `POST /permissions`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Requires "manage_roles" permission

**Description:** Creates a new permission.

**Request Body:**

```json
{
  "name": "manage_orders",
  "description": "Permission to manage restaurant orders"
}
```

**Request Parameters:**

| Parameter   | Type   | Required | Description          |
|-------------|--------|----------|----------------------|
| name        | string | Yes      | Permission name      |
| description | string | No       | Permission description |

**Response (201 Created):**

```json
{
  "status": "success",
  "statusCode": 201,
  "message": "permission created",
  "data": {
    "permission": {
      "id": 5,
      "name": "manage_orders",
      "description": "Permission to manage restaurant orders",
      "created_at": "2026-03-20T16:30:00Z"
    }
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/permissions' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "name": "manage_orders",
    "description": "Permission to manage restaurant orders"
  }'
```

---

### 4. Get All Permissions

**Endpoint:** `GET /permissions`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Requires "manage_roles" permission

**Description:** Retrieves all available permissions.

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "permissions fetched",
  "data": {
    "permissions": [
      {
        "id": 1,
        "name": "manage_roles",
        "description": "Permission to manage roles and permissions",
        "created_at": "2026-03-20T16:30:00Z"
      },
      {
        "id": 5,
        "name": "manage_orders",
        "description": "Permission to manage restaurant orders",
        "created_at": "2026-03-20T16:32:00Z"
      }
    ]
  }
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/permissions' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN'
```

---

### 5. Assign Permission to Role

**Endpoint:** `POST /roles/assign`

**Authentication:** Required (JWT token in Authorization header)

**Authorization:** Requires "manage_roles" permission

**Description:** Assigns a permission to a role.

**Request Body:**

```json
{
  "role_id": 1,
  "permission_id": 5
}
```

**Request Parameters:**

| Parameter     | Type    | Required | Description    |
|---------------|---------|----------|-----------------|
| role_id       | integer | Yes      | Role ID        |
| permission_id | integer | Yes      | Permission ID  |

**Response (200 OK):**

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "permission assigned"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/roles/assign' \
  --header 'Authorization: Bearer YOUR_AUTH_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "role_id": 1,
    "permission_id": 5
  }'
```

---

## Error Handling

All API errors follow a consistent format:

**Error Response Format:**

```json
{
  "status": "error",
  "statusCode": 400,
  "message": "User-friendly error message",
  "error": "Detailed error information (optional)"
}
```

### Common HTTP Status Codes

| Status Code | Meaning                                      |
|-------------|----------------------------------------------|
| 200 OK      | Request successful                           |
| 201 Created | Resource created successfully                |
| 204 No Content | Successful deletion (no response body)      |
| 400 Bad Request | Invalid request payload or parameters      |
| 401 Unauthorized | Missing or invalid authentication token   |
| 403 Forbidden | Authenticated but not authorized for action |
| 404 Not Found | Resource not found                           |
| 500 Internal Server Error | Server-side error occurred           |

---

## Response Format

### Success Response Format

```json
{
  "status": "success",
  "statusCode": 200,
  "message": "Human-readable success message",
  "data": {
    // Response data varies by endpoint
  }
}
```

### Examples of Success Responses by Status Code

**200 OK (GET request):**
```json
{
  "status": "success",
  "statusCode": 200,
  "message": "Resource fetched",
  "data": {
    "resource": { /* resource data */ }
  }
}
```

**201 Created (POST request):**
```json
{
  "status": "success",
  "statusCode": 201,
  "message": "Resource created successfully",
  "data": {
    "resource": { /* created resource data */ }
  }
}
```

**204 No Content (DELETE request):**
No response body returned.

---

## Authentication

Most endpoints require a JWT token in the `Authorization` header:

```
Authorization: Bearer <your_jwt_token>
```

### Token Structure

JWTs are obtained from:
1. **OTP Verification** - `/auth/verify-otp`
2. **User Login** - `/users/login`
3. **Password Reset** - `/auth/forgot-password/reset`

The token expires after a configured period (typically 24 hours).

### Token Claims

```json
{
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "phone": "+1234567890",
  "role": "customer",
  "iat": 1737000000,
  "exp": 1737086400
}
```

---

## Pagination

Endpoints that return lists support pagination via query parameters:

- **page**: Page number (1-indexed, default: 1)
- **limit**: Items per page (default: 10, max: 100)

Example:
```
GET /users?page=2&limit=20
```

---

## Middleware & Authorization

### AuthRequired Middleware

Protects endpoints that require authentication. Validates JWT token and extracts user information.

Used in endpoints:
- All address endpoints (`/addresses/*`)
- All role & permission endpoints (`/roles`, `/permissions`)

### OwnerOnly Middleware

Restricts operations to the resource owner or admin.

Used in endpoints:
- `PUT /addresses/:id` - Only address owner
- `DELETE /addresses/:id` - Only address owner
- `POST /users/:id/addresses` - Only user themselves

### OwnerOrAdmin Middleware

Allows access to both resource owner and admin users.

Used in endpoints:
- `GET /users/:id/addresses` - User or admin

### RequirePermission Middleware

Checks if the authenticated user has the required permission.

Used in endpoints:
- All role endpoints (`/roles`) - Requires "manage_roles"
- All permission endpoints (`/permissions`) - Requires "manage_roles"

---

## Field Definitions

### User Fields

| Field                        | Type    | Description                            |
|------------------------------|---------|----------------------------------------|
| id                           | UUID    | Unique user identifier                 |
| email                        | string  | User's email address                   |
| phone                        | string  | User's phone number                    |
| phone_country_code           | string  | Country code for phone                 |
| first_name                   | string  | User's first name                      |
| last_name                    | string  | User's last name                       |
| display_name                 | string  | Display name                           |
| full_name                    | string  | Full name (computed)                   |
| primary_role                 | string  | "customer", "restaurant_owner", "restaurant_staff" |
| user_type                    | string  | Legacy field (same as primary_role)    |
| status                       | string  | "active", "inactive", "suspended"      |
| business_name                | string  | Business name (for owners)             |
| business_legal_name          | string  | Legal business name                    |
| business_phone               | string  | Business phone                         |
| business_registration_number | string  | Business registration/CIN number       |
| gstin                        | string  | GST Identification Number              |
| business_address             | string  | Physical business address              |
| created_at                   | datetime | Account creation timestamp             |
| updated_at                   | datetime | Last update timestamp                  |

### Address Fields

| Field          | Type    | Description               |
|----------------|---------|---------------------------|
| id             | UUID    | Unique address identifier |
| user_id        | UUID    | Associated user ID        |
| label          | string  | Address label (Home, Work, etc.) |
| address_line1  | string  | Primary address line      |
| address_line2  | string  | Secondary address line    |
| city           | string  | City name                 |
| state          | string  | State/Province            |
| country        | string  | Country name              |
| pincode        | string  | Postal/ZIP code           |
| is_default     | boolean | Default address flag      |
| latitude       | float   | Latitude coordinate       |
| longitude      | float   | Longitude coordinate      |
| created_at     | datetime | Creation timestamp        |
| updated_at     | datetime | Last update timestamp     |

### Role Fields

| Field       | Type     | Description           |
|-------------|----------|-----------------------|
| id          | integer  | Unique role ID        |
| name        | string   | Role name             |
| description | string   | Role description      |
| created_at  | datetime | Creation timestamp    |

### Permission Fields

| Field       | Type     | Description              |
|-------------|----------|--------------------------|
| id          | integer  | Unique permission ID     |
| name        | string   | Permission name          |
| description | string   | Permission description   |
| created_at  | datetime | Creation timestamp       |

---

## Health Check

**Endpoint:** `GET /health`

**Authentication:** Not required

**Description:** Checks the health status of the service and database connection.

**Response (200 OK):**

```json
{
  "status": "healthy",
  "database": "connected"
}
```

**cURL Example:**

```bash
curl --location 'http://localhost:8080/health'
```

---

## Rate Limiting & Best Practices

1. **Rate Limiting**: No explicit rate limiting. Implement on your end if needed.
2. **Pagination**: Always use pagination for list endpoints to avoid performance issues.
3. **Error Handling**: Always check the response status code and error message.
4. **Token Expiration**: Handle 401 errors by requesting a new token.
5. **CORS**: Service is configured to accept requests from configured origins.

---

## Additional Notes

- All timestamps are in ISO 8601 format (UTC timezone)
- UUIDs are used for user and address identifiers
- Numeric IDs are used for roles and permissions
- Phone numbers should include country code (e.g., "+1" for USA)
- Passwords must be at least 8 characters long
- Email validation is performed on all email fields
- All endpoints support CORS with properly configured headers

---

## Support & Contact

For API issues or questions, contact the development team or check the service logs for detailed error information.

**Service Repository:** `Gursevak56/food-delivery-platform/services/user-service`
