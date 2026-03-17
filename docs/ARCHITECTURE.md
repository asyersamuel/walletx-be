# Arsitektur Proyek WalletX Backend

Proyek WalletX Backend dibangun di atas arsitektur _Clean Architecture / Layered Architecture_ (Arsitektur Berlapis). Pendekatan ini memastikan pemisahan masalah (_separation of concerns_), skalabilitas, dan kemudahan pengujian.

## Pola Desain (Design Pattern)

Aplikasi ini dibagi menjadi beberapa lapisan utama:
1. **Router & Handlers (Layer Presentasi)**: Menerima permintaan HTTP (via Gin), memvalidasi input, memanggil service layer, dan mengembalikan respons HTTP.
2. **Services (Layer Bisnis/Logic)**: Berisi inti logika bisnis aplikasi (contoh: validasi logika, manipulasi data sebelum disimpan).
3. **Repository (Layer Akses Data)**: Bertanggung jawab untuk komunikasi langsung dengan basis data menggunakan GORM. Layer ini memisahkan logika kueri DB dari logika bisnis.
4. **Models (Domain/Entitas)**: Representasi dari struktur tabel basis data.

## Direktori dan Fungsinya

- `cmd/server/main.go`: Entry point dari aplikasi. Menyambungkan koneksi DB, inisialisasi modul, dependency injection, cron jobs, dan router.
- `configs/`: Memuat konfigurasi dari fail `.env` atau *environment variables* ke dalam struktur (struct) yang kuat (strongly-typed).
- `internal/`: Kode sumber utama spesifik untuk aplikasi yang tidak boleh di-import oleh proyek lain.
  - `models/`: Definisi entitas GORM seperti `User` dan `Transaction`.
  - `database/`: Menginisialisasi koneksi antarmuka *database* PostgreSQL dan melakukan migrasi auto (AutoMigrate).
  - `repository/`: Layer pembungkus (wrapper) query dasar ke basis data. Memiliki antarmuka (interface) di dalamnya.
  - `services/`: Memproses *business logic*. Terdapat proses seperti autentikasi (`auth_service.go`), manajemen transaksi (`transaction_service.go`), dan parser (pengurai) teks (`parser_service.go`).
  - `handlers/`: Titik masuk HTTP (Controller) yang menggunakan framework Gin.
  - `router/`: Tempat mendaftarkan semua endpoint (`/api/...`) dan menyisipkan *middleware* (seperti CORS atau Autentikasi JWT).
  - `middleware/`: Terdapat penyekat (interceptor) untuk mengecek dan memvalidasi JWT Token pengguna sebelum dieksekusi oleh Handlers.
  - `workers/`: Background Job (Pekerja Latar Belakang). Di sinilah `IMAPWorker` berada, ia berfungsi untuk menghubungkan secara konstan dengan server Email untuk menarik bukti transaksi (Bank) terbaru, yang dipicu menggunakan modul cron.
  - `constants/`, `utils/`: Konstanta global dan fungsi-fungsi utilitas bersama (seperti generator string, waktu).

## Diagram Alur Eksekusi (User HTTP Request)
Client (Mobile App) -> Router (Gin) -> Middleware (Validasi Token JWT) -> Handler -> Service (Business Logic) -> Repository -> PostgreSQL.

## Diagram Alur Eksekusi (Background Worker IMAP)
Cron Job (`robfig/cron/v3`) -> IMAP Worker (Ambil Email Masuk) -> Transaction Service -> Parser Service (Regex Ekstrak Data HTML/Text) -> Repository -> PostgreSQL.
