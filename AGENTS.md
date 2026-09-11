# Agent Rules

Dokumen ini adalah aturan kerja untuk agent dan developer di repository ini.

## Logger

Semua application log wajib menggunakan abstraction:

```go
import platformlogger "walletx-be/internal/platform/logger"

appLogger := platformlogger.NewLogger()
appLogger.Info("Application started")
appLogger.WithError(err).Error("Operation failed")
```

Aturan wajib:

- Hanya `internal/platform/logger` yang boleh mengimpor `github.com/sirupsen/logrus`.
- Package lain tidak boleh memanggil `logrus`, `log`, `fmt.Print*`, atau logger
  library lain untuk mencatat log.
- Dependency logger harus di-inject sebagai `logger.Logger`; jangan membuat
  logger baru di dalam service, handler, repository, atau middleware.
- Gunakan `WithError` dan `WithFields` untuk context terstruktur.
- Jangan gunakan `logrus.Fatal` atau mematikan process dari package library.
  Catat error lalu return error; keputusan exit hanya boleh dilakukan oleh
  entry point.
- Gin tidak boleh memakai default request logger. Gunakan
  `middleware.RequestLogger` agar request log melewati abstraction yang sama.
- Logger adapter boleh diganti di masa depan tanpa mengubah business module.

## Level log

| Level | Gunakan untuk |
|---|---|
| `Debug` | Detail diagnosis lokal/development yang tidak diperlukan operasi normal. |
| `Info` | Startup/shutdown, konfigurasi non-rahasia, dan request yang selesai normal. |
| `Warn` | Fallback, konfigurasi opsional yang hilang, dependency yang degraded, dan request 4xx. |
| `Error` | Kegagalan startup, panic, database/provider failure, dan request 5xx. |

Gunakan level terendah yang masih cukup untuk memahami kejadian. Error yang
sudah ditangani di boundary tidak perlu dicatat ulang oleh setiap layer.

## Kapan harus mencatat log

### Startup dan shutdown

Catat:

- server mulai menerima request;
- konfigurasi opsional memakai fallback;
- koneksi atau dependency gagal;
- server shutdown normal atau gagal.

Sertakan field seperti `port`, `environment`, atau nama dependency bila aman.
Jangan mencatat connection string atau credential.

### HTTP request

`middleware.RequestLogger` mencatat satu event setelah request selesai dengan
field `method`, `path`, `status`, `latency_ms`, dan `client_ip`.

- Response 2xx/3xx: `Info`.
- Response 4xx: `Warn`.
- Response 5xx: `Error`.

Jangan menambahkan log kedua untuk error 4xx yang memang merupakan input user,
kecuali ada alasan operasional seperti pola abuse yang perlu dianalisis.

### Authentication

Catat keberhasilan atau kegagalan authentication hanya jika dibutuhkan untuk
operasional atau audit. Gunakan event dan identifier yang tidak sensitif.
Jangan pernah mencatat:

- Google ID token;
- JWT atau Authorization header;
- OAuth client secret;
- password;
- request body penuh.

### Database dan dependency eksternal

Saat operasi gagal, log di boundary yang menangani kegagalan dengan
`WithError(err)` dan field context seperti operation atau dependency. Service
boleh mengembalikan error tanpa log jika error tersebut akan dicatat oleh
handler atau entry point.

### Panic

Gunakan `middleware.Recovery`. Panic harus dicatat sebagai `Error` bersama
path, method, dan context yang aman, lalu response ke client harus tetap
generik.

## Data sensitif

Tidak boleh ada secret, token, password, database URL, authorization header,
atau data pribadi lengkap di log. Email dan identifier user juga jangan
dimasukkan sebagai field default kecuali benar-benar diperlukan dan sudah
dipertimbangkan dampaknya.

Log harus berbentuk event yang singkat dan terstruktur. Hindari log berulang,
stack trace buatan sendiri, dan pesan yang hanya mengulang error tanpa context.

## Checklist perubahan

Sebelum membuat perubahan:

1. Inject `logger.Logger` jika component perlu mencatat log.
2. Pastikan import `logrus` hanya ada di `internal/platform/logger`.
3. Pastikan field log tidak mengandung secret atau PII yang tidak perlu.
4. Jalankan `gofmt`, `go test ./...`, dan `go vet ./...`.
