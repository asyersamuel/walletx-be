# Database, Supabase, dan sqlc

Database dikelola melalui Supabase CLI dan PostgreSQL. API hanya membuka
connection pool ketika startup; API tidak menjalankan migration secara
otomatis.

## Baseline schema

Baseline saat ini hanya membuat tabel `public.users` untuk authentication:

- `id` sebagai UUID primary key;
- `google_id` dan `email` sebagai nilai unik;
- `name` dan `picture` sebagai profile data;
- `created_at`, `updated_at`, dan `deleted_at` untuk lifecycle record;
- trigger `updated_at` untuk menjaga timestamp perubahan.

Query source yang aktif berada di:

```text
internal/platform/database/queries/auth.sql
internal/platform/database/queries/health.sql
```

Generated code berada di `internal/platform/database/sqlc`. File generated
tidak boleh diedit manual.

## Prasyarat

- Docker Desktop
- Supabase CLI
- `sqlc` 1.29.0+
- Go 1.25+

Install sqlc:

```text
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.29.0
```

## Workflow lokal

Jalankan dari root repository:

```powershell
Copy-Item .env.example .env
supabase start
supabase status
supabase db reset
sqlc generate
go test ./...
go run ./cmd/server
```

`supabase db reset` membangun database lokal dari seluruh file di
`supabase/migrations`. Connection string default API adalah:

```text
postgresql://postgres:postgres@127.0.0.1:54322/postgres?sslmode=disable
```

## Menambah schema

1. Buat migration baru:

   ```text
   supabase migration new add_feature_schema
   ```

2. Edit file SQL migration.
3. Tambahkan atau ubah query di `internal/platform/database/queries`.
4. Jalankan `supabase db reset`.
5. Jalankan `sqlc generate` dan `go test ./...`.
6. Review lalu commit migration, query, dan generated code bersama-sama.

Untuk deployment production, gunakan `supabase migration list` lalu
`supabase db push` setelah migration direview. Jika baseline lama sudah
tercatat di project production, jangan mengedit histori migration secara
langsung; buat migration baru atau rekonsiliasi histori dengan hati-hati.

## Environment database

API memerlukan setidaknya:

```dotenv
DATABASE_URL=...
JWT_SECRET=...
JWT_EXPIRATION=720
```

Password database tidak boleh diletakkan di migration, `config.toml`, source
code, atau dokumentasi.
