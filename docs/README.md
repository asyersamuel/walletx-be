# WalletX Backend

WalletX Backend adalah layanan REST API yang dibangun menggunakan [Go](https://golang.org/) dan framework [Gin](https://gin-gonic.com/). Proyek ini berfungsi sebagai inti untuk mengelola autentikasi pengguna dan pemrosesan transaksi dari berbagai sumber.

Salah satu fitur unggulan dari WalletX Backend adalah kemampuannya untuk membaca dan mem-parsing email dari bank (via IMAP) guna mengekstrak informasi transaksi seperti jumlah dan nama merchant secara otomatis.

## Fitur Utama

- **Autentikasi Pengguna**: Manajemen sesi dan akses menggunakan JWT (JSON Web Tokens).
- **Manajemen Transaksi**: Mencatat dan mengkategorikan transaksi pengguna.
- **Automasi Email (IMAP Worker)**: Menarik email masuk secara berkala untuk mem-parsing bukti transaksi pembayaran/transfer.
- **Parsing Cerdas (Regex)**: Secara otomatis mengekstrak nominal (`Rp`, `IDR`) dan nama merchant/penerima dari isi email transaksi.
- **Cron Job**: Scheduler latar belakang untuk menjalankan worker secara otomatis.

## Prasyarat

- Go 1.25.0 atau lebih baru
- PostgreSQL
- Akun Email dengan akses IMAP diaktifkan (untuk worker)

## Instalasi & Menjalankan Aplikasi

1. **Clone repositori**
   ```bash
   git clone <repo-url>
   cd walletx-be
   ```

2. **Konfigurasi Environment**
   Ganti nama fail atau buat fail `.env` di *root* direktori (lihat `configs/config.go` untuk referensi variabel).
   ```env
   # Contoh Variabel
   SERVER_PORT=8080
   DB_HOST=localhost
   DB_PORT=5432
   DB_USER=postgres
   DB_PASS=password
   DB_NAME=walletx
   JWT_SECRET=supersecret
   IMAP_EMAIL=your_email@gmail.com
   IMAP_PASSWORD=your_app_password
   ```

3. **Jalankan Aplikasi**
   ```bash
   go run cmd/server/main.go
   ```

## Struktur Proyek

Detail mengenai arsitektur perangkat lunak yang digunakan di proyek ini dapat dilihat pada dokumen [ARCHITECTURE.md](ARCHITECTURE.md).
