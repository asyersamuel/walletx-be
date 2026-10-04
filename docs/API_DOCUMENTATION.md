# API Documentation

Base URL lokal: `http://localhost:8080/api/v1`

API menggunakan response envelope berikut untuk response JSON:

```json
{
  "status": "success",
  "message": "Operation completed successfully",
  "data": {},
  "timestamp": "2026-01-01T00:00:00Z"
}
```

## Endpoint umum

### `GET /ping`

Memastikan server dapat menerima request. Endpoint ini tidak memerlukan token.

```http
GET /api/v1/ping
```

### `POST /auth/google`

Memvalidasi Google ID token, membuat atau memperbarui user, lalu menerbitkan
JWT internal.

```http
POST /api/v1/auth/google
Content-Type: application/json

{
  "id_token": "<google-id-token>"
}
```

Response sukses menggunakan `201 Created` untuk user baru dan `200 OK` untuk
user yang sudah ada. Token tersedia di `data.token`.

```json
{
  "status": "success",
  "message": "Login successful",
  "data": {
    "user": {
      "id": "uuid",
      "google_id": "provider-user-id",
      "email": "user@example.com",
      "name": "User Name",
      "picture": "https://example.com/avatar.jpg",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    },
    "token": "<internal-jwt>"
  },
  "timestamp": "2026-01-01T00:00:00Z"
}
```

### `GET /auth/google/test-login`

Helper OAuth untuk development. Route ini hanya terdaftar jika `DEV_MODE=true`
dan mengarahkan client ke halaman consent provider.

### `GET /auth/google/callback`

Callback helper OAuth untuk development. Route ini hanya terdaftar jika
`DEV_MODE=true`.

### `POST /auth/logout`

Mencatat JWT aktif sebagai revoked token. Endpoint ini membutuhkan header:

```http
Authorization: Bearer <internal-jwt>
```

Response sukses adalah `204 No Content`.

## Users

Semua endpoint users membutuhkan JWT valid:

```http
Authorization: Bearer <internal-jwt>
```

User selalu diambil dari claim token; tidak ada user ID di path atau body.

### `PUT /users/me`

Memperbarui profil user yang sedang login. Hanya `name` dan `picture` yang
boleh diubah. Field lain (`email`, `google_id`, timestamp) ditolak bila
dikirim. `picture` boleh dikosongkan (`""`) untuk menghapus avatar; bila
diisi harus URL `http`/`https` maksimal 2048 karakter. Unknown field
menghasilkan `400`.

```http
PUT /api/v1/users/me
Content-Type: application/json

{
  "name": "Updated Name",
  "picture": "https://example.com/new-avatar.jpg"
}
```

Response sukses `200 OK`:

```json
{
  "status": "success",
  "message": "User profile updated successfully",
  "data": {
    "user": {
      "id": "uuid",
      "google_id": "provider-user-id",
      "email": "user@example.com",
      "name": "Updated Name",
      "picture": "https://example.com/new-avatar.jpg",
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  },
  "timestamp": "2026-01-01T00:00:00Z"
}
```

Error: `400` validasi gagal, `401` token tidak valid, `404` user tidak
ditemukan/sudah dihapus, `500` kegagalan server.

### `DELETE /users/me`

Menghapus akun user yang sedang login secara soft delete (`deleted_at` terisi)
dan me-revoke token yang dipakai pada request. Operasi idempotent: request
ulang tidak mengembalikan error.

```http
DELETE /api/v1/users/me
```

Response sukses `204 No Content` tanpa body. Error: `401` token tidak valid,
`500` kegagalan server.

## Authentication untuk endpoint protected

Simpan JWT secara aman di client dan kirimkan pada setiap endpoint protected:

```http
Authorization: Bearer <internal-jwt>
```

JWT yang tidak valid, kedaluwarsa, atau sudah di-logout menghasilkan `401 Unauthorized`.
Blacklist token saat ini disimpan di memory proses API; token
akan hilang ketika proses restart.

## Transactions

Semua endpoint transaction membutuhkan JWT valid.

```http
POST /api/v1/transactions
GET /api/v1/transactions
GET /api/v1/transactions/:id
PATCH /api/v1/transactions/:id
DELETE /api/v1/transactions/:id
POST /api/v1/transactions/chat
POST /api/v1/transactions/receipt
```

`POST /transactions` menerima body JSON dengan field `category_id`,
`category_name`, `merchant_name`, `amount`, dan `transaction_date`. Field
`receipt_image_url` dan `notes` bersifat opsional. `transaction_date` harus
menggunakan format RFC3339, contohnya `2026-09-20T12:00:00Z`.

`POST /transactions/receipt` menerima upload multipart dengan field file
`receipt`. Endpoint chat sudah dihubungkan ke provider Gemini melalui adapter
generasi teks sinkron. Adapter dasar ini belum menghasilkan tool call;
`AgentService` masih mengembalikan teks fallback untuk respons tanpa tool call.
Endpoint receipt memerlukan implementasi receipt service yang di-inject;
tanpa service tersebut endpoint mengembalikan `503 Service Unavailable`.

## AI advisor dan extraction

Endpoint berikut membutuhkan JWT melalui `Authorization: Bearer <token>`.
User ID selalu diambil dari JWT; field identitas dalam body tidak diterima.
Model chat dapat dikonfigurasi melalui `GEMINI_CHAT_MODEL` (default
`gemini-3.8-flash`). Embedding query memakai model dan dimensi yang sama dengan
index transaction (`gemini-embedding-2`, 768).

### POST /api/v1/ai/chat

Body JSON (maksimum 1 MiB):

```json
{
  "prompt": "Ringkas pengeluaran saya",
  "personalized": true,
  "isThinking": true,
  "history": [
    {"role": "user", "parts": [{"text": "Saya ingin menabung"}]}
  ]
}
```

Response sukses memakai `text/event-stream`. Setiap event segera di-flush:

```text
event: thought
data: {"text":"..."}

event: answer
data: {"text":"**Ringkasan**\n..."}

event: complete
data: {"message":{"role":"assistant","parts":[{"text":"**Ringkasan**\n..."}]}}
```

`thought` dan `answer` berisi delta Markdown mentah. Frontend merendernya dan
menyimpan `complete.message` untuk dikirim kembali dalam `history`, termasuk
`thoughtSignature` yang opaque jika ada. Backend tidak menyimpan history antar
request. Kegagalan setelah stream dimulai menghasilkan terminal
`event: error` dengan `data: {"message":"AI chat failed"}`. Kegagalan sebelum
stream dimulai memakai response JSON dan HTTP status biasa. Disconnect
membatalkan generation. Karena endpoint menggunakan POST, frontend memakai
streaming fetch, bukan native EventSource.

### POST /api/v1/ai/extract

Share-sheet text memakai JSON (maksimum 1 MiB):

```json
{"text":"Bayar kopi 25000 hari ini", "timeZone":"Asia/Jakarta"}
```

Receipt memakai `multipart/form-data` dengan tepat satu file bernama `image`
(maksimum 10 MiB), dan field `timeZone` opsional. MIME dideteksi dari bytes.
Jangan mengirim text dan image bersamaan. `timeZone` harus berupa zona IANA;
default `Asia/Jakarta`. Waktu referensi diisi server. Kategori diambil dari
database milik user dan hasil divalidasi terhadap kategori tersebut.

Response `200 OK` memakai envelope API biasa; `data` berisi:

```json
{
  "amount": 25000,
  "merchant": "Coffee Shop",
  "category_id": "11111111-1111-4111-8111-111111111111",
  "notes": null,
  "date": "2026-10-03"
}
```

Extraction menghasilkan draft; endpoint ini tidak menyimpan transaction.
Body terlalu besar menghasilkan `413`, media type request yang tidak
didukung menghasilkan `415`.

## Status dan error

- `200 OK` — request berhasil.
- `201 Created` — resource baru berhasil dibuat.
- `204 No Content` — request berhasil tanpa body.
- `400 Bad Request` — input tidak valid.
- `401 Unauthorized` — token tidak ada atau tidak valid.
- `404 Not Found` — resource tidak ditemukan.
- `409 Conflict` — konflik data (duplicate).
- `500 Internal Server Error` — kesalahan server.

Contoh error:

```json
{
  "status": "fail",
  "message": "Invalid request body",
  "errors": "...",
  "timestamp": "2026-01-01T00:00:00Z"
}
```
