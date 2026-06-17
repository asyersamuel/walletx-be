# WalletX — Master API Documentation & Integration Guide

> **PENTING UNTUK TIM FRONTEND (React Native)**
> Dokumen ini dirancang khusus untuk integrasi Frontend dengan WalletX Backend. Baca bagian **Network & HTTPS Setup** dan **Authentication Architecture** dengan seksama sebelum mengimplementasikan API call apapun.

---

## 1. Network & HTTPS Setup (CRITICAL FOR FE)

- **Strict HTTPS Requirement:** Backend berjalan di atas HTTPS menggunakan `mkcert` sebagai local Certificate Authority (CA).
- **Network Resolution:** Anda **TIDAK BISA** menggunakan `localhost` (atau `10.0.2.2` di Android emulator) jika testing di real device atau emulator yang perlu resolve IP lokal backend. Gunakan IP LAN host machine (contoh: `https://192.168.X.X:8080`).
- **SSL Certificate Handling:** Karena backend menggunakan local CA via `mkcert`, app React Native akan melempar `HandshakeException` (CERTIFICATE_VERIFY_FAILED). Anda **HARUS** mem-bypass validasi SSL lokal saat development.

---

## 2. Authentication Architecture (Single JWT System)

- **Single Token System:** API ini menggunakan sistem **SATU token**. Tidak ada refresh token untuk saat ini.
- **Token Delivery:** Setelah memanggil `/auth/google` dengan sukses, token ada di response payload di dalam `data.token` (ini adalah internal JWT).
- **Token Storage & Usage:** Simpan token secara aman (misalnya menggunakan `react-native-encrypted-storage`). Token harus disertakan di header semua protected endpoint:
  `Authorization: Bearer <token>`
- **Token Expiration & 401s:** Jika protected endpoint mengembalikan `401 Unauthorized`, FE harus menghapus token yang tersimpan dan meminta user untuk re-autentikasi via Google.
- **Logout:** Panggil `POST /api/v1/auth/logout` untuk menginvalidasi token di sisi server (token di-blacklist di Redis). FE tetap harus menghapus token dari local storage setelahnya.

---

## 3. Standar Format Response

Semua endpoint (sukses maupun error) menggunakan struktur JSON yang seragam:

```json
{
  "status": "success | fail | error",
  "message": "Human-readable description",
  "data": { ... },
  "errors": "Detail error (hanya untuk status fail/error)",
  "meta": { ... },
  "timestamp": "2026-06-18T03:00:00Z"
}
```

### Status Values
| `status` | HTTP Code | Artinya |
|---|---|---|
| `"success"` | 2xx | Operasi berhasil |
| `"fail"` | 4xx | Kesalahan dari sisi klien (input tidak valid, tidak ditemukan, dll) |
| `"error"` | 5xx | Kesalahan dari sisi server |

### Error Response Standar (Berlaku untuk Semua Protected Endpoint)

| HTTP Code | `status` | Kapan Terjadi |
|---|---|---|
| `400 Bad Request` | `"fail"` | Request body / query param tidak valid |
| `401 Unauthorized` | `"fail"` | Token tidak ada, expired, atau di-blacklist |
| `403 Forbidden` | `"fail"` | Resource ditemukan tapi bukan milik user ini |
| `404 Not Found` | `"fail"` | Resource tidak ditemukan di database |
| `409 Conflict` | `"fail"` | Terjadi duplikasi data (misalnya budget sudah ada untuk kategori ini) |
| `500 Internal Server Error` | `"error"` | Kesalahan server yang tidak terduga |

**Contoh `401 Unauthorized`:**
```json
{
  "status": "fail",
  "message": "Unauthorized access",
  "timestamp": "2026-06-18T03:00:00Z"
}
```

**Contoh `404 Not Found`:**
```json
{
  "status": "fail",
  "message": "Transaction not found",
  "timestamp": "2026-06-18T03:00:00Z"
}
```

**Contoh `409 Conflict`:**
```json
{
  "status": "fail",
  "message": "Budget already exists for this category",
  "timestamp": "2026-06-18T03:00:00Z"
}
```

---

## 4. Pagination (`meta` Object)

Endpoint yang mengembalikan list data besar menggunakan pagination via query params `limit` dan `offset`. Response akan menyertakan objek `meta`:

```json
{
  "status": "success",
  "message": "...",
  "data": [ ... ],
  "meta": {
    "page": 1,
    "limit": 50,
    "total": 150,
    "total_pages": 3
  },
  "timestamp": "..."
}
```

| Field `meta` | Tipe | Deskripsi |
|---|---|---|
| `page` | int | Halaman saat ini (dihitung dari offset/limit) |
| `limit` | int | Jumlah item per halaman |
| `total` | int | Total item yang cocok dengan filter |
| `total_pages` | int | Total halaman yang tersedia |

---

## 5. Complete Endpoints Specification

### 5.1. System & Authentication

#### Health Check (Ping)
- **Endpoint:** `GET /api/v1/ping`
- **Description:** Verifikasi apakah backend server berjalan dan dapat diakses.
- **Auth Required:** No
- **Success Response** (`200 OK`):
  ```json
  {
    "message": "WalletX API is running!"
  }
  ```

---

#### Google Authentication (Mobile App)
- **Endpoint:** `POST /api/v1/auth/google`
- **Description:** Autentikasi user menggunakan Google `id_token` yang diperoleh dari Google Sign-In SDK di mobile app.
- **Auth Required:** No
- **Headers:** `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "id_token": "string" // JWT string dari Google Sign-In SDK
  }
  ```
- **Success Response** (`200 OK` untuk Login, `201 Created` untuk Registrasi Baru):
  ```json
  {
    "status": "success",
    "message": "Login successful",
    "data": {
      "user": {
        "id": "uuid string",
        "google_id": "string",
        "email": "string",
        "name": "string",
        "picture": "string",
        "telegram_chat_id": "string | null",
        "created_at": "2026-06-18T03:00:00Z",
        "updated_at": "2026-06-18T03:00:00Z"
      },
      "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
    },
    "timestamp": "2026-06-18T03:00:00Z"
  }
  ```
- **Error Responses:**
  - `400 Bad Request`: `{"status": "fail", "message": "Invalid request body", "errors": "...", "timestamp": "..."}`
  - `500 Internal Server Error`: `{"status": "error", "message": "Failed to process authentication", "errors": "...", "timestamp": "..."}`

---

#### Logout
- **Endpoint:** `POST /api/v1/auth/logout`
- **Description:** Menginvalidasi JWT token saat ini dengan menambahkannya ke Redis blacklist hingga masa expiry-nya habis. Setelah ini, token tidak bisa dipakai lagi di endpoint manapun.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Request Body:** None
- **Success Response** (`204 No Content`): Response body kosong. FE harus menghapus token dari local storage setelah menerima response ini.
- **Error Responses:**
  - `400 Bad Request`: Header `Authorization` tidak ada atau formatnya salah.
  - `500 Internal Server Error`: Gagal memproses logout di server.

---

### 5.2. User Profile

#### Get Current User Profile
- **Endpoint:** `GET /api/v1/users/me`
- **Description:** Mengambil data profil lengkap dari user yang sedang terautentikasi. Gunakan endpoint ini untuk menampilkan nama/foto di halaman profil, atau untuk mengecek status koneksi Telegram (`telegram_chat_id`).
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Request Body:** None
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "User profile retrieved successfully",
    "data": {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "google_id": "1234567890",
      "email": "user@gmail.com",
      "name": "Budi Santoso",
      "picture": "https://lh3.googleusercontent.com/...",
      "telegram_chat_id": "987654321",
      "created_at": "2026-01-15T10:00:00Z",
      "updated_at": "2026-06-18T03:00:00Z"
    },
    "timestamp": "2026-06-18T03:00:00Z"
  }
  ```
  > **Catatan:** `telegram_chat_id` bernilai `null` jika user belum menghubungkan akun Telegram mereka.

---

### 5.3. Transactions Management

#### Get User Transactions (dengan Filter & Search)
- **Endpoint:** `GET /api/v1/transactions`
- **Description:** Mengambil daftar transaksi keuangan user dengan dukungan pagination, filter, sorting, dan pencarian. Endpoint ini juga berfungsi sebagai endpoint export CSV (lihat bagian Export di bawah).
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`

**Query Parameters:**

| Parameter | Tipe | Default | Deskripsi |
|---|---|---|---|
| `limit` | int | `50` | Jumlah transaksi per halaman |
| `offset` | int | `0` | Jumlah data yang dilewati (untuk pagination) |
| `q` | string | - | Kata kunci pencarian (mencari di kolom `merchant` dan `note`) |
| `date_from` | string | - | Filter tanggal mulai, format `YYYY-MM-DD` (misal: `2026-06-01`) |
| `date_to` | string | - | Filter tanggal akhir, format `YYYY-MM-DD` (misal: `2026-06-30`) |
| `date` | string | - | Filter satu hari spesifik, format `YYYY-MM-DD`. Jika diisi, mengabaikan `date_from` dan `date_to` |
| `amount_min` | float | - | Filter transaksi dengan nominal minimal |
| `amount_max` | float | - | Filter transaksi dengan nominal maksimal |
| `sort` | string | `transaction_date:DESC` | Sorting. Format: `<field>:<direction>`. Field yang valid: `transaction_date`, `amount`. Arah: `ASC` atau `DESC` |
| `last_updated_at` | string | - | Untuk delta-sync: mengambil hanya transaksi yang diupdate setelah timestamp ini (ISO 8601) |

**Success Response** (`200 OK`):
```json
{
  "status": "success",
  "message": "Transactions retrieved successfully",
  "data": [
    {
      "id": "uuid string",
      "user_id": "uuid string",
      "category_id": "uuid string | null",
      "amount": 150000.00,
      "merchant": "Kopi Kenangan",
      "note": "Meeting dengan klien",
      "transaction_date": "2026-06-18T10:00:00Z",
      "message_id": null,
      "is_recurring": false,
      "created_at": "2026-06-18T10:05:00Z",
      "updated_at": "2026-06-18T10:05:00Z",
      "category": {
        "id": "uuid string",
        "user_id": "uuid string",
        "name": "Food & Drink",
        "icon": "ic_food",
        "created_at": "...",
        "updated_at": "..."
      }
    }
  ],
  "meta": {
    "page": 1,
    "limit": 50,
    "total": 120,
    "total_pages": 3
  },
  "timestamp": "2026-06-18T10:05:00Z"
}
```

> **Catatan Field `category`:** Field ini akan `null` untuk transaksi yang belum dikategorikan.
> **Catatan Field `message_id`:** Berisi Email Message-ID jika transaksi dibuat otomatis oleh IMAP worker. `null` untuk transaksi manual.

**Contoh Penggunaan:**
```
# Ambil 20 transaksi bulan ini, urutkan terbaru dulu
GET /api/v1/transactions?limit=20&offset=0&date_from=2026-06-01&date_to=2026-06-30&sort=transaction_date:DESC

# Cari transaksi dengan keyword "kopi"
GET /api/v1/transactions?q=kopi&limit=10

# Filter transaksi dengan nominal 50.000 - 500.000
GET /api/v1/transactions?amount_min=50000&amount_max=500000
```

---

#### Export Transactions (CSV via Accept Header)
- **Endpoint:** `GET /api/v1/transactions` (endpoint SAMA, dibedakan oleh header)
- **Description:** Mengekspor transaksi user untuk bulan dan tahun tertentu sebagai file CSV. Trigger export dilakukan dengan mengirim header `Accept: text/csv`.
- **Auth Required:** Yes
- **Headers:**
  - `Authorization: Bearer <token>`
  - `Accept: text/csv` ← **Ini yang memicu mode export!**
- **Query Parameters (wajib saat export):**
  - `month` (int, Required): Bulan (1-12)
  - `year` (int, Required): Tahun (contoh: 2026)
- **Success Response** (`200 OK`): Response di-stream sebagai file `text/csv` dengan header `Content-Disposition: attachment; filename=transactions.csv`. Kolom CSV: `Tanggal`, `Merchant`, `Nominal`, `Kategori`, `Catatan`.

**Contoh Request:**
```
GET /api/v1/transactions?month=6&year=2026
Headers:
  Authorization: Bearer <token>
  Accept: text/csv
```

---

#### Create Transaction (Manual)
- **Endpoint:** `POST /api/v1/transactions`
- **Description:** Membuat transaksi manual dari mobile app.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "amount": 150000.00,         // required, harus > 0
    "merchant": "Kopi Kenangan", // required
    "note": "Meeting klien",     // optional
    "category_id": "uuid string",// optional, bisa null
    "transaction_date": "2026-06-18T10:00:00Z" // required, ISO 8601
  }
  ```
- **Success Response** (`201 Created`): Mengembalikan objek transaksi yang baru dibuat (format sama dengan item di GET list).
- **Error Responses:**
  - `400 Bad Request`: Field wajib tidak ada atau tipe data salah.
  - `409 Conflict`: Transaksi dengan `message_id` yang sama sudah ada.

---

#### Update Transaction
- **Endpoint:** `PUT /api/v1/transactions/:id`
- **Description:** Memperbarui transaksi yang ada. Hanya bisa mengupdate transaksi milik user sendiri.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "amount": 120000.00,
    "merchant": "Kopi Kenangan",
    "note": "Updated catatan",
    "category_id": "uuid string",
    "transaction_date": "2026-06-18T10:00:00Z"
  }
  ```
- **Success Response** (`200 OK`): Mengembalikan objek transaksi yang sudah diupdate.
- **Error Responses:**
  - `404 Not Found`: Transaksi tidak ditemukan.
  - `403 Forbidden`: Transaksi bukan milik user ini.

---

#### Delete Transaction
- **Endpoint:** `DELETE /api/v1/transactions/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`204 No Content`): Response body kosong (soft delete).
- **Error Responses:**
  - `404 Not Found`: Transaksi tidak ditemukan.
  - `403 Forbidden`: Transaksi bukan milik user ini.

---

### 5.4. Categories Management

#### Get All Categories
- **Endpoint:** `GET /api/v1/categories`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Categories retrieved successfully",
    "data": [
      {
        "id": "uuid string",
        "user_id": "uuid string",
        "name": "Food & Drink",
        "icon": "ic_food",
        "created_at": "2026-06-18T10:00:00Z",
        "updated_at": "2026-06-18T10:00:00Z"
      }
    ],
    "timestamp": "2026-06-18T10:00:00Z"
  }
  ```

---

#### Create Category
- **Endpoint:** `POST /api/v1/categories`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "name": "Food & Drink", // required
    "icon": "ic_food"       // optional, string identifier ikon
  }
  ```
- **Success Response** (`201 Created`): Mengembalikan objek category yang baru dibuat.
- **Error Responses:**
  - `409 Conflict`: Kategori dengan nama yang sama sudah ada.

---

#### Get Category By ID
- **Endpoint:** `GET /api/v1/categories/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Mengembalikan satu objek category.

---

#### Update Category
- **Endpoint:** `PUT /api/v1/categories/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "name": "Dining",   // required
    "icon": "ic_dining" // optional
  }
  ```
- **Success Response** (`200 OK`): Mengembalikan objek category yang sudah diupdate.

---

#### Delete Category
- **Endpoint:** `DELETE /api/v1/categories/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`204 No Content`): Response body kosong (soft delete).

---

### 5.5. Budget Limits Management

#### Get All Budgets
- **Endpoint:** `GET /api/v1/budgets`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Budget limits retrieved successfully",
    "data": [
      {
        "id": "uuid string",
        "user_id": "uuid string",
        "category_id": "uuid string",
        "limit_amount": 500000.00,
        "period": "monthly",
        "is_active": true,
        "created_at": "2026-06-18T10:00:00Z",
        "updated_at": "2026-06-18T10:00:00Z",
        "category": {
          "id": "uuid string",
          "name": "Food & Drink",
          "icon": "ic_food"
        }
      }
    ],
    "timestamp": "2026-06-18T10:00:00Z"
  }
  ```

---

#### Get Budget Progress
- **Endpoint:** `GET /api/v1/budgets/progress`
- **Description:** Mengembalikan progres (pengeluaran aktual vs batas budget) untuk semua budget aktif user berdasarkan tanggal target. Periode yang dihitung adalah bulan dari tanggal yang diberikan.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `date` (string, Optional, format: `YYYY-MM-DD`): Mengembalikan progres untuk periode bulan yang mengandung tanggal ini. Default: hari ini.
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Budget progress retrieved successfully",
    "data": [
      {
        "category_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "category_name": "Food & Drink",
        "limit_amount": 500000.00,
        "spent_amount": 320000.00
      },
      {
        "category_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
        "category_name": "Transport",
        "limit_amount": 200000.00,
        "spent_amount": 75000.00
      }
    ],
    "timestamp": "2026-06-18T10:00:00Z"
  }
  ```
  > **Catatan:** `spent_amount` adalah total pengeluaran di kategori tersebut selama bulan berjalan dari tanggal yang diberikan. FE bisa menghitung persentase penggunaan dengan `(spent_amount / limit_amount) * 100`.

---

#### Create Budget Limit
- **Endpoint:** `POST /api/v1/budgets`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "category_id": "uuid string", // required
    "limit_amount": 500000.00,    // required, harus > 0
    "period": "monthly"           // required, nilai: "weekly" atau "monthly"
  }
  ```
- **Success Response** (`201 Created`): Mengembalikan objek budget yang baru dibuat.
- **Error Responses:**
  - `409 Conflict`: Budget untuk kategori ini sudah ada.

---

#### Get Budget By ID
- **Endpoint:** `GET /api/v1/budgets/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Mengembalikan satu objek budget limit.

---

#### Update Budget Limit
- **Endpoint:** `PUT /api/v1/budgets/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "limit_amount": 600000.00, // required, harus > 0
    "period": "monthly",       // required, "weekly" atau "monthly"
    "is_active": true          // optional, untuk menonaktifkan/mengaktifkan budget
  }
  ```
- **Success Response** (`200 OK`): Mengembalikan objek budget yang sudah diupdate.

---

#### Delete Budget
- **Endpoint:** `DELETE /api/v1/budgets/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`204 No Content`): Response body kosong (hard delete).

---

### 5.6. Recurring Transactions Config

#### Get All Recurring Configs
- **Endpoint:** `GET /api/v1/recurrings`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Recurring configs retrieved successfully",
    "data": [
      {
        "id": "uuid string",
        "user_id": "uuid string",
        "category_id": "uuid string",
        "amount": 100000.00,
        "frequency": "monthly",
        "start_date": "2026-01-25T00:00:00Z",
        "next_due_date": "2026-07-25T00:00:00Z",
        "created_at": "2026-01-01T10:00:00Z",
        "updated_at": "2026-06-01T10:00:00Z",
        "category": {
          "id": "uuid string",
          "name": "Subscription",
          "icon": "ic_subscription"
        }
      }
    ],
    "timestamp": "2026-06-18T10:00:00Z"
  }
  ```
  > **Catatan:** Recurring config **tidak memiliki field `merchant` dan `note`**. Ketika cron job mengeksekusi transaksi berulang, merchant di-generate otomatis dari nama kategori. Tampilkan `category.name` sebagai label utama di UI.

---

#### Create Recurring Config
- **Endpoint:** `POST /api/v1/recurrings`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "category_id": "uuid string",       // required
    "amount": 100000.00,                // required, harus > 0
    "frequency": "monthly",             // required, nilai: "weekly", "monthly", "yearly"
    "start_date": "2026-07-01T00:00:00Z" // required, ISO 8601
  }
  ```
- **Success Response** (`201 Created`): Mengembalikan objek recurring config yang baru dibuat.

---

#### Get Recurring By ID
- **Endpoint:** `GET /api/v1/recurrings/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`): Mengembalikan satu objek recurring config.

---

#### Update Recurring Config
- **Endpoint:** `PUT /api/v1/recurrings/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`, `Content-Type: application/json`
- **Request Body:**
  ```json
  {
    "amount": 150000.00,                 // required, harus > 0
    "frequency": "monthly",              // required
    "start_date": "2026-08-01T00:00:00Z" // optional
  }
  ```
- **Success Response** (`200 OK`): Mengembalikan objek recurring config yang sudah diupdate.

---

#### Delete Recurring Config
- **Endpoint:** `DELETE /api/v1/recurrings/:id`
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`204 No Content`): Response body kosong (hard delete).

---

### 5.7. Reports & Analytics

#### Expenses By Category
- **Endpoint:** `GET /api/v1/reports/expenses`
- **Description:** Mengembalikan ringkasan pengeluaran yang dikelompokkan per kategori untuk bulan dan tahun tertentu. Cocok untuk pie chart atau analytics.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `month` (int, Required): Bulan (1-12)
  - `year` (int, Required): Tahun (contoh: 2026)
  - `group_by` (string, Optional, default: `category`): Saat ini hanya mendukung nilai `category`.
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Reports retrieved",
    "data": [
      {
        "category_id": "uuid string",
        "category_name": "Food & Drink",
        "total_amount": 800000.00
      },
      {
        "category_id": "uuid string",
        "category_name": "Transport",
        "total_amount": 150000.00
      }
    ],
    "timestamp": "2026-06-18T10:00:00Z"
  }
  ```

---

#### Get Budget Summary (Remaining Limits)
- **Endpoint:** `GET /api/v1/reports/budget-summary`
- **Description:** Mengembalikan tampilan gabungan antara setiap budget aktif dengan jumlah yang telah dibelanjakan.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Budget summary retrieved successfully",
    "data": [
      {
        "category_id": "uuid string",
        "limit_amount": 500000.00,
        "spent_amount": 250000.00,
        "remaining_budget": 250000.00,
        "period": "monthly"
      }
    ],
    "timestamp": "2026-06-18T10:00:00Z"
  }
  ```

---

#### Get Daily Calendar Totals
- **Endpoint:** `GET /api/v1/reports/daily-calendar`
- **Description:** Mengembalikan total pengeluaran yang dikelompokkan per hari untuk bulan dan tahun tertentu. Cocok untuk tampilan kalender di halaman utama.
- **Auth Required:** Yes
- **Headers:** `Authorization: Bearer <token>`
- **Query Parameters:**
  - `month` (int, Required): Bulan 1-12
  - `year` (int, Required): Tahun, contoh: 2026
- **Success Response** (`200 OK`):
  ```json
  {
    "status": "success",
    "message": "Daily calendar retrieved successfully",
    "data": [
      {
        "day": "2026-06-01T00:00:00Z",
        "total_amount": 45000.00
      },
      {
        "day": "2026-06-02T00:00:00Z",
        "total_amount": 120000.00
      }
    ],
    "timestamp": "2026-06-18T10:00:00Z"
  }
  ```
  > **Catatan:** Hanya hari yang memiliki transaksi yang akan muncul di array `data`. Hari tanpa transaksi tidak akan disertakan.

---

## 6. Ringkasan Semua Endpoint

| Grup | Method | Endpoint | Auth | Deskripsi |
|---|---|---|---|---|
| **System** | `GET` | `/api/v1/ping` | No | Health check |
| **Auth** | `POST` | `/api/v1/auth/google` | No | Google SSO Login / Register |
| **Auth** | `POST` | `/api/v1/auth/logout` | Yes | Invalidasi JWT (Redis Blacklist) |
| **User** | `GET` | `/api/v1/users/me` | Yes | Profil user yang sedang login |
| **Transactions** | `GET` | `/api/v1/transactions` | Yes | Daftar transaksi (+ filter, search, sort) |
| **Transactions** | `POST` | `/api/v1/transactions` | Yes | Buat transaksi manual |
| **Transactions** | `PUT` | `/api/v1/transactions/:id` | Yes | Update transaksi |
| **Transactions** | `DELETE` | `/api/v1/transactions/:id` | Yes | Hapus transaksi |
| **Export** | `GET` | `/api/v1/transactions` | Yes | Export CSV (gunakan `Accept: text/csv`) |
| **Categories** | `GET` | `/api/v1/categories` | Yes | Daftar semua kategori |
| **Categories** | `POST` | `/api/v1/categories` | Yes | Buat kategori baru |
| **Categories** | `GET` | `/api/v1/categories/:id` | Yes | Detail satu kategori |
| **Categories** | `PUT` | `/api/v1/categories/:id` | Yes | Update kategori |
| **Categories** | `DELETE` | `/api/v1/categories/:id` | Yes | Hapus kategori |
| **Budgets** | `GET` | `/api/v1/budgets` | Yes | Daftar semua budget |
| **Budgets** | `POST` | `/api/v1/budgets` | Yes | Buat budget baru |
| **Budgets** | `GET` | `/api/v1/budgets/:id` | Yes | Detail satu budget |
| **Budgets** | `PUT` | `/api/v1/budgets/:id` | Yes | Update budget |
| **Budgets** | `DELETE` | `/api/v1/budgets/:id` | Yes | Hapus budget |
| **Budgets** | `GET` | `/api/v1/budgets/progress` | Yes | Progres spent vs limit |
| **Recurrings** | `GET` | `/api/v1/recurrings` | Yes | Daftar recurring config |
| **Recurrings** | `POST` | `/api/v1/recurrings` | Yes | Buat recurring config |
| **Recurrings** | `GET` | `/api/v1/recurrings/:id` | Yes | Detail satu recurring |
| **Recurrings** | `PUT` | `/api/v1/recurrings/:id` | Yes | Update recurring config |
| **Recurrings** | `DELETE` | `/api/v1/recurrings/:id` | Yes | Hapus recurring config |
| **Reports** | `GET` | `/api/v1/reports/expenses` | Yes | Pengeluaran per kategori |
| **Reports** | `GET` | `/api/v1/reports/budget-summary` | Yes | Budget summary (limit vs spent) |
| **Reports** | `GET` | `/api/v1/reports/daily-calendar` | Yes | Total harian per bulan |

---

*End of Document — WalletX API v1*
*Last Updated: 2026-06-18*
