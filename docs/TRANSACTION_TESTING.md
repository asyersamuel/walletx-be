# Testing flow receipt dan CRUD transaksi dengan Bruno

Gunakan endpoint transaksi untuk menguji flow aplikasi: upload receipt, simpan
gambar ke storage, buat transaksi pending, ekstraksi AI, lalu lengkapi transaksi.
Endpoint `/api/v1/dev/storage/receipts` hanya untuk diagnosis adapter storage dan
bukan flow yang digunakan dalam panduan ini.

## Persiapan

Konfigurasi `.env` backend kemudian restart server:

```dotenv
RECEIPT_STORAGE_PROVIDER=supabase
SUPABASE_URL=https://<project-ref>.supabase.co
SUPABASE_SERVICE_ROLE_KEY=<service-role-key>
RECEIPT_STORAGE_TIMEOUT=30s
GEMINI_API_KEY=<gemini-api-key>
RECEIPT_AI_TIMEOUT=20s
```

Database harus tersedia dan seluruh migration sudah diterapkan, termasuk schema
transaksi pending. Bucket `receipts` harus sudah tersedia. Pakai model Gemini
yang dikonfigurasi aplikasi. User harus memiliki kategori yang bisa dipilih AI.
Endpoint transaksi bekerja baik dengan `DEV_MODE=true` maupun `false`.

Di Bruno siapkan variabel berikut:

| Variabel | Isi |
|---|---|
| `base_url` | `http://localhost:8080` |
| `token` | JWT aplikasi WalletX untuk user A |
| `other_token` | JWT aplikasi WalletX untuk user B, untuk uji ownership |
| `transaction_id` | Salin `data.transaction_id` dari response upload receipt |
| `receipt_path` | Salin `data.receipt_image_url` dari response upload receipt |
| `category_id` | UUID kategori milik user A, bila ingin mengubah kategori |

Tambahkan header `Authorization: Bearer {{token}}` untuk semua request di bawah.
Jangan kirim service-role key atau Google ID token sebagai bearer request
transaksi. Google ID token hanya digunakan pada request login.

Jika belum memiliki JWT, gunakan:

```http
POST {{base_url}}/api/v1/auth/google
Content-Type: application/json
```

```json
{"id_token":"<Google ID token yang valid>"}
```

Ambil JWT WalletX dari `data.token`. Jangan commit token atau receipt asli ke Git.

## 1. Pastikan kategori user tersedia

```http
GET {{base_url}}/api/v1/categories
```

Tanpa body. Jika kategori kosong, buat kategori dengan JWT user yang sama:

```http
POST {{base_url}}/api/v1/categories
Content-Type: application/json
```

```json
{
  "name": "Makanan",
  "type": "expense"
}
```

Simpan `data.category.id` bila diperlukan untuk PATCH. User baru dengan kategori
kosong tidak dapat menyelesaikan flow receipt ke status success.

## 2. Upload receipt dan jalankan ekstraksi AI

```http
POST {{base_url}}/api/v1/transactions/receipt
```

Body Bruno: **Multipart Form**.

| Key | Type | Value |
|---|---|---|
| `receipt` | File | Foto struk JPEG, PNG, GIF, atau WebP dengan isi yang terbaca AI |

Maksimal image 10 MiB. Handler saat ini juga membatasi total Content-Length
request pada 10 MiB, sehingga pilih file yang ukurannya di bawah batas tersebut
agar multipart overhead ikut tertampung. Biarkan Bruno membuat Content-Type
multipart beserta boundary. User ID berasal dari JWT; field amount, merchant,
dan kategori diambil dari hasil AI.

Contoh response `201 Created` berikut menghilangkan beberapa field transaksi dan
field timestamp agar fokus pada data yang digunakan untuk request berikutnya:

```json
{
  "status": "success",
  "message": "Transaction recorded.",
  "data": {
    "status": "success",
    "transaction_id": "<transaction-uuid>",
    "receipt_image_url": "<user-uuid>/<object-uuid>_<timestamp>.png",
    "transaction": {
      "id": "<transaction-uuid>",
      "merchant_name": "Warung Makan",
      "amount": 35000,
      "category_id": "<category-uuid>",
      "transaction_date": "2026-10-07T00:00:00+07:00",
      "receipt_image_url": "<user-uuid>/<object-uuid>_<timestamp>.png"
    }
  }
}
```

Simpan `data.transaction_id` sebagai `transaction_id`, dan
`data.receipt_image_url` sebagai `receipt_path`. Jangan menggunakan object UUID
atau category UUID sebagai transaction ID.

Untuk otomatisasi, script Post Response berikut menyimpan variabel runtime
tanpa menulis credential ke file environment:

```javascript
bru.setVar("receipt_status", "");
bru.setVar("transaction_id", "");
bru.setVar("receipt_path", "");
const body = res.getBody();
if ([201, 202].includes(res.getStatus()) && body.data) {
  bru.setVar("receipt_status", body.data.status);
  bru.setVar("transaction_id", body.data.transaction_id);
  bru.setVar("receipt_path", body.data.receipt_image_url);
}
```

API script mengacu pada [referensi resmi Bruno](https://github.com/usebruno/bruno-docs/blob/main/testing/script/javascript-reference.mdx).

Jika ekstraksi, validasi hasil AI, atau resolusi kategori gagal/timeout, response
adalah `202 Accepted` dengan `data.status=pending` dan `data.transaction=null`.
Image dan row pending tetap tersimpan. Status success di envelope luar berarti
receipt diterima, bukan ekstraksi selesai. Belum ada retry worker; GET tidak
menjalankan ulang ekstraksi dan transaksi pending tidak ditampilkan.

Setelah 201 atau 202, GET detail/list dan DELETE dapat dijalankan. Jika 202,
GET detail menghasilkan 404 dan list tidak memuat transaksi pending. DELETE
tetap dapat menghapus pending beserta receipt milik user. PATCH biasa tetap
hanya untuk transaksi success; PATCH pending menghasilkan 404.

## 3. GET detail transaksi

```http
GET {{base_url}}/api/v1/transactions/{{transaction_id}}
```

Tanpa body, expected `200 OK`. Pastikan:

- `data.id` sama dengan transaction ID dari upload.
- Amount, merchant, kategori, dan tanggal sesuai hasil ekstraksi.
- `data.receipt_image_url` sama dengan path dari upload, bukan public URL.
- Object yang sesuai tersedia di Supabase Dashboard, bucket receipts.

Untuk success, response menyertakan `data.status=success` dan
`receipt_image_url`. GET transaksi pending menghasilkan `404 Not Found`.

## 4. GET list transaksi

```http
GET {{base_url}}/api/v1/transactions?limit=20&offset=0
```

Tanpa body, expected `200 OK`. `data` berupa array transaksi success milik user,
diurutkan berdasarkan tanggal lalu ID secara descending. Pending selalu
dikecualikan sebelum pagination, termasuk ketika tidak ada filter tanggal.

Filter tanggal opsional memakai RFC3339:

```text
start_date=2026-10-01T00:00:00+07:00
end_date=2026-10-31T23:59:59+07:00
```

Masukkan melalui tab query params Bruno agar tanda `+` pada timezone ter-encode.

## 5. PATCH transaksi

```http
PATCH {{base_url}}/api/v1/transactions/{{transaction_id}}
Content-Type: application/json
```

```json
{
  "merchant_name": "Warung Makan - Koreksi",
  "amount": 40000,
  "transaction_date": "2026-10-07T12:30:00+07:00",
  "notes": "Koreksi hasil ekstraksi melalui Bruno"
}
```

Expected `200 OK`. PATCH bersifat partial: kirim field yang ingin diubah saja.
Tahap ini memakai transaksi success (hasil upload 201). Pending belum dapat diedit
melalui PATCH biasa.
Untuk perubahan kategori, tambahkan `"category_id":"{{category_id}}"` dengan
kategori milik user yang sama. Untuk mengosongkan notes, gunakan
`{"clear_notes":true}`. Field id, user_id, dan status tidak tersedia untuk PATCH.

PATCH ini memperbarui data transaksi; tidak mengupload image baru dan tidak
menjalankan ulang ekstraksi AI. `receipt_image_url` dapat diubah sebagai string
referensi, dan `clear_receipt_url=true` mengosongkan referensi database; keduanya
tidak menghapus image lama atau memverifikasi keberadaan object di storage.
Untuk flow utama ini, jangan ubah referensi receipt.

Ulangi GET detail setelah PATCH. Pastikan nilai berubah dan receipt_image_url
tetap sama.

## 6. Uji ownership dan input sebelum menghapus

Dengan JWT user B (`Authorization: Bearer {{other_token}}`), pakai transaction ID
user A:

| Request | Expected |
|---|---|
| GET detail | `404` |
| PATCH dengan payload valid | `404` |
| DELETE | `404`; transaksi user A tetap ada |
| GET list | Transaksi user A tidak muncul |

Dengan JWT user A:

| Input/request | Expected |
|---|---|
| JWT hilang/invalid | `401` |
| UUID transaksi malformed | `400` |
| PATCH `{"amount":0}` atau amount negatif | `400`; nilai lama tetap ada |
| PATCH `{"merchant_name":" "}` | `400` |
| PATCH category_id user B | `404`; kategori lama tetap ada |
| PATCH `{"user_id":"<uuid-user-B>"}` | `400`; field tidak diterima |
| Upload file kosong/field receipt hilang | `400`; tidak ada row transaksi baru |
| Upload isi teks dengan nama receipt.png | `415`; tidak ada row baru |
| Upload terlalu besar | `400` pada endpoint receipt utama |

## 7. DELETE transaksi

```http
DELETE {{base_url}}/api/v1/transactions/{{transaction_id}}
```

Tanpa body. Expected **204 No Content**, response kosong. Tidak menggunakan
payload path seperti endpoint diagnosis storage sebelumnya.

Ulangi GET detail dengan user A: expected `404`. GET list tidak lagi memuat item
tersebut. DELETE kedua dengan ID yang sama juga menghasilkan `404`.

DELETE membaca row milik user, menghapusnya dari database, kemudian mencoba
menghapus image receipt di storage dengan timeout 5 detik. Berlaku untuk pending
dan success. Periksa bahwa object yang sebelumnya disimpan benar-benar hilang
dari bucket receipts. Jika penghapusan database gagal, image tidak dihapus.

Jika cleanup storage gagal, DELETE tetap mengembalikan 204 karena row sudah
terhapus; kegagalan cleanup dicatat pada log aman. Periksa bucket dan log untuk
memastikan cleanup, karena belum ada antrean retry cleanup yang durable.

## Batas hasil pengujian

Panduan ini memverifikasi kontrak API dan ownership transaksi. Pengujian nyata
dengan Bruno tetap membutuhkan server, database, Supabase, Gemini, dan JWT valid.
Bucket receipts saat ini public; JWT melindungi endpoint WalletX, sementara akses
publik image tetap mengikuti konfigurasi bucket. Menguji isolation CRUD database
tidak membuktikan privasi image di bucket public.
