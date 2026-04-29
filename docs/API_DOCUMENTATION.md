# WalletX API Documentation

**Version:** 2.0.0 (RESTful Compliant)  
**Base URL:** `http://localhost:8080/api/v1`  
**Last Updated:** January 2026

---

## Table of Contents

1. [Overview](#overview)
2. [Authentication](#authentication)
3. [Response Format](#response-format)
4. [HTTP Status Codes](#http-status-codes)
5. [Endpoints](#endpoints)
   - [Health Check](#health-check)
   - [Authentication](#authentication-endpoints)
   - [Transactions](#transactions)
   - [Categories](#categories)
   - [Budgets](#budgets)
   - [Reports](#reports)
   - [Cron Jobs](#cron-jobs)
6. [Query Parameters Reference](#query-parameters-reference)
7. [Error Handling](#error-handling)
8. [Rate Limiting](#rate-limiting)
9. [Changelog](#changelog)

---

## Overview

WalletX API adalah RESTful API untuk aplikasi manajemen keuangan pribadi. API ini mendukung operasi CRUD lengkap untuk transaksi, kategori, budget, dan laporan keuangan.

### Design Principles

- **Resource-Based URLs**: Semua endpoint menggunakan kata benda jamak (e.g., `/transactions`, `/categories`)
- **HTTP Methods**: GET (read), POST (create), PUT (update), DELETE (remove)
- **JSON Responses**: Semua response menggunakan format JSON yang konsisten
- **Stateless**: Setiap request harus menyertakan authentication token
- **Content Negotiation**: Support CSV export via `Accept` header

---

## Authentication

### JWT Bearer Token

Semua endpoint (kecuali `/auth/google` dan `/cron/*`) memerlukan JWT token.

**Header Format:**
```
Authorization: Bearer <your_jwt_token>
```

### Mendapatkan JWT Token

1. Login melalui Google OAuth di endpoint `/auth/google`
2. Response akan menyertakan `token` di dalam `data`
3. Gunakan token tersebut untuk semua request selanjutnya

### Token Expiration

- JWT token berlaku selama **7 hari** dari tanggal pembuatan
- Setelah expired, client harus login ulang
- Untuk logout, gunakan endpoint `/auth/logout` (token akan di-blacklist)

---

## Response Format

### Standard Response Structure

Semua response mengikuti format berikut:

```json
{
  "status": "success",
  "message": "Operation completed successfully",
  "data": { ... },
  "meta": {
    "page": 1,
    "limit": 50,
    "total": 127,
    "total_pages": 3
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Response Fields

| Field | Type | Description |
|---|---|---|
| `status` | string | Status operasi: `"success"`, `"error"`, atau `"fail"` |
| `message` | string | Pesan human-readable |
| `data` | object/array | Response payload (tidak ada jika null) |
| `meta` | object | Metadata (pagination, dll.) |
| `timestamp` | string | Timestamp response (ISO 8601) |

### Status Values

| Value | HTTP Codes | Description |
|---|---|---|
| `success` | 200, 201, 204 | Operasi berhasil |
| `error` | 500, 502, 503 | Server error |
| `fail` | 400-499 | Client error |

---

## HTTP Status Codes

### Success Codes

| Code | Name | Usage |
|---|---|---|
| 200 | OK | GET, PUT berhasil |
| 201 | Created | POST berhasil membuat resource |
| 204 | No Content | DELETE berhasil (no body) |

### Client Error Codes

| Code | Name | Usage |
|---|---|---|
| 400 | Bad Request | Invalid input, validation error |
| 401 | Unauthorized | Missing/invalid JWT token |
| 403 | Forbidden | User tidak memiliki akses ke resource |
| 404 | Not Found | Resource tidak ditemukan |
| 422 | Unprocessable Entity | Validasi business logic gagal |

### Server Error Codes

| Code | Name | Usage |
|---|---|---|
| 500 | Internal Server Error | Server error umum |
| 502 | Bad Gateway | External service error |
| 503 | Service Unavailable | Service sedang down |

---

## Endpoints

### Health Check

#### `GET /ping`

Health check endpoint untuk memastikan API berjalan.

**Authentication:** Tidak diperlukan (public)

**Request:**
```http
GET /api/v1/ping HTTP/1.1
Host: localhost:8080
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "WalletX API is running!",
  "data": null,
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

### Authentication Endpoints

#### `POST /auth/google`

Login menggunakan Google OAuth token dari mobile app.

**Authentication:** Tidak diperlukan (public)

**Request:**
```http
POST /api/v1/auth/google HTTP/1.1
Host: localhost:8080
Content-Type: application/json

{
  "id_token": "your-google-id-token"
}
```

**Request Body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `id_token` | string | Yes | Google ID token dari mobile app |

**Response (200 OK - Login):**
```json
{
  "status": "success",
  "message": "Login berhasil",
  "data": {
    "user": {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "email": "user@example.com",
      "name": "John Doe",
      "avatar_url": "https://lh3.googleusercontent.com/...",
      "created_at": "2024-01-01T00:00:00Z"
    },
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (201 Created - New User Registration):**
```json
{
  "status": "success",
  "message": "Registrasi berhasil",
  "data": {
    "user": { ... },
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (400 Bad Request):**
```json
{
  "status": "fail",
  "message": "Data tidak valid",
  "errors": "invalid id_token format",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `POST /auth/logout`

Logout dan blacklist JWT token.

**Authentication:** JWT Required

**Request:**
```http
POST /api/v1/auth/logout HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (204 No Content):**
```
(no body)
```

**Response (400 Bad Request):**
```json
{
  "status": "fail",
  "message": "Invalid Authorization header",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

### Transactions

#### `GET /transactions`

Mengambil daftar transaksi dengan filtering, sorting, dan pagination.

**Authentication:** JWT Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `limit` | integer | 50 | Items per page (max: 100) |
| `offset` | integer | 0 | Offset untuk pagination |
| `date_from` | string | - | Filter tanggal mulai (YYYY-MM-DD) |
| `date_to` | string | - | Filter tanggal akhir (YYYY-MM-DD) |
| `date` | string | - | Filter tanggal exact (legacy) |
| `amount_min` | number | - | Filter nominal minimum |
| `amount_max` | number | - | Filter nominal maksimum |
| `q` | string | - | Search query (merchant name) |
| `sort` | string | `transaction_date:desc` | Sorting (format: `field:direction`) |
| `last_updated_at` | string | - | Delta sync timestamp (ISO 8601) |

**Sort Options:**
- `transaction_date:asc` / `transaction_date:desc`
- `amount:asc` / `amount:desc`
- `created_at:asc` / `created_at:desc`
- `updated_at:asc` / `updated_at:desc`
- `merchant:asc` / `merchant:desc`

**Request (JSON):**
```http
GET /api/v1/transactions?limit=50&offset=0&sort=transaction_date:desc HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Accept: application/json
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Transactions retrieved successfully",
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "550e8400-e29b-41d4-a716-446655440001",
      "category_id": "550e8400-e29b-41d4-a716-446655440002",
      "amount": 75000,
      "merchant": "Starbucks Indonesia",
      "note": "Coffee meeting",
      "transaction_date": "2024-01-15T10:30:00Z",
      "created_at": "2024-01-15T10:30:00Z",
      "updated_at": "2024-01-15T10:30:00Z",
      "deleted_at": null,
      "category": {
        "id": "550e8400-e29b-41d4-a716-446655440002",
        "name": "Food & Beverage",
        "icon": "🍔"
      }
    }
  ],
  "meta": {
    "page": 1,
    "limit": 50,
    "total": 127,
    "total_pages": 3
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Request (Search):**
```http
GET /api/v1/transactions?q=starbucks&limit=20 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK - Search):**
```json
{
  "status": "success",
  "message": "Search results",
  "data": [ ... ],
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 5,
    "total_pages": 1
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Request (CSV Export):**
```http
GET /api/v1/transactions?month=1&year=2024 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Accept: text/csv
```

**Response (200 OK - CSV):**
```csv
Tanggal,Merchant,Nominal,Kategori,Catatan
2024-01-15 10:30,Starbucks Indonesia,75000.00,Food & Beverage,Coffee meeting
2024-01-14 14:20,McDonald's,50000.00,Food & Beverage,Lunch
```

---

#### `POST /transactions`

Membuat transaksi baru secara manual.

**Authentication:** JWT Required

**Request Body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `amount` | number | Yes | Nominal transaksi (harus > 0) |
| `merchant` | string | Yes | Nama merchant/toko |
| `note` | string | No | Catatan tambahan |
| `category_id` | string (UUID) | No | ID kategori |
| `transaction_date` | string (ISO 8601) | Yes | Tanggal transaksi |

**Request:**
```http
POST /api/v1/transactions HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "amount": 75000,
  "merchant": "Starbucks Indonesia",
  "note": "Coffee meeting with client",
  "category_id": "550e8400-e29b-41d4-a716-446655440002",
  "transaction_date": "2024-01-15T10:30:00Z"
}
```

**Response (201 Created):**
```json
{
  "status": "success",
  "message": "Transaction created successfully",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "user_id": "550e8400-e29b-41d4-a716-446655440001",
    "category_id": "550e8400-e29b-41d4-a716-446655440002",
    "amount": 75000,
    "merchant": "Starbucks Indonesia",
    "note": "Coffee meeting with client",
    "transaction_date": "2024-01-15T10:30:00Z",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z",
    "deleted_at": null,
    "category": { ... }
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (400 Bad Request):**
```json
{
  "status": "fail",
  "message": "Invalid request body",
  "errors": {
    "amount": "amount must be greater than 0",
    "merchant": "merchant cannot be empty"
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `PUT /transactions/:id`

Update transaksi (partial update supported).

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID transaksi yang akan diupdate |

**Request Body (semua field optional):**

| Field | Type | Description |
|---|---|---|
| `amount` | number | Nominal transaksi baru |
| `merchant` | string | Nama merchant baru |
| `note` | string | Catatan baru |
| `category_id` | string (UUID) | ID kategori baru |
| `transaction_date` | string (ISO 8601) | Tanggal baru |

**Note:** Hanya field yang dikirim yang akan diupdate (partial update).

**Request:**
```http
PUT /api/v1/transactions/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "amount": 80000,
  "note": "Updated: Coffee meeting"
}
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Transaction updated successfully",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "amount": 80000,
    "merchant": "Starbucks Indonesia",
    "note": "Updated: Coffee meeting",
    ...
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (404 Not Found):**
```json
{
  "status": "fail",
  "message": "Transaction not found",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (403 Forbidden):**
```json
{
  "status": "fail",
  "message": "You are not authorized to access this transaction",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `DELETE /transactions/:id`

Menghapus transaksi.

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID transaksi yang akan dihapus |

**Request:**
```http
DELETE /api/v1/transactions/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (204 No Content):**
```
(no body)
```

**Response (404 Not Found):**
```json
{
  "status": "fail",
  "message": "Transaction not found",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (403 Forbidden):**
```json
{
  "status": "fail",
  "message": "You are not authorized to access this transaction",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

### Categories

#### `GET /categories`

Mengambil daftar semua kategori milik user.

**Authentication:** JWT Required

**Request:**
```http
GET /api/v1/categories HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Categories retrieved successfully",
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "550e8400-e29b-41d4-a716-446655440001",
      "name": "Food & Beverage",
      "icon": "🍔",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440002",
      "user_id": "550e8400-e29b-41d4-a716-446655440001",
      "name": "Transportation",
      "icon": "🚗",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `POST /categories`

Membuat kategori baru.

**Authentication:** JWT Required

**Request Body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Nama kategori |
| `icon` | string | No | Emoji/icon kategori |

**Request:**
```http
POST /api/v1/categories HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "name": "Entertainment",
  "icon": "🎬"
}
```

**Response (201 Created):**
```json
{
  "status": "success",
  "message": "Category created successfully",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "user_id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Entertainment",
    "icon": "🎬",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `GET /categories/:id`

Mengambil detail kategori berdasarkan ID.

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID kategori |

**Request:**
```http
GET /api/v1/categories/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Category retrieved successfully",
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "user_id": "550e8400-e29b-41d4-a716-446655440001",
    "name": "Food & Beverage",
    "icon": "🍔",
    "created_at": "2024-01-01T00:00:00Z",
    "updated_at": "2024-01-01T00:00:00Z"
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (404 Not Found):**
```json
{
  "status": "fail",
  "message": "Category not found",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `PUT /categories/:id`

Update kategori.

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID kategori |

**Request Body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Nama kategori baru |
| `icon` | string | No | Icon baru |

**Request:**
```http
PUT /api/v1/categories/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "name": "Food & Drinks",
  "icon": "🍕"
}
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Category updated successfully",
  "data": { ... },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `DELETE /categories/:id`

Menghapus kategori.

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID kategori |

**Request:**
```http
DELETE /api/v1/categories/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (204 No Content):**
```
(no body)
```

---

### Budgets

#### `GET /budgets`

Mengambil daftar semua budget milik user.

**Authentication:** JWT Required

**Request:**
```http
GET /api/v1/budgets HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Budget limits retrieved successfully",
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "550e8400-e29b-41d4-a716-446655440001",
      "category_id": "550e8400-e29b-41d4-a716-446655440002",
      "limit_amount": 2000000,
      "period": "monthly",
      "created_at": "2024-01-01T00:00:00Z",
      "updated_at": "2024-01-01T00:00:00Z",
      "category": {
        "id": "550e8400-e29b-41d4-a716-446655440002",
        "name": "Food & Beverage",
        "icon": "🍔"
      }
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `GET /budgets/progress`

Mengambil progress budget vs actual spending.

**Authentication:** JWT Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `date` | string (YYYY-MM-DD) | today | Tanggal untuk menghitung progress |

**Request:**
```http
GET /api/v1/budgets/progress?date=2024-01-15 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Budget progress retrieved successfully",
  "data": [
    {
      "category_id": "550e8400-e29b-41d4-a716-446655440002",
      "category_name": "Food & Beverage",
      "limit_amount": 2000000,
      "spent_amount": 750000,
      "remaining_budget": 1250000,
      "percentage_used": 37.5
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `POST /budgets`

Membuat budget limit baru.

**Authentication:** JWT Required

**Request Body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `category_id` | string (UUID) | Yes | ID kategori |
| `limit_amount` | number | Yes | Nominal budget limit |
| `period` | string | Yes | Periode: `"monthly"` |

**Request:**
```http
POST /api/v1/budgets HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "category_id": "550e8400-e29b-41d4-a716-446655440002",
  "limit_amount": 2000000,
  "period": "monthly"
}
```

**Response (201 Created):**
```json
{
  "status": "success",
  "message": "Budget limit created successfully",
  "data": { ... },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `GET /budgets/:id`

Mengambil detail budget berdasarkan ID.

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID budget |

**Request:**
```http
GET /api/v1/budgets/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Budget limit retrieved successfully",
  "data": { ... },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `PUT /budgets/:id`

Update budget.

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID budget |

**Request Body:**

| Field | Type | Required | Description |
|---|---|---|---|
| `category_id` | string (UUID) | No | ID kategori baru |
| `limit_amount` | number | No | Limit baru |
| `period` | string | No | Periode baru |

**Request:**
```http
PUT /api/v1/budgets/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "limit_amount": 2500000
}
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Budget limit updated successfully",
  "data": { ... },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `DELETE /budgets/:id`

Menghapus budget.

**Authentication:** JWT Required

**Path Parameters:**

| Parameter | Type | Description |
|---|---|---|
| `id` | string (UUID) | ID budget |

**Request:**
```http
DELETE /api/v1/budgets/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (204 No Content):**
```
(no body)
```

---

### Reports

#### `GET /reports/expenses`

Laporan pengeluaran grouped by kategori.

**Authentication:** JWT Required

**Query Parameters:**

| Parameter | Type | Default | Description |
|---|---|---|---|
| `month` | integer | - | Bulan (1-12) |
| `year` | integer | - | Tahun (4 digit) |
| `group_by` | string | `category` | Grouping (hanya support: `"category"`) |

**Request:**
```http
GET /api/v1/reports/expenses?month=1&year=2024&group_by=category HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Reports retrieved",
  "data": [
    {
      "category_name": "Food & Beverage",
      "total_amount": 750000,
      "percentage": 45.5
    },
    {
      "category_name": "Transportation",
      "total_amount": 500000,
      "percentage": 30.3
    },
    {
      "category_name": "Lainnya",
      "total_amount": 400000,
      "percentage": 24.2
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (400 Bad Request):**
```json
{
  "status": "fail",
  "message": "Unsupported group_by value. Use 'category'",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `GET /reports/budget-summary`

Ringkasan budget dengan actual spending.

**Authentication:** JWT Required

**Request:**
```http
GET /api/v1/reports/budget-summary HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Budget summary retrieved successfully",
  "data": [
    {
      "category_id": "550e8400-e29b-41d4-a716-446655440002",
      "category_name": "Food & Beverage",
      "limit_amount": 2000000,
      "spent_amount": 750000,
      "remaining_budget": 1250000
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `GET /reports/daily-calendar`

Kalender pengeluaran harian untuk bulan/tahun tertentu.

**Authentication:** JWT Required

**Query Parameters:**

| Parameter | Type | Required | Description |
|---|---|---|---|
| `month` | integer | Yes | Bulan (1-12) |
| `year` | integer | Yes | Tahun (4 digit) |

**Request:**
```http
GET /api/v1/reports/daily-calendar?month=1&year=2024 HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Daily calendar retrieved successfully",
  "data": [
    {
      "date": "2024-01-01",
      "total_amount": 150000,
      "transaction_count": 3
    },
    {
      "date": "2024-01-02",
      "total_amount": 75000,
      "transaction_count": 1
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (400 Bad Request):**
```json
{
  "status": "fail",
  "message": "month and year query parameters are required",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

### Cron Jobs

Endpoint cron diproteksi dengan secret-based authentication (bukan JWT).

**Authentication:** Cron Secret

**Header Options:**
```
Authorization: Bearer <cron_secret>
```
atau
```
x-cron-secret: <cron_secret>
```

---

#### `GET /cron/keep-alive`

Menjaga database tetap aktif (mencegah Supabase free-tier pause).

**Request:**
```http
GET /api/v1/cron/keep-alive HTTP/1.1
Host: localhost:8080
Authorization: Bearer your-cron-secret
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Keep-alive successful",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `GET /cron/imap`

Memicu sync transaksi dari email via IMAP.

**Request:**
```http
GET /api/v1/cron/imap HTTP/1.1
Host: localhost:8080
Authorization: Bearer your-cron-secret
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "IMAP sync executed successfully",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

**Response (500 Internal Server Error):**
```json
{
  "status": "error",
  "message": "IMAP sync failed",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

#### `GET /cron/recurring`

Memicu processing recurring transactions.

**Request:**
```http
GET /api/v1/cron/recurring HTTP/1.1
Host: localhost:8080
Authorization: Bearer your-cron-secret
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Recurring processing executed successfully",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

## Query Parameters Reference

### Filtering

| Parameter | Format | Example | Description |
|---|---|---|---|
| `date_from` | YYYY-MM-DD | `2024-01-01` | Filter dari tanggal |
| `date_to` | YYYY-MM-DD | `2024-01-31` | Filter sampai tanggal |
| `amount_min` | number | `50000` | Nominal minimum |
| `amount_max` | number | `500000` | Nominal maksimum |

### Pagination

| Parameter | Type | Default | Max | Description |
|---|---|---|---|---|
| `limit` | integer | 50 | 100 | Items per page |
| `offset` | integer | 0 | - | Offset |

### Sorting

Format: `field:direction`

| Field | Direction | Example |
|---|---|---|
| `transaction_date` | `asc`, `desc` | `transaction_date:desc` |
| `amount` | `asc`, `desc` | `amount:asc` |
| `created_at` | `asc`, `desc` | `created_at:desc` |
| `merchant` | `asc`, `desc` | `merchant:asc` |

### Search

| Parameter | Description | Example |
|---|---|---|
| `q` | Search merchant name (case-insensitive, partial match) | `?q=starbucks` |

---

## Error Handling

### Error Response Format

```json
{
  "status": "fail" | "error",
  "message": "Human-readable error message",
  "errors": "Detailed error information (optional)",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### Common Errors

#### 400 Bad Request
```json
{
  "status": "fail",
  "message": "Invalid request body",
  "errors": {
    "amount": "amount must be greater than 0",
    "merchant": "merchant cannot be empty"
  },
  "timestamp": "2024-01-15T10:30:00Z"
}
```

#### 401 Unauthorized
```json
{
  "status": "fail",
  "message": "Invalid or expired token",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

#### 403 Forbidden
```json
{
  "status": "fail",
  "message": "You are not authorized to access this transaction",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

#### 404 Not Found
```json
{
  "status": "fail",
  "message": "Transaction not found",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

#### 500 Internal Server Error
```json
{
  "status": "error",
  "message": "Failed to retrieve transactions",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

---

## Rate Limiting

Saat ini API tidak menerapkan rate limiting. Namun, disarankan untuk:

1. **Pagination**: Selalu gunakan `limit` dan `offset` untuk menghindari response terlalu besar
2. **Caching**: Cache response GET yang tidak sering berubah
3. **Delta Sync**: Gunakan `last_updated_at` untuk sinkronisasi incremental

---

## Changelog

### v2.0.0 (January 2026) - RESTful Compliance

**Breaking Changes:**
- ✅ Endpoint restructuring: `/dashboard/*` → `/reports/*`
- ✅ Merged `/transactions/search` → `GET /transactions?q=`
- ✅ Merged `/transactions/export` → Content negotiation (`Accept: text/csv`)
- ✅ DELETE endpoints now return `204 No Content` (was `200 OK`)
- ✅ Partial update support for PUT endpoints

**New Features:**
- ✅ Advanced filtering: `date_from`, `date_to`, `amount_min`, `amount_max`
- ✅ Sorting: `sort=field:direction`
- ✅ Pagination metadata in response
- ✅ 403 Forbidden for unauthorized access attempts
- ✅ Proper 404 Not Found handling

**Improvements:**
- ✅ Consistent response envelope across all endpoints
- ✅ SQL injection protection via whitelist validation
- ✅ Better error messages and status codes

### v1.0.0 (2024) - Initial Release

- Basic CRUD operations
- JWT authentication
- Google OAuth login
- Simple pagination

---

## Support & Contact

**Documentation:** This file  
**API Base URL:** `http://localhost:8080/api/v1`  
**Version:** 2.0.0

For issues and questions, please refer to the project repository.
