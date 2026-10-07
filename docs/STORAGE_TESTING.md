# Testing storage dengan Bruno

Untuk menguji upload receipt, ekstraksi AI, dan GET/PATCH/DELETE transaksi,
gunakan [panduan flow transaksi](TRANSACTION_TESTING.md). Endpoint di dokumen
ini hanya untuk diagnosis storage.

Endpoint development ini memanggil adapter yang dipilih melalui konfigurasi,
tanpa membuat transaksi atau menjalankan ekstraksi AI. File benar-benar disimpan
di storage, jadi hapus file uji setelah selesai. Route hanya terdaftar ketika
`DEV_MODE=true` dan tetap menggunakan middleware JWT WalletX.

## Konfigurasi

Isi `.env` lokal lalu restart server:

```dotenv
DEV_MODE=true
RECEIPT_STORAGE_PROVIDER=supabase
SUPABASE_URL=https://<project-ref>.supabase.co
SUPABASE_SERVICE_ROLE_KEY=<service-role-key>
RECEIPT_STORAGE_TIMEOUT=30s
```

Bucket `receipts` harus sudah tersedia. Service-role key hanya berada di backend;
request Bruno menggunakan JWT aplikasi WalletX.

Variabel environment Bruno:

| Nama | Isi |
|---|---|
| `base_url` | `http://localhost:8080` |
| `token` | JWT WalletX dari response login `data.token` |
| `receipt_path` | Salin `data.path` dari response upload |

Untuk login, gunakan `POST {{base_url}}/api/v1/auth/google` dengan JSON
`{"id_token":"<Google ID token yang valid>"}`. Alternatif development:
buka `http://localhost:8080/api/v1/auth/google/test-login` di browser dan ikuti
Google OAuth; response callback menghasilkan JWT WalletX.

## Upload

- Method: `POST`
- URL: `{{base_url}}/api/v1/dev/storage/receipts`
- Header: `Authorization: Bearer {{token}}`
- Body: `multipart/form-data`

| Key | Type | Value |
|---|---|---|
| `receipt` | File | Pilih foto struk JPEG, PNG, GIF, atau WebP, maksimal 10 MiB |

Biarkan Bruno membuat `Content-Type` multipart beserta boundary. Jangan mengisi
`Content-Type: application/json` atau `image/png` secara manual untuk request ini.
Backend mendeteksi MIME dari isi file. User ID diambil dari JWT.

Response `201 Created` (field `timestamp` dihilangkan dari contoh):

```json
{
  "status": "success",
  "message": "Receipt uploaded successfully",
  "data": {
    "path": "<user-uuid>/<object-uuid>_<timestamp>.png"
  }
}
```

Simpan `data.path` ke variabel `receipt_path`. Periksa bahwa object muncul di
Supabase Dashboard → Storage → receipts → folder user UUID. Response adapter
tetap berupa path relatif.

## Hapus

- Method: `DELETE`
- URL: `{{base_url}}/api/v1/dev/storage/receipts`
- Headers: `Authorization: Bearer {{token}}`, `Content-Type: application/json`
- Body: JSON

```json
{
  "path": "{{receipt_path}}"
}
```

Response `200 OK`:

```json
{
  "status": "success",
  "message": "Receipt deleted successfully",
  "data": {
    "path": "<user-uuid>/<object-uuid>_<timestamp>.png"
  }
}
```

Gunakan path hasil upload dan JWT user yang sama. Adapter menolak path milik
user lain, full URL, dan traversal. Jangan memakai endpoint ini untuk file yang
sudah direferensikan oleh transaksi; endpoint ini tidak mengubah database.

## Skenario pengujian

| Skenario | Expected status |
|---|---|
| Upload valid | `201` |
| Delete valid; mengulang delete object yang sudah tidak ada | `200` |
| JWT hilang atau tidak valid | `401` |
| File kosong, field receipt hilang, JSON delete tidak valid, path user lain | `400` |
| Upload terlalu besar | `413` |
| Isi file bukan image yang didukung | `415` |
| Provider storage gagal | `500`, pesan generik |
| Storage timeout | `504` |
| `DEV_MODE=false` | `404`, route tidak terdaftar |

Flow produksi tetap tersedia melalui `POST /api/v1/transactions/receipt` dengan
body multipart `receipt`; flow tersebut juga menyimpan transaksi dan menjalankan
ekstraksi AI.
