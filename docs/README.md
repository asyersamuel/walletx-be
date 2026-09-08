# WalletX Backend

REST API WalletX dibangun dengan Go, Gin, PostgreSQL Supabase, `pgx`, dan
`sqlc`. Schema database dikelola menggunakan Supabase migration dan database
local dijalankan melalui Docker.

## Menjalankan local

Prasyarat:

- Go 1.25 atau lebih baru
- Docker Desktop aktif
- Supabase CLI
- `sqlc` versi 1.29.0 atau lebih baru

```powershell
Copy-Item .env.example .env
supabase start
supabase db reset
sqlc generate
go run ./cmd/server
```

API tersedia di `http://localhost:8080`. Detail koneksi, deployment, dan
workflow perubahan schema ada di
[DATABASE_MIGRATIONS.md](DATABASE_MIGRATIONS.md).
