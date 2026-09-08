# Arsitektur Backend

Repository ini menggunakan struktur berlapis yang dapat diperluas:

1. Router menerima request melalui Gin.
2. Middleware menangani concern lintas fitur seperti CORS dan JWT.
3. Handler menerjemahkan request dan response HTTP.
4. Service menjalankan aturan bisnis.
5. Repository mengakses database melalui query typed hasil generate `sqlc`.
6. Platform menyediakan koneksi database dan logger.

## Alur authentication

```text
Client
  -> Gin Router
  -> Auth Handler
  -> Auth Service
  -> Auth Repository
  -> sqlc Queries
  -> pgx connection pool
  -> PostgreSQL
```

JWT yang diterbitkan oleh service divalidasi oleh middleware untuk route
protected. Auth module dan middleware berbagi interface blacklist token yang
sempit agar storage dapat diganti kemudian.

## Struktur penting

```text
.
├── api/                         # Entry point deployment
├── cmd/server/                  # Entry point lokal
├── configs/                     # Loader environment
├── internal/
│   ├── app/                     # Composition root dan router
│   ├── middleware/              # CORS, JWT, recovery
│   ├── modules/auth/            # Handler, service, repository, model
│   ├── platform/database/
│   │   ├── database.go          # pgxpool runtime connection
│   │   ├── convert.go           # Mapping UUID/time
│   │   ├── errors.go            # Mapping error PostgreSQL
│   │   ├── queries/             # Sumber query SQL
│   │   └── sqlc/                # Generated Go code
│   ├── platform/logger/         # Logger adapter
│   └── shared/                  # Utilities lintas module
├── supabase/migrations/         # Versioned schema
├── sqlc.yaml                    # Konfigurasi sqlc
└── vercel.json                  # Deployment routing
```

`internal/app` adalah composition root: dependency dibuat di sana, lalu
diserahkan ke module. Module tidak membuat koneksi database atau membaca
environment secara langsung.

## Aturan pengembangan

- Migration SQL adalah sumber kebenaran schema.
- Query ditulis di `internal/platform/database/queries`.
- Jalankan `sqlc generate` setelah schema/query berubah.
- Jangan mengedit file generated di `internal/platform/database/sqlc`.
- Tambahkan feature baru sebagai module terpisah dan daftarkan dependency-nya
  di `internal/app`.
