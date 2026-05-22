package utils

import (
	"testing"

	"github.com/stretchr/testify/assert" // Ini library sakti yang barusan Anda install
)

func TestParseAmount(t *testing.T) {
	// 1. ARRANGE: Kita buat "Tabel Excel" berisi daftar skenario ujian
	scenarios := []struct {
		name           string
		input          string
		expectedAmount float64
		expectedOk     bool
	}{
		// Skenario Sukses (Valid Input)
		{"Format k kecil", "500k", 500000, true},
		{"Format K besar", "50K", 50000, true},
		{"Format titik ribuan", "500.000", 500000, true},
		{"Format koma ribuan", "500,000", 500000, true},
		{"Format angka polos", "500000", 500000, true},
		{"Format desimal pakai k", "1.5k", 1500, true},
		{"Format banyak spasi", "  25k  ", 25000, true},

		// Skenario Gagal (Invalid Input)
		{"Input huruf murni", "ayam goreng", 0, false},
		{"Input kosong", "", 0, false},
		{"Input minus (nggak boleh utang)", "-50k", 0, false},
	}

	// 2. ACT & ASSERT: Robot kita akan me-looping tabel di atas satu per satu
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			// Menjalankan fungsi asli
			amount, ok := ParseAmount(s.input)

			// Menggunakan "testify" untuk mengecek apakah hasilnya sama dengan ekspektasi
			// assert.Equal(t, HARAPAN, HASIL_ASLI, "Pesan error jika gagal")
			assert.Equal(t, s.expectedOk, ok, "Status boolean gagal di skenario ini")

			// Cek nominal hanya jika kita berharap hasilnya sukses
			if s.expectedOk {
				assert.Equal(t, s.expectedAmount, amount, "Nominal hasil parse salah")
			}
		})
	}
}
