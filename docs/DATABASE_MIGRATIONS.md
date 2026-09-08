# Supabase, sqlc, dan Database Migration

Dokumen ini menjelaskan workflow database WalletX setelah migrasi dari GORM ke
sqlc.

## Prinsip utama

    supabase/migrations/*.sql
            ├── supabase db reset  -> PostgreSQL local (Docker)
            └── supabase db push    -> PostgreSQL Supabase production

    Go API -> DATABASE_URL -> PostgreSQL local atau production
    Query SQL -> sqlc generate -> internal/platform/database/sqlc/*.go

- Migration SQL adalah sumber kebenaran schema.
- Query bisnis ditulis di internal/platform/database/queries.
- sqlc menghasilkan typed Go code dari query tersebut.
- API hanya membuka connection pool; API tidak mengubah schema ketika startup.
- Jangan mengedit file generated di internal/platform/database/sqlc.

## Isi schema saat ini

Migration baseline membuat:

- users
- categories
- transactions
- category_limits
- recurring_configs
- view daily_expense_summary

Baseline juga menambahkan primary key, foreign key, index, unique constraint,
check constraint, default timestamp, dan trigger updated_at.

Beberapa perbedaan yang sengaja dirapikan dari schema lama:

- duplicate foreign key pada transactions.category_id dan
  category_limits.category_id dihapus;
- google_id, email, dan kombinasi nama kategori per user dibuat unik;
- category_limits.period memiliki default monthly;
- note, icon, dan flag boolean memiliki default yang konsisten;
- nama recurring dibuat eksplisit recurring_configs, tidak bergantung pada
  konvensi nama struct GORM.

## Instalasi tool

Supabase CLI dan Docker harus tersedia di PATH. Instal sqlc dengan versi yang
dipakai repository:

    go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0

Pastikan:

    supabase --version
    sqlc version
    docker version

## Setup database local

Jalankan dari root repository:

    supabase start
    supabase status

supabase/config.toml mengatur database local pada port 54322, API Supabase
pada 54321, dan Studio pada 54323. Tidak diperlukan docker-compose.yml
tambahan; Supabase CLI yang mengelola container-container tersebut.

Salin environment template:

    Copy-Item .env.example .env

Connection string local yang dipakai API:

    DATABASE_URL=postgresql://postgres:postgres@127.0.0.1:54322/postgres?sslmode=disable

Terapkan seluruh migration dari awal:

    supabase db reset
    sqlc generate
    go test ./...
    go run ./cmd/server

supabase db reset hanya menghapus database local, kemudian menjalankan semua
file di supabase/migrations secara berurutan.

## Kondisi baseline migration

File migration pertama bernama
20260908000100_reset_and_create_walletx_schema.sql. Migration tersebut
memiliki DROP TABLE ... CASCADE untuk membersihkan tabel aplikasi lama,
kemudian membuat schema yang konsisten.

Baseline ini destruktif terhadap tabel aplikasi yang disebutkan di dalamnya.
Sebelum push production, buat backup jika masih ada data yang ingin
dipertahankan. Jangan mengubah migration ini setelah sudah tercatat di
production; migration berikutnya harus dibuat sebagai file baru.

## Workflow perubahan schema

Contoh menambah kolom:

    supabase migration new add_transaction_source

Edit file SQL yang baru dibuat:

    alter table public.transactions
    add column if not exists source text not null default 'manual';

Validasi local:

    supabase db reset
    sqlc generate
    go test ./...

Setelah review dan commit ke Git, terapkan ke production dengan:

    supabase db push

Untuk query baru atau query yang berubah, edit file SQL di
internal/platform/database/queries, lalu jalankan kembali sqlc generate.

## Koneksi ke Supabase production

Login dan link repository ke project Supabase:

    supabase login
    supabase link --project-ref <PROJECT_REF>
    supabase migration list

Project ref dapat dilihat dari URL project Supabase atau dashboard. Jangan
menaruh database password di config.toml, migration, source code, atau Git.

Terapkan migration:

    supabase db push

Perintah tersebut menggunakan project yang sudah di-link dan mencatat
migration yang telah diterapkan. Setelah database siap, set environment API
production pada provider deployment:

    DATABASE_URL=<production-postgres-connection-string>

Untuk runtime Vercel, gunakan connection pooler Supabase yang sesuai untuk
serverless. Untuk migration, gunakan Supabase CLI/project link, bukan koneksi
pooler transaction sebagai pengganti migration runner.

Jika password memiliki karakter khusus seperti @, password harus di-URL-encode
di connection string, misalnya @ menjadi %40. Jangan menyalin connection string
asli ke dokumentasi.

Urutan deployment yang disarankan:

    1. Buat dan test migration di local
    2. Commit migration + query + hasil generate sqlc
    3. supabase db push
    4. Deploy API Go

Untuk perubahan breaking, gunakan pola expand-contract: tambah struktur baru,
deploy kode yang kompatibel, backfill data, lalu hapus struktur lama pada
migration berikutnya.

## Environment API

Minimal untuk API:

    PORT=8080
    DATABASE_URL=...
    JWT_SECRET=...
    REDIS_URL=...
    TELEGRAM_BOT_TOKEN=...

Environment lain seperti Google OAuth, IMAP, SMTP, Gemini, dan CRON_SECRET
hanya diperlukan oleh fitur masing-masing. DB_HOST, DB_PORT, DB_USER,
DB_PASSWORD, DB_NAME, dan DB_SSLMODE sudah tidak digunakan; semuanya digantikan
oleh DATABASE_URL.

SUPABASE_PUBLISHABLE_KEY juga tidak diperlukan oleh API database ini karena API
menggunakan koneksi PostgreSQL langsung melalui pgx. Key tersebut baru
diperlukan jika aplikasi memakai Supabase Auth, REST, Realtime, atau Storage
client.

## Supabase Auth vs auth aplikasi

Migration ini tidak mengubah authentication flow. WalletX masih memakai Google
ID token lalu menerbitkan JWT internal. Supabase Database dan Supabase Auth
adalah dua hal terpisah; Supabase Auth dapat diadopsi nanti melalui migration
dan refactor auth service jika memang diperlukan.

## Troubleshooting

### DATABASE_URL is not set

Pastikan .env berada di root repository dan berisi DATABASE_URL. Untuk local,
pastikan supabase start sudah sukses.

### sqlc generate gagal

Periksa SQL migration dan query agar schema dapat diparse. Setelah mengubah
schema, jalankan supabase db reset terlebih dahulu, lalu jalankan sqlc generate.

### supabase db push menolak migration history

Jangan menggunakan force atau menghapus tabel migration history. Jalankan
supabase migration list, cocokkan migration local dan remote, lalu buat
baseline yang benar atau lakukan rekonsiliasi secara eksplisit.

### Docker local berhenti

    supabase stop
    supabase start

