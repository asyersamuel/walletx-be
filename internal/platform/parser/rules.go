package parser

import "regexp"

// MerchantRule menyimpan pola regex spesifik untuk setiap bank.
// Regex di-compile sekali saat startup (package level) untuk performa optimal.
type MerchantRule struct {
	BankName string
	Regex    *regexp.Regexp
}

// merchantRules adalah daftar aturan yang dicoba secara berurutan.
// Urutkan dari yang paling spesifik ke paling umum.
var merchantRules = []MerchantRule{
	// ─────────────────────────────────────────────────────────────────
	// Mandiri (Livin' by Mandiri) — format: Penerima\nNama Merchant
	// Lebih spesifik: newline-delimited, tanpa titik dua.
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "Mandiri (Livin)",
		Regex:    regexp.MustCompile(`(?i)Penerima\s*\r?\n([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// BNI (wondr by BNI) — format: *Penerima*\nNama Merchant
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "BNI (wondr)",
		Regex:    regexp.MustCompile(`(?i)\*Penerima\*\s*\r?\n([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// BRI (BRImo) — format: Kepada : Nama Merchant
	// \b memastikan 'Kepada' tidak cocok di tengah string base64.
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "BRI (Brimo)",
		Regex:    regexp.MustCompile(`(?i)\bKepada\b\s*:\s*([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// BCA — format: Nama Rekening Tujuan : Nama / Ke Rekening : Nama
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "BCA",
		Regex:    regexp.MustCompile(`(?i)(?:Nama Rekening Tujuan|Ke Rekening)\s*:\s*([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// CIMB Niaga — format: Nama Penerima : Nama Merchant
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "CIMB Niaga",
		Regex:    regexp.MustCompile(`(?i)Nama\s+Penerima\s*:\s*([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// Bank Danamon — format: Kepada/Tujuan : Nama
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "Danamon",
		Regex:    regexp.MustCompile(`(?i)(?:Tujuan\s+Transfer|Tujuan)\s*:\s*([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// Permata Bank — format: Beneficiary Name : Nama
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "Permata Bank",
		Regex:    regexp.MustCompile(`(?i)Beneficiary\s+Name\s*:\s*([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// OCBC NISP — format: Merchant / To : Nama
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "OCBC NISP",
		Regex:    regexp.MustCompile(`(?i)(?:To|Merchant)\s*:\s*([A-Za-z0-9\s\.\-&']+?)(?:\s*\r?\n|$)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// GoPay / OVO / Dana / ShopeePay (E-Wallet umum)
	// \b pada setiap kata kunci mencegah kecocokan di dalam header base64/DKIM.
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "E-Wallet/Umum",
		Regex:    regexp.MustCompile(`(?i)\b(?:ke|kepada|merchant|pembayaran)\b\s*:?\s*([A-Za-z0-9\s]+)(?:Rp|\n|\.|$)`),
	},
}

// amountRegex ultra-resilient: tidak menggunakan \b karena Go RE2 gagal pada
// Non-Breaking Space (\xA0) dan batas tag HTML di email yang di-forward.
// Guard kiri: awal baris (^) ATAU karakter non-kata (spasi, >, :, &nbsp;, dll.)
// sehingga string base64 seperti 'f7Rp7c' atau 'a1Rp2' tetap diabaikan.
// Menangani: " Rp14.000", ">Rp14.000<", "IDR 1.000.000", ": Rp 50.000"
var amountRegex = regexp.MustCompile(`(?im)(?:^|[^\w])(?:Rp|IDR)\s*\.?\s*([\d][\d\.]*)`)
