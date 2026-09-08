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

## Authentication untuk endpoint protected

Simpan JWT secara aman di client dan kirimkan pada setiap endpoint protected:

```http
Authorization: Bearer <internal-jwt>
```

JWT yang tidak valid, kedaluwarsa, atau sudah di-logout menghasilkan `401 Unauthorized`.
Blacklist token saat ini disimpan di memory proses API; token
akan hilang ketika proses restart.

## Status dan error

- `200 OK` — request berhasil.
- `201 Created` — resource baru berhasil dibuat.
- `204 No Content` — request berhasil tanpa body.
- `400 Bad Request` — input tidak valid.
- `401 Unauthorized` — token tidak ada atau tidak valid.
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
