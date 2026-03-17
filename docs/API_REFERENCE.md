# WalletX Backend - API Reference

Dokumen ini berisi dokumentasi lengkap untuk seluruh *endpoint* REST API WalletX Backend, beserta format *Request Body* dan *Response*-nya.

---

## 🌎 Base URL & Authentication

- Batalkan semua request ke prefix: `/api/v1`
- **Protected Endpoints** (membutuhkan login) wajib menyertakan HTTP Header:
  ```http
  Authorization: Bearer <jwt_token>
  ```

### Standar Format Response JSAON

**Berhasil (Success API Response):**
```json
{
  "status": "success",
  "message": "Deskripsi pesan sukses",
  "data": { ... } // Isi payload data aktual
}
```

**Gagal (Error API Response):**
```json
{
  "status": "fail", // atau "error" untuk server crash
  "message": "Deskripsi pesan gagal",
  "errors": "..." // Detail error opsional
}
```

---

## 1. Authentication & Health Check

### 1.1 Ping Server
Mengecek apakah server sedang berjalan (Health check).
- **Method**: `GET`
- **URL**: `/api/v1/ping`
- **Auth**: Public

**Response (200 OK):**
```json
{
  "message": "WalletX API is running!"
}
```

---

### 1.2 Google Auth (Login / Register)
Otentikasi pengguna via Google SSO dari aplikasi *mobile*.
- **Method**: `POST`
- **URL**: `/api/v1/auth/google`
- **Auth**: Public

**Request Body:**
```json
{
  "id_token": "eyJhbGciOiJSUzI1..." // Token yang didapat dari Google SDK di Mobile Android/iOS
}
```

**Response (200 OK - Login / 201 Created - Baru Pertama Register):**
```json
{
  "status": "success",
  "message": "Login berhasil",
  "data": {
    "user": {
      "id": "e458e0a3-00fb-40db-9556-9e9d6d3d9e80",
      "google_id": "1141380...",
      "email": "user@gmail.com",
      "name": "User Name",
      "picture": "https://lh3.googleusercontent.com/a/...",
      "created_at": "2026-03-01T15:04:05Z",
      "updated_at": "2026-03-01T15:04:05Z"
    },
    "token": "eyJhbGciOiJIUzI1NiIs..." // Gunakan token ini untuk endpoint Protected
  }
}
```

---

## 2. Transactions

Segala proses rekaman arus uang yang ditarik dari bukti mutasi (*email crawler*) dan atau juga dimasukkan secara manual.

### 2.1 Get All Transactions
Mengambil riwayat transaksi.
- **Method**: `GET`
- **URL**: `/api/v1/transactions`
- **Auth**: Bearer Token
- **Query Params**:
  - `limit` (int, default: 20)
  - `offset` (int, default: 0)

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Transactions retrieved successfully",
  "data": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "e458e0a3-00fb-40db-9556-9e9d6d3d9e80",
      "category_id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871", // Bisa null
      "amount": 50000,
      "merchant": "GrabFood",
      "note": "Lunch",
      "transaction_date": "2026-03-16T12:00:00Z",
      "message_id": "manual-msg-id-123", // Null jika manual, email ID jika auto
      "is_recurring": false,
      "created_at": "2026-03-16T12:05:00Z",
      "updated_at": "2026-03-16T12:05:00Z"
    }
  ]
}
```

---

### 2.2 Create Manual Transaction
Membuat catatan transaksi manual oleh pengguna melalui tombol *App/Frontend*.
- **Method**: `POST`
- **URL**: `/api/v1/transactions`
- **Auth**: Bearer Token

**Request Body:**
```json
{
  "amount": 50000,
  "merchant": "GrabFood",
  "note": "Makan siang pakai promo", 
  "category_id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871", // Opsional, bisa null (dikirim sbg UUID string biasa atau hapus dari payload)
  "transaction_date": "2026-03-16T12:00:00+07:00" // Wajib format RFC3339 / ISO 8601
}
```

**Response (201 Created):**
```json
{
  "status": "success",
  "message": "Transaction created successfully",
  "data": {
    "id": "...",
    "user_id": "...",
    "category_id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871",
    "amount": 50000,
    "merchant": "GrabFood",
    "note": "Makan siang pakai promo",
    "transaction_date": "2026-03-16T05:00:00Z",
    "message_id": null,
    "is_recurring": false,
    "created_at": "...",
    "updated_at": "..."
  }
}
```

---

## 3. Categories

Master data pemetaan kategori pengeluaran oleh pengguna.

### 3.1 Get All Categories
- **Method**: `GET`
- **URL**: `/api/v1/categories`
- **Auth**: Bearer Token

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Categories retrieved successfully",
  "data": [
    {
      "id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871",
      "user_id": "...",
      "name": "Food & Beverage",
      "icon": "🍔",
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

### 3.2 Create Category
- **Method**: `POST`
- **URL**: `/api/v1/categories`
- **Auth**: Bearer Token

**Request Body:**
```json
{
  "name": "Transport",
  "icon": "🚗" // Opsional
}
```

**Response (201 Created):** Mengembalikan object *Category* yang baru dibuat.

### 3.3 Get Category by ID
- **Method**: `GET`
- **URL**: `/api/v1/categories/:id`
- **Auth**: Bearer Token
- Mengembalikan object *Category*.

### 3.4 Update Category
- **Method**: `PUT`
- **URL**: `/api/v1/categories/:id`
- **Auth**: Bearer Token

**Request Body:** (Sama dengan format Create)
```json
{
  "name": "Public Transport",
  "icon": "🚆"
}
```

### 3.5 Delete Category
- **Method**: `DELETE`
- **URL**: `/api/v1/categories/:id`
- **Auth**: Bearer Token

**Response (200 OK):** *"Category deleted successfully"*

---

## 4. Budgets (Category Limits)

Fitur pembatasan/rencana batas anggaran maksimum *spending* dalam kategori pengeluaran per minggu/bulan.

### 4.1 Get All Budget Limits
- **Method**: `GET`
- **URL**: `/api/v1/budgets`
- **Auth**: Bearer Token

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Budget limits retrieved successfully",
  "data": [
    {
      "id": "e7c6fd34-17c1-4c77-96a1-87a3def24220",
      "user_id": "...",
      "category_id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871",
      "limit_amount": 1000000,
      "period": "monthly", // atau "weekly"
      "is_active": true,
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

### 4.2 Create Budget Limit
- **Method**: `POST`
- **URL**: `/api/v1/budgets`
- **Auth**: Bearer Token

**Request Body:**
```json
{
  "category_id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871",
  "limit_amount": 1000000,
  "period": "monthly" // Harus "weekly" atau "monthly"
}
```

### 4.3 Get Budget Limit by ID
- **Method**: `GET`
- **URL**: `/api/v1/budgets/:id`
- **Auth**: Bearer Token
- Mengembalikan object *CategoryLimit*.

### 4.4 Update Budget Limit
- **Method**: `PUT`
- **URL**: `/api/v1/budgets/:id`
- **Auth**: Bearer Token

**Request Body:**
```json
{
  "limit_amount": 1500000,
  "period": "monthly",
  "is_active": true // Jika false akan dianggap mati dari kalkulasi
}
```

### 4.5 Delete Budget Limit
- **Method**: `DELETE`
- **URL**: `/api/v1/budgets/:id`
- **Auth**: Bearer Token
- Menghapus konfigurasi (Hard delete).

---

## 5. Recurring Transactions

Fitur cicilan otomatis atau langganan tetap yang selalu memotong uang *(tagihan kos, subscription Spotify/Netflix dll)* secara *weekly/monthly/yearly*.

### 5.1 Get All Recurring Configs
- **Method**: `GET`
- **URL**: `/api/v1/recurrings`
- **Auth**: Bearer Token

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Recurring configs retrieved successfully",
  "data": [
    {
      "id": "89ec2880-9cc9-450f-9d33-1cb05ccb1cd5",
      "user_id": "...",
      "category_id": "...",
      "amount": 60000,
      "frequency": "monthly",
      "start_date": "2026-03-01T00:00:00Z",
      "next_due_date": "2026-04-01T00:00:00Z",
      "created_at": "...",
      "updated_at": "..."
    }
  ]
}
```

### 5.2 Create Recurring Config
- **Method**: `POST`
- **URL**: `/api/v1/recurrings`
- **Auth**: Bearer Token

**Request Body:**
```json
{
  "category_id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871",
  "amount": 60000,
  "frequency": "monthly", // Harus "weekly", "monthly", atau "yearly"
  "start_date": "2026-03-10T00:00:00Z"
}
```
*Catatan: Sistem menyalin `next_due_date` sama dengan `start_date` saat dibuat.*

### 5.3 Get Recurring Config by ID
- **Method**: `GET`
- **URL**: `/api/v1/recurrings/:id`
- **Auth**: Bearer Token

### 5.4 Update Recurring Config
- **Method**: `PUT`
- **URL**: `/api/v1/recurrings/:id`
- **Auth**: Bearer Token

**Request Body:**
```json
{
  "amount": 65000,
  "frequency": "monthly",
  "start_date": "2026-03-10T00:00:00Z" // Opsional, bila diubah hanya menarget meta namun tidak mengubah next_due_date saat ini
}
```

### 5.5 Delete Recurring Config
- **Method**: `DELETE`
- **URL**: `/api/v1/recurrings/:id`
- **Auth**: Bearer Token

---

## 6. Dashboard (Budget Summary)

Ringkasan pengeluaran riil *(actual spending)* versus *(vs)* batas batas anggaran *(budget limits)* yang telah ditetapkan (berguna untuk menampilkan *Chart / UI Progress Bar* pada aplikasi Mobile).

### 6.1 Get Budget Summary
- **Method**: `GET`
- **URL**: `/api/v1/dashboard/budget-summary`
- **Auth**: Bearer Token

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Budget summary retrieved successfully",
  "data": [
    {
      "category_id": "a4d8f8d9-e889-4b2a-8ea6-583eb1be6871",
      "limit_amount": 1000000,      // Batas maksimal anggaran
      "spent_amount": 450000,       // Uang yang sudah dikonsumsi
      "remaining_budget": 550000,   // Sisa uang sebelum jebol
      "period": "monthly"           // Mengikuti konfigurasi budget
    },
    {
      "category_id": "b3d8f8aa-e889-4b2a-8ea6-583eb1be6111",
      "limit_amount": 250000,
      "spent_amount": 300000,
      "remaining_budget": -50000,   // Minus berarti Overbudget
      "period": "weekly"
    }
  ]
}
```
*(Array data diolah dari gabungan referensi data tabel `category_limits` dan PostgreSQL VIEW `daily_expense_summary` untuk efisiensi.)*

### 6.2 Get Daily Calendar (Aggregated Spending)
Mengambil agregasi total pengeluaran harian pada bulan dan tahun tertentu (berguna untuk menampilkan total di kotak kalender frontend).
- **Method**: `GET`
- **URL**: `/api/v1/dashboard/calendar`
- **Auth**: Bearer Token
- **Query Params**:
  - `month` (int, 1-12) - Wajib
  - `year` (int, misal: 2026) - Wajib

**Request Example:**
```http
GET /api/v1/dashboard/calendar?month=3&year=2026
```

**Response (200 OK):**
```json
{
  "status": "success",
  "message": "Daily calendar retrieved successfully",
  "data": [
    {
      "day": "2026-03-10T00:00:00Z",
      "total_amount": 150000
    },
    {
      "day": "2026-03-15T00:00:00Z",
      "total_amount": 75000
    },
    {
      "day": "2026-03-16T00:00:00Z",
      "total_amount": 200000
    }
  ]
}
```
*(Data hanya dikembalikan untuk hari yang memiliki pengeluaran pada bulan dan tahun tersebut. Jika suatu hari tidak memiliki pengeluaran, maka tanggal tersebut tidak akan muncul di array)*
