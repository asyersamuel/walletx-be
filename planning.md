# WalletX Users API Planning

## [x] Master Checklist: Authentication Google OAuth

- [x] Menetapkan kontrak tunggal `POST /api/v1/auth/google` untuk login dan register.
- [x] Memvalidasi Google `id_token` menggunakan `google.golang.org/api/idtoken` dengan OAuth client ID dari konfigurasi.
- [x] Memastikan claim wajib (`sub`, `email`, `name`, dan `email_verified`) tervalidasi sebelum mengakses database.
- [x] Menambahkan alur create user melalui fungsi users yang membuat user dan default categories dalam satu transaksi atomik. *(categories BLOCKED: schema belum ada; transaction infra sudah siap)*
- [x] Menangani user lama berdasarkan `google_id` dan memperbarui `name`/`picture` dari Google bila berubah.
- [x] Menolak konflik antara `email` dan identitas Google yang berbeda tanpa membuat data parsial.
- [x] Membuat internal JWT hanya setelah verifikasi Google dan operasi database berhasil.
- [ ] Menulis unit test untuk token valid, token invalid, token expired, claim wajib tidak ada, konflik email, user baru, user lama, dan rollback transaksi.
- [ ] Menguji endpoint `POST /api/v1/auth/google` melalui Bruno: token valid user baru menghasilkan `201`, token valid user lama menghasilkan `200`, token expired menghasilkan `401`, dan payload kosong/malformed menghasilkan `400`.
- [ ] Menguji retry token valid tidak membuat user atau default categories duplikat.
- [x] Menjalankan `gofmt`, `go test ./...`, dan `go vet ./...` setelah fase implementasi selesai.

## Current Phase & Objective

### Phase: Authentication Contract and Google OAuth Design

**Objective:** Menetapkan desain implementasi autentikasi berbasis Google OAuth di
modul `internal/modules/auth/`. Login dan register sengaja digabung dalam satu
endpoint publik. Authorization/protected resource flow berada di luar scope fase
ini.

Prinsip utama:

- Client hanya mengirim Google `id_token`; client tidak boleh mengirim `google_id`,
  email, atau data user yang dipercaya backend.
- Backend memvalidasi token, mengambil identity/profile dari claim yang tervalidasi,
  lalu mendelegasikan persistence user dan default categories ke modul users.
- Token Google tidak pernah dikembalikan atau dicatat. Client menerima internal
  JWT WalletX setelah seluruh proses berhasil.

## Rincian Task

### Task 1: Route dan Payload

**Route:**

```http
POST /api/v1/auth/google
Content-Type: application/json
```

**Payload:**

```json
{
  "id_token": "<google-id-token>"
}
```

Aturan payload:

1. `id_token` wajib berupa string non-empty.
2. JSON malformed, body kosong, field unknown, atau tipe field salah ditolak.
3. Google access token, authorization header, password, dan `google_id` bukan
   bagian dari kontrak endpoint ini.

### Task 2: Verifikasi Google Token

1. `auth/service.go` menggunakan `google.golang.org/api/idtoken.Validate` dengan
   OAuth client ID dari konfigurasi server sebagai audience.
2. Library harus memvalidasi signature Google, issuer, audience, expiry, dan
   struktur token. Token expired atau signature/audience tidak valid dipetakan
   menjadi `401 Unauthorized`.
3. Pastikan claim `sub`, `email`, dan `name` ada dan memiliki tipe/nilai valid.
   `email_verified` wajib bernilai true sebelum identity dipakai untuk login atau
   register.
4. Normalisasi email untuk lookup/unique check, tetapi gunakan `sub` sebagai
   stable provider identity (`google_id`). Claim `picture` bersifat optional.
5. Error dari provider tidak boleh mengandung atau mengembalikan token kepada
   client. Log hanya dilakukan di boundary dengan logger abstraction dan tanpa
   token, authorization header, atau request body.

### Task 3: Business Logic Register dan Login

1. Cari user aktif berdasarkan `google_id` melalui capability yang disediakan
   modul users; jika perlu, lakukan pengecekan email untuk mendeteksi identity
   conflict.
2. **Register (user baru):** bila `google_id` dan email belum ada, panggil fungsi
   users seperti `CreateWithDefaultCategories` yang menjalankan insert user dan
   insert seluruh default categories dalam satu database transaction. Commit hanya
   jika semua operasi berhasil; rollback jika salah satu operasi gagal.
3. Default categories harus memakai daftar yang telah disetujui product, memiliki
   ownership ke user baru, dan aman terhadap retry/idempotent sehingga tidak ada
   duplikasi akibat request yang diulang.
4. **Login (user lama):** bila user aktif ditemukan berdasarkan `google_id`, update
   `name` dan `picture` hanya bila nilai dari Google berubah. Jangan menerima atau
   mengubah `google_id` dari client.
5. Jika email telah dimiliki user lain atau `google_id` terhubung ke email yang
   berbeda, kembalikan `409 Conflict` dan jangan menghasilkan perubahan parsial.
6. Jika user ditemukan soft-deleted, jangan otomatis mengaktifkan kembali akun;
   kembalikan conflict atau error account-recovery sesuai keputusan produk.
7. Setelah persistence sukses, generate internal JWT WalletX menggunakan secret,
   expiry, `user_id`, `jti`, `iat`, dan claim minimum yang disepakati. JWT dibuat
   setelah commit, bukan sebelum transaction selesai.

### Task 4: Expected Response

**Register berhasil:** `201 Created`.

**Login berhasil:** `200 OK`.

```json
{
  "status": "success",
  "message": "Registration successful",
  "data": {
    "user": {
      "id": "uuid",
      "google_id": "provider-user-id",
      "email": "user@example.com",
      "name": "User Name",
      "picture": "https://example.com/avatar.jpg"
    },
    "token": "<internal-jwt>"
  },
  "timestamp": "timestamp"
}
```

Untuk user lama, message menjadi `Login successful`. Error contract:

- `400 Bad Request`: body/payload invalid atau claim wajib tidak valid secara bentuk.
- `401 Unauthorized`: Google token invalid, expired, audience salah, atau email
  belum terverifikasi.
- `409 Conflict`: email/provider identity conflict atau akun telah dihapus.
- `500 Internal Server Error`: kegagalan database, transaction, atau pembuatan
  internal JWT tanpa membocorkan detail internal.

### Task 5: Struktur Modul dan File yang Dimodifikasi

**Modul `internal/modules/auth/`:**

- `dto.go`: request/response DTO, validasi payload, dan pemisahan claim provider
  dari model internal.
- `handler.go`: strict JSON binding, pemetaan error ke HTTP status, response
  envelope, dan pemanggilan service; tidak berisi business logic persistence.
- `service.go`: verifikasi Google token, orchestration login/register, pemanggilan
  capability users, dan pembuatan internal JWT.
- `routes.go`: registrasi endpoint publik `POST /api/v1/auth/google`; route logout
  atau helper development tidak menjadi bagian alur utama ini.
- `module.go`: wiring service, dependency users, konfigurasi OAuth/JWT, dan logger.
- `repository.go`/`model.go`/`blacklist.go`: hanya dimodifikasi bila kontrak
  persistence atau model auth memang membutuhkan perubahan; jangan membuat
  duplicate persistence logic yang seharusnya berada di users.

**Integrasi modul users dan persistence:**

- Tambahkan narrow interface/capability users untuk lookup berdasarkan Google ID
  atau email serta `CreateWithDefaultCategories` yang menjamin satu transaction.
- Modifikasi repository/query/migration users/categories sesuai schema yang
  diperlukan; transaksi harus dimiliki boundary persistence, bukan handler auth.
- Mapping duplicate/not-found/soft-deleted ke application error dilakukan sebelum
  handler menerjemahkannya menjadi HTTP response.

### Task 6: Skenario Pengujian Bruno

Tambahkan request collection/environment Bruno dengan `baseUrl` dan token Google
yang tidak disimpan di repository. Jalankan kasus berikut terhadap endpoint yang
sama:

1. **Register token valid:** kirim token valid untuk email baru; verifikasi `201`,
   `data.user`, internal JWT, dan user serta default categories tercipta.
2. **Login token valid:** kirim token valid untuk user yang sudah ada; verifikasi
   `200`, profile `name`/`picture` tersinkron, dan tidak ada categories duplikat.
3. **Profile berubah:** kirim token valid dengan nama/foto berbeda; verifikasi row
   user diperbarui sesuai claim Google.
4. **Token expired:** kirim Google token expired; verifikasi `401` dan tidak ada
   insert/update database.
5. **Token invalid atau audience salah:** verifikasi `401` dan response tidak
   membocorkan detail token/provider.
6. **Payload kosong:** kirim `{}`; verifikasi `400`.
7. **Payload malformed atau `id_token` bukan string:** verifikasi `400`.
8. **Email/provider conflict:** verifikasi `409` dan tidak ada data parsial.
9. **Retry register:** ulangi request token valid yang sama; verifikasi hasil
   idempotent dan jumlah default categories tetap satu set.
10. **Database failure/rollback:** simulasikan kegagalan pembuatan category;
    verifikasi user baru juga tidak tersisa setelah request gagal.

## Status Eksekusi

**Status:** EXECUTED (scope: Authentication Google OAuth, Task 1–5) — siap untuk unit test dan pengujian Bruno.

Dieksekusi:

- `internal/shared/errors/errors.go`: tambah `ErrConflict` dan `ErrAccountDeleted`.
- `internal/platform/database/queries/auth.sql`: tambah `GetUserByGoogleIDAny`
  (tanpa filter `deleted_at`) dan `UpdateUserGoogleProfile` (hanya `name`/`picture`);
  regenerate sqlc v1.29.0.
- `internal/modules/auth/dto.go`: tambah `UserResponse`, `GoogleAuthResponse`,
  helper `toUserResponse`; hapus `binding:"required"` dari `GoogleAuthInput`
  (validasi dipindah ke handler dengan strict JSON decoder).
- `internal/modules/auth/repository.go`: tambah `pool *pgxpool.Pool`; implementasi
  `FindByGoogleIDAny`, `UpdateGoogleProfile`, `CreateWithDefaultCategories`
  (transaksi pgx; insert default categories masih TODO/BLOCKED menunggu schema).
- `internal/modules/auth/service.go`: inject `logger.Logger`; validasi claim
  `sub`/`email`/`name`/`email_verified`; normalisasi email (lowercase+trim);
  deteksi akun soft-deleted via `FindByGoogleIDAny`; deteksi konflik email via
  `FindByEmail`; update profile hanya bila `name`/`picture` berubah; map semua
  error Google ke `apperrors.ErrUnauthorized` tanpa membocorkan detail token.
- `internal/modules/auth/handler.go`: strict JSON decoder (`DisallowUnknownFields`);
  validasi `id_token` non-empty; mapping error ke HTTP status
  (400/401/409/500); log di boundary dengan level Warn untuk 4xx dan Error untuk
  5xx; response menggunakan `GoogleAuthResponse` DTO.
- `internal/modules/auth/module.go`: terima `*pgxpool.Pool`, teruskan ke repository
  dan logger ke service+handler.
- `internal/app/modules.go` dan `internal/app/app.go`: teruskan `db` (pool) ke
  `buildModules` dan `auth.NewModule`.
- Verifikasi: `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` → PASS.

Belum dieksekusi (blocked):

- Insert default categories di `CreateWithDefaultCategories` — menunggu migration
  tabel `categories` dan product decision daftar default category.
- Unit test service auth (token valid/invalid/expired, konflik email, rollback).
- Pengujian endpoint via Bruno (Task 6).

## Master Checklist Proyek

- [x] Menyetujui kontrak API users dan keputusan soft delete.
- [ ] Menambahkan migration untuk tabel `categories` dan `transactions` beserta foreign key ke `users`. *(BLOCKED: menunggu product decision daftar default category)*
- [ ] Menentukan daftar dan atribut default category yang bersifat product decision. *(BLOCKED)*
- [x] Menambahkan DTO, validasi, service, repository, dan route untuk operasi users.
- [ ] Menambahkan transaksi database atomik untuk create user dan default categories. *(BLOCKED: tabel categories belum ada)*
- [x] Menambahkan soft-delete untuk user (cascade ke categories/transactions menyusul setelah migration-nya ada).
- [x] Menambahkan invalidasi token setelah penghapusan user.
- [x] Menambahkan unit test service users (repository/integration test menyusul).
- [x] Memperbarui dokumentasi API (`docs/API_DOCUMENTATION.md`).
- [x] Menjalankan `gofmt`, `go test ./...`, dan `go vet ./...`.

## Current Phase

### Phase: API Contract and Data-Lifecycle Design

**Objective:** Menetapkan kontrak HTTP, aturan ownership, validasi, dan lifecycle
data user sebelum implementasi apa pun dimulai.

Kondisi repository saat ini:

- Tabel `users` sudah tersedia dengan kolom `deleted_at`, unique `google_id`, dan
  unique `email`.
- User baru saat ini dibuat oleh `POST /api/v1/auth/google` setelah Google ID
  token tervalidasi. Route ini tetap menjadi jalur Create publik yang canonical;
  tidak menambahkan `POST /users` yang menerima `google_id` mentah.
- Tabel `categories` dan `transactions` belum ada pada migration aktif. Keduanya
  wajib dibuat terlebih dahulu sebelum aturan relasional di bawah dapat diterapkan.
- Semua route users bersifat self-service dan membutuhkan JWT valid. User ID
  selalu diambil dari claim/context authentication, bukan dari body request.

## Rincian Per Task

### Task 1: Create User

**Route:**

```http
POST /api/v1/auth/google
Content-Type: application/json
```

Route ini mewakili operasi Create user. Endpoint user CRUD tidak menyediakan
create anonim atau menerima identitas provider dari client secara langsung.

**Payload:**

```json
{
  "id_token": "<google-id-token>"
}
```

**Business Logic:**

1. Parse JSON dan validasi `id_token` wajib, tidak kosong, dan bertipe string.
2. Validasi token ke Google menggunakan configured client ID. Jangan mencatat
   token, Authorization header, atau request body ke log.
3. Ambil `sub`, `email`, `name`, dan optional `picture` dari claims provider.
   `sub`, `email`, dan `name` wajib tersedia; normalisasi email untuk lookup dan
   unique check.
4. Cari user berdasarkan `google_id`.
5. Jika user belum ada, buat row `users` dan default categories dalam satu
   database transaction. Commit hanya jika seluruh operasi berhasil.
6. Default categories harus idempotent dan dimiliki oleh user tersebut. Nama,
   tipe, warna, dan icon final ditetapkan pada migration/product decision; retry
   tidak boleh membuat duplikasi.
7. Jika email sudah dimiliki user lain, hentikan proses dengan conflict dan jangan
   membuat sebagian data.
8. Jika user sudah ada dan aktif, update field profile yang berasal dari Google
   (`name` dan `picture`) sesuai perilaku authentication saat ini. Jangan
   mengubah `google_id` melalui request client.
9. Jika user ditemukan dengan `deleted_at` terisi, jangan diam-diam menghidupkan
   kembali akun. Kembalikan conflict atau gunakan alur account-recovery yang
   disetujui secara terpisah.
10. Generate JWT hanya setelah transaction berhasil. Error database atau provider
    dikembalikan ke boundary handler dan dicatat di boundary tersebut dengan
    logger abstraction.

**Expected Response:**

- User baru: `201 Created`.
- User existing yang berhasil login/update profile: `200 OK`.
- Body menggunakan response envelope standar:

```json
{
  "status": "success",
  "message": "Registration successful",
  "data": {
    "user": {
      "id": "uuid",
      "google_id": "provider-user-id",
      "email": "user@example.com",
      "name": "User Name",
      "picture": "https://example.com/avatar.jpg",
      "created_at": "timestamp",
      "updated_at": "timestamp"
    },
    "token": "<internal-jwt>"
  },
  "timestamp": "timestamp"
}
```

Error contract:

- `400 Bad Request` untuk JSON atau payload invalid.
- `401 Unauthorized` untuk Google token invalid/expired.
- `409 Conflict` untuk email/provider identity conflict atau akun yang telah
  dihapus.
- `500 Internal Server Error` untuk kegagalan database/provider internal tanpa
  membocorkan detail sensitif.

### Task 2: Edit User Profile

**Route:**

```http
PUT /api/v1/users/me
Authorization: Bearer <internal-jwt>
Content-Type: application/json
```

**Payload:**

```json
{
  "name": "Updated Name",
  "picture": "https://example.com/new-avatar.jpg"
}
```

`name` wajib dan boleh diedit. `picture` optional dan boleh dikosongkan untuk
menghapus avatar. `email`, `google_id`, `id`, `created_at`, `updated_at`, dan
`deleted_at` tidak boleh diterima sebagai field mutable dari client.

**Business Logic:**

1. Pastikan JWT valid dan parse `user_id` sebagai UUID.
2. Reject unknown field, malformed JSON, `name` kosong setelah trim, name di luar
   batas panjang yang disepakati, dan picture yang bukan URL valid atau melebihi
   batas panjang.
3. Ambil user berdasarkan user ID dari JWT dan pastikan `deleted_at IS NULL`.
4. Update hanya field profile yang diizinkan. Database trigger mengisi
   `updated_at`; client tidak boleh mengontrol timestamp.
5. Bila row tidak ditemukan, kembalikan `404`; bila terjadi unique conflict yang
   tidak terduga, kembalikan `409`.
6. Jangan mengubah category atau transaction user pada profile update.

**Expected Response:**

- `200 OK` dengan `data.user` berisi representasi user terbaru dan message
  `User profile updated successfully`.
- `400 Bad Request` untuk validation error.
- `401 Unauthorized` untuk token invalid/expired/revoked.
- `404 Not Found` bila user tidak aktif atau tidak ditemukan.
- `500 Internal Server Error` untuk kegagalan persistence.

### Task 3: Delete User Account

**Route:**

```http
DELETE /api/v1/users/me
Authorization: Bearer <internal-jwt>
```

**Payload:** Tidak ada request body. Jika body dikirim, endpoint tetap tidak
boleh menerima field penghapusan atau user ID dari client.

**Business Logic:**

1. Validasi JWT dan ambil `user_id` dari context authentication.
2. Jalankan satu database transaction dengan locking/guard agar dua request
   delete bersamaan tetap aman.
3. Set `users.deleted_at` dan `updated_at` untuk user aktif. Jangan hard-delete
   pada operasi normal karena data finansial membutuhkan audit/history dan
   `users.deleted_at` memang sudah disediakan schema.
4. Terapkan soft-delete cascade: tandai categories milik user sebagai deleted
   dan sembunyikan/arsipkan transactions milik user sesuai kolom lifecycle yang
   disediakan migration. Semua query feature wajib memfilter data user yang
   sudah dihapus.
5. Foreign key categories dan transactions tetap menjaga ownership ke user.
   Jangan memakai database `ON DELETE CASCADE` pada soft delete karena trigger
   tersebut tidak berjalan saat hanya mengisi `deleted_at`. Hard purge, bila
   dibutuhkan untuk privacy compliance, harus menjadi job/admin flow terpisah
   dengan policy `ON DELETE CASCADE` yang diuji dan disetujui.
6. Setelah commit berhasil, revoke token aktif yang digunakan request. Jika
   requirement mengharuskan seluruh session user direvoke, diperlukan session
   store/token version; blacklist memory saat ini belum cukup untuk itu.
7. Operasi harus idempotent: user yang sudah soft-deleted tidak menghapus data
   ulang dan dapat mengembalikan `204 No Content` tanpa membocorkan informasi.

**Keputusan final (dieksekusi):** delete sepenuhnya idempotent — selalu
`204 No Content` untuk JWT valid, termasuk akun yang sudah soft-deleted.
Tidak ada `404` agar status akun tidak dapat dienumerasi.

**Expected Response:**

- `204 No Content` tanpa response envelope dan tanpa response body setelah commit
  berhasil.
- `401 Unauthorized` untuk token invalid/expired/revoked.
- `500 Internal Server Error` bila transaction gagal; tidak boleh ada partial
  delete.

### Cross-Cutting Implementation Tasks

- Tambahkan schema migration categories/transactions lebih dahulu, termasuk
  `user_id`, ownership index, lifecycle columns, dan foreign key policy.
- Tambahkan query SQL di `internal/platform/database/queries`, lalu generate
  sqlc; jangan mengedit file generated secara manual.
- Pisahkan DTO request dari model database agar field immutable tidak dapat
  diubah lewat mass binding.
- Map database errors ke application errors (`not found`, `duplicate`, dan
  `conflict`) pada repository; handler hanya menerjemahkan ke HTTP response.
- Inject `logger.Logger` ke component yang membutuhkan logging. Hanya logger
  abstraction yang boleh memakai logrus dan log harus bebas dari token, password,
  email lengkap, serta request body.
- Uji unauthorized ownership, invalid payload, duplicate/retry create, rollback
  default category, update deleted user, concurrent delete, dan cascade filter.

## Status Eksekusi

**Status:** EXECUTED (scope: Users API) — siap untuk testing Bruno.

Dieksekusi:

- Query sqlc baru: `UpdateUserProfile`, `SoftDeleteUser`
  (`internal/platform/database/queries/users.sql` → `sqlc generate` v1.29.0).
- Module baru `internal/modules/users/`: `model.go`, `dto.go`
  (`UpdateUserRequest`), `repository.go`, `service.go`, `handler.go`,
  `routes.go`, `module.go`, `service_test.go`.
- Route protected terdaftar di `internal/app/router.go`:
  - `PUT /api/v1/users/me` → `200 OK` envelope `data.user`.
  - `DELETE /api/v1/users/me` → `204 No Content`, idempotent, revoke token aktif.
- Token revocation setelah delete memakai `auth.Service.Logout` melalui
  interface sempit `users.TokenRevoker`.
- Validasi: strict JSON (unknown field ditolak), name wajib/max 100 setelah
  trim, picture null=tidak berubah / `""`=hapus avatar / selain itu wajib URL
  http(s) max 2048.
- `docs/API_DOCUMENTATION.md` diperbarui.
- Verifikasi: `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` → PASS
  (12 unit test service users).

Belum dieksekusi (blocked, butuh keputusan produk):

- Migration `categories` + `transactions` dan default categories saat create
  user (Task 1 poin 5–6) — tabel belum ada di schema aktif.
- Soft-delete cascade ke categories/transactions — menyusul setelah migration.
- Hard purge flow (privacy compliance) — job/admin terpisah.
- Integration test endpoint dan repository test terhadap database nyata.
