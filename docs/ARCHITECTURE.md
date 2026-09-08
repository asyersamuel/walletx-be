# Arsitektur WalletX Backend

WalletX menggunakan Clean Architecture berlapis:

1. **Router/Handler** menerima request Gin dan mengembalikan response HTTP.
2. **Service** menjalankan aturan bisnis.
3. **Repository** memanggil query yang digenerate oleh `sqlc`.
4. **SQL migration** di `supabase/migrations` menjadi sumber kebenaran schema.
5. **Supabase PostgreSQL** menyimpan data aplikasi.

## Alur request

```text
Mobile App
  -> Gin Router
  -> Middleware JWT
  -> Handler
  -> Service
  -> Repository
  -> sqlc-generated Queries
  -> pgx connection pool
  -> Supabase PostgreSQL
```

GORM sudah tidak digunakan. File `internal/platform/database/sqlc/*.go` adalah
hasil generate dan tidak boleh diedit manual. Query sumbernya berada di
`internal/platform/database/queries/*.sql`.

## Struktur penting

```text
walletx-be/
├── api/                         # Entry point Vercel
├── cmd/server/                  # Entry point local
├── configs/                     # Environment loader
├── internal/
│   ├── modules/                 # Handler, service, repository per fitur
│   └── platform/database/
│       ├── database.go          # pgxpool runtime connection
│       ├── convert.go           # Mapping UUID/time pgtype
│       ├── errors.go            # Mapping error PostgreSQL
│       ├── queries/             # Query SQL untuk sqlc
│       └── sqlc/                # Generated Go code
├── supabase/
│   ├── config.toml              # Supabase local via Docker
│   ├── migrations/              # Schema versioning
│   └── seed.sql                 # Seed opsional untuk local
├── sqlc.yaml                    # Konfigurasi code generation
└── .env.example                 # Template environment tanpa secret
```

## Database dan migration

Supabase CLI menjalankan PostgreSQL lokal beserta service Supabase melalui
Docker. API local terhubung ke port database `54322`; API production terhubung
ke `DATABASE_URL` production. API tidak menjalankan migration ketika startup.

Workflow lengkap tersedia di [DATABASE_MIGRATIONS.md](DATABASE_MIGRATIONS.md).

## Catatan schema

- `users`, `categories`, `transactions`, `category_limits`, dan
  `recurring_configs` adalah tabel aplikasi.
- `daily_expense_summary` adalah view untuk kebutuhan dashboard.
- Delete kategori dan transaksi bersifat soft delete melalui `deleted_at`.
- Delete budget dan recurring config bersifat hard delete.
- Constraint unik dan check constraint didefinisikan di SQL migration, bukan di
  struct Go.
