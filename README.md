# Go API Backend Starter

Backend REST API minimal berbasis Go. Repository ini sengaja dipertahankan
sebagai fondasi yang sederhana: boilerplate HTTP, konfigurasi, middleware,
shared utilities, logger, database PostgreSQL melalui `pgx`/`sqlc`, dan module
authentication.

## Ruang lingkup saat ini

- Entry point server lokal di `cmd/server` dan entry point Vercel di `api`.
- Konfigurasi environment di `configs`.
- Module `auth` untuk Google ID token, JWT internal, dan logout.
- `internal/shared` untuk response, request, pagination, dan error umum.
- `internal/platform/database` untuk koneksi PostgreSQL, helper database, query
  SQL, dan code generated `sqlc`.
- Logger dan middleware lintas fitur.

Feature module lain dapat ditambahkan kembali nanti tanpa mengubah struktur
dasar repository.

## Struktur repository

```text
.
├── api/                         # Entry point deployment serverless
├── cmd/server/                  # Entry point server lokal
├── configs/                     # Environment loader
├── internal/
│   ├── app/                     # Application composition dan router
│   ├── middleware/              # CORS dan JWT middleware
│   ├── modules/auth/            # Handler, service, repository authentication
│   ├── platform/database/       # pgx, query source, dan generated sqlc
│   ├── platform/logger/         # Logger adapter
│   └── shared/                  # Utilities lintas module
├── supabase/migrations/         # Sumber kebenaran schema
├── sqlc.yaml                    # Konfigurasi code generation
└── vercel.json                  # Routing deployment
```

## Prasyarat

- Go 1.25 atau lebih baru
- Docker Desktop
- Supabase CLI
- `sqlc` 1.29.0 atau lebih baru

## Menjalankan secara lokal

```powershell
Copy-Item .env.example .env
supabase start
supabase db reset
sqlc generate
go test ./...
go run ./cmd/server
```

API berjalan di `http://localhost:8080`. Database lokal Supabase memakai port
`54322` sesuai `DATABASE_URL` pada `.env.example`.

## Environment minimum

```dotenv
PORT=8080
DATABASE_URL=postgresql://postgres:postgres@127.0.0.1:54322/postgres?sslmode=disable
JWT_SECRET=replace-with-a-long-random-value
JWT_EXPIRATION=720
GIN_MODE=debug
DEV_MODE=false
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REDIRECT_URL=http://localhost:8080/api/v1/auth/google/callback
```

`DEV_MODE=true` mengaktifkan helper OAuth untuk pengujian lokal. Jangan commit
file `.env` atau secret apa pun.

## Endpoint

| Method | Path | Auth |
|---|---|---|
| `GET` | `/api/v1/ping` | Tidak |
| `POST` | `/api/v1/auth/google` | Tidak |
| `GET` | `/api/v1/auth/google/test-login` | Tidak, development |
| `GET` | `/api/v1/auth/google/callback` | Tidak, development |
| `POST` | `/api/v1/auth/logout` | JWT |

Detail request dan response tersedia di
[docs/API_DOCUMENTATION.md](docs/API_DOCUMENTATION.md).

## Alur perubahan database

1. Edit migration SQL di `supabase/migrations`.
2. Edit query SQL di `internal/platform/database/queries` jika diperlukan.
3. Jalankan `supabase db reset` dan `sqlc generate`.
4. Jalankan `go test ./...` sebelum commit.

File di `internal/platform/database/sqlc` dihasilkan otomatis dan tidak boleh
diedit manual. Lihat [docs/DATABASE_MIGRATIONS.md](docs/DATABASE_MIGRATIONS.md)
untuk workflow database yang lebih lengkap.
