# API Integration Guide

Panduan singkat untuk mengintegrasikan client apa pun dengan backend.

## Base URL

Development default:

```text
http://localhost:8080/api/v1
```

Gunakan base URL deployment pada environment production. Client mobile yang
terhubung ke server lokal biasanya perlu memakai alamat IP LAN komputer host,
bukan `localhost`.

## Login

1. Dapatkan Google ID token menggunakan SDK Google pada client.
2. Kirim token ke `POST /auth/google`.
3. Ambil JWT internal dari `data.token`.
4. Simpan token pada secure storage.

```http
POST /api/v1/auth/google
Content-Type: application/json

{
  "id_token": "<google-id-token>"
}
```

## Request terautentikasi

Kirim JWT pada header berikut:

```http
Authorization: Bearer <internal-jwt>
```

Saat menerima `401 Unauthorized`, hapus token lokal dan minta user login
kembali. Logout dilakukan dengan `POST /auth/logout` menggunakan token yang
sama.

## Endpoint yang tersedia

| Method | Path | Keterangan |
|---|---|---|
| `GET` | `/ping` | Health check |
| `POST` | `/auth/google` | Login/registrasi melalui Google |
| `GET` | `/auth/google/test-login` | Helper OAuth development |
| `GET` | `/auth/google/callback` | Callback OAuth development |
| `POST` | `/auth/logout` | Logout dengan JWT |

Route helper OAuth hanya tersedia ketika `DEV_MODE=true`.

## Format response

Response JSON memakai field `status`, `message`, `data`, dan `timestamp`.
Field `errors` dapat muncul pada response gagal. Logout sukses mengembalikan
`204` tanpa body.
