# WalletX Backend Documentation

WalletX Backend adalah REST API berbasis Go dan Gin untuk autentikasi Google,
transaksi, kategori, budget, recurring transaction, parser email bank, dan
integrasi Telegram.

## Tech stack

- Go 1.25+
- Gin
- PostgreSQL Supabase
- pgx connection pool
- sqlc untuk typed SQL queries
- Supabase CLI + Docker untuk database local
- Redis/Upstash untuk cache dan token blacklist
- Vercel untuk serverless deployment
- Google OAuth 2.0 dan JWT internal

## Struktur repository

    walletx-be/
    ├── api/                         # Entry point Vercel
    ├── cmd/server/                  # Entry point local
    ├── configs/                     # Environment loader
    ├── internal/modules/            # Handler, service, repository per feature
    ├── internal/platform/database/
    │   ├── database.go              # pgxpool runtime connection
    │   ├── convert.go               # Konversi pgtype ke domain type
    │   ├── errors.go                # Mapping error database
    │   ├── queries/                 # SQL source untuk sqlc
    │   └── sqlc/                    # Generated Go code
    ├── supabase/
    │   ├── config.toml              # Supabase local via Docker
    │   ├── migrations/              # Versioned schema
    │   └── seed.sql                 # Optional seed local
    ├── sqlc.yaml                    # Konfigurasi sqlc
    ├── .env.example                 # Environment template
    └── vercel.json                  # Vercel routes dan cron

## Data access

Repository menerima *sqlc.Queries* dan memanggil query yang typed. GORM sudah
dihapus. Struct domain tidak lagi membawa GORM tag.

File generated di internal/platform/database/sqlc tidak boleh diedit manual.
Jika schema berubah, buat migration SQL. Jika query berubah, edit file SQL di
internal/platform/database/queries, lalu jalankan:

    sqlc generate

## Schema

Migration baseline membuat tabel:

- users
- categories
- transactions
- category_limits
- recurring_configs

View daily_expense_summary dipakai untuk agregasi dashboard. Schema juga
memiliki foreign key, index, unique constraint, check constraint, default
timestamp, dan trigger updated_at.

Tabel kategori dan transaksi memakai soft delete melalui deleted_at. Budget dan
recurring config memakai hard delete.

## Authentication

Flow authentication tetap menggunakan Google ID token yang divalidasi oleh
backend, kemudian backend menerbitkan JWT internal. Supabase Database tidak
secara otomatis menggantikan flow ini dengan Supabase Auth.

## Local development

Prasyarat:

- Docker Desktop aktif
- Supabase CLI
- sqlc 1.29.0+
- Go 1.25+

Perintah utama:

    Copy-Item .env.example .env
    supabase start
    supabase db reset
    sqlc generate
    go test ./...
    go run ./cmd/server

API local berjalan pada port 8080. Database Supabase local berjalan pada port
54322.

## Production

Repository harus di-link ke project Supabase:

    supabase login
    supabase link --project-ref <PROJECT_REF>
    supabase migration list
    supabase db push

Set environment production pada Vercel, terutama DATABASE_URL, JWT_SECRET,
REDIS_URL, TELEGRAM_BOT_TOKEN, Google OAuth, SMTP, IMAP, Gemini, dan
CRON_SECRET sesuai fitur yang digunakan.

Migration dijalankan sebelum deploy API. API tidak menjalankan migration saat
startup karena Vercel dapat memiliki beberapa instance dan cold start.

Workflow lengkap ada di docs/DATABASE_MIGRATIONS.md.

## API routes

- GET /api/v1/ping
- POST /api/v1/auth/google
- POST /api/v1/auth/logout
- CRUD /api/v1/transactions
- CRUD /api/v1/categories
- CRUD /api/v1/budgets
- GET /api/v1/budgets/progress
- CRUD /api/v1/recurrings
- GET /api/v1/reports/expenses
- GET /api/v1/reports/daily-calendar
- POST /api/v1/telegram/webhook
- GET /api/v1/telegram/verify
- GET /api/v1/cron/imap
- GET /api/v1/cron/recurring

Protected routes membutuhkan JWT. Endpoint cron membutuhkan CRON_SECRET.
