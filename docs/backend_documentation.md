# Backend Documentation

Backend ini adalah starter REST API berbasis Go, Gin, PostgreSQL, `pgx`, dan
`sqlc`. Baseline hanya mengaktifkan authentication sehingga struktur tetap
ringan dan siap dikembangkan.

## Komponen

- `api` dan `cmd/server`: entry point deployment dan lokal.
- `configs`: loader environment.
- `internal/app`: composition root dan router.
- `internal/modules/auth`: Google authentication, JWT, dan persistence user.
- `internal/middleware`: CORS, JWT guard, dan recovery.
- `internal/platform/database`: connection pool, SQL source, dan generated
  queries.
- `internal/platform/logger`: logger adapter.
- `internal/shared`: response, request, pagination, dan error utilities.

## Local development

```powershell
Copy-Item .env.example .env
supabase start
supabase db reset
sqlc generate
go test ./...
go run ./cmd/server
```

## Route summary

- `GET /api/v1/ping`
- `POST /api/v1/auth/google`
- `GET /api/v1/auth/google/test-login` ketika `DEV_MODE=true`
- `GET /api/v1/auth/google/callback` ketika `DEV_MODE=true`
- `POST /api/v1/auth/logout` dengan JWT

## Prinsip perubahan

Migration SQL dan query SQL menjadi sumber kebenaran. Setelah mengubahnya,
regenerate `sqlc` dan jalankan test. File generated tidak diedit manual.

Dokumentasi API ada di [API_DOCUMENTATION.md](API_DOCUMENTATION.md), sedangkan
workflow database ada di [DATABASE_MIGRATIONS.md](DATABASE_MIGRATIONS.md).
