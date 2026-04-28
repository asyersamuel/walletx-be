package services

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
	// BNI (wondr by BNI) — format: *Penerima*\nNama Merchant
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "BNI (wondr)",
		Regex:    regexp.MustCompile(`(?i)\*Penerima\*\s*\r?\n([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// Mandiri (Livin' by Mandiri) — format: Penerima : Nama / Ke Nama : Nama
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "Mandiri (Livin)",
		Regex:    regexp.MustCompile(`(?i)(?:Penerima|Ke\s+Nama)\s*:\s*([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// BRI (BRImo) — format: Kepada : Nama Merchant
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "BRI (BRImo)",
		Regex:    regexp.MustCompile(`(?i)Kepada\s*:\s*([^\r\n]+)`),
	},

	// ─────────────────────────────────────────────────────────────────
	// BCA — format: Nama Rekening Tujuan : Nama / Ke Rekening : Nama
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "BCA",
		Regex:    regexp.MustCompile(`(?i)(?:Nama\s+Rekening\s+Tujuan|Ke\s+Rekening)\s*:\s*([^\r\n]+)`),
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
	// Format: Ke : Nama / Pembayaran ke : Nama / Merchant : Nama
	// ─────────────────────────────────────────────────────────────────
	{
		BankName: "E-Wallet (GoPay/OVO/Dana/ShopeePay)",
		Regex:    regexp.MustCompile(`(?i)(?:Ke|Kepada|Merchant|Pembayaran\s+ke)\s*:?\s*([A-Za-z0-9\s\.\-&']+?)(?:\s*(?:Rp|IDR)|\r?\n|$)`),
	},
}

// amountRegex mendeteksi nominal uang dalam format Rupiah Indonesia.
// Menangani format: Rp1.000.000 / Rp 1.000.000 / IDR 1.000.000 / Rp1.000.000,00
var amountRegex = regexp.MustCompile(`(?i)(?:Rp|IDR)\.?\s*([\d\.]+(?:,\d{1,2})?)`)