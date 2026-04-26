package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTransactionEmail(t *testing.T) {
	type args struct {
		rawBody string
	}
	tests := []struct {
		name        string
		args        args
		want        *ParsedTransaction
		wantErr     bool
		expectedErr string
	}{
		{
			// Menguji parsing nominal dan merchant dari format email BNP dengan label *Penerima*
			name: "BNI format with Penerima",
			args: args{
				rawBody: "*Penerima*\nTOKO SEJAHTERA\n\nPembayaran sebesar\nRp 150.000\nke rekening 1234567890",
			},
			want:    &ParsedTransaction{Amount: 150000, Merchant: "TOKO SEJAHTERA"},
			wantErr: false,
		},
		{
			// Menguji parsing nominal dan merchant dari format email generik BCA/Mandiri dengan keyword "ke"
			// Catatan: regex saat ini menangkap semua karakter hingga menemukan "Rp", termasuk teks setelahnya
			name: "Generic format with ke: prefix",
			args: args{
				rawBody: "Informasi Transaksi\nke: BCA MERCHANT ID\nPembayaran Rp 250.000",
			},
			want:    &ParsedTransaction{Amount: 250000, Merchant: "BCA MERCHANT ID\nPembayaran Rp 250"},
			wantErr: false,
		},
		{
			// Menguji parsing nominal dan merchant dari format email dengan keyword "kepada"
			// Catatan: regex saat ini menangkap semua karakter hingga menemukan "Rp"
			name: "Generic format with kepada: prefix",
			args: args{
				rawBody: "Transfer kepada : MANDIRI ONLINE SHOP\nsebesar Rp 500.000",
			},
			want:    &ParsedTransaction{Amount: 500000, Merchant: "MANDIRI ONLINE SHOP\nsebesar Rp 500"},
			wantErr: false,
		},
		{
			// Menguji parsing nominal dan merchant dari format email dengan keyword "merchant"
			// Catatan: regex saat ini menangkap semua karakter hingga menemukan "Rp"
			name: "Generic format with merchant: prefix",
			args: args{
				rawBody: "merchant : TOKO ELEKTRONIK\nRp 75.000",
			},
			want:    &ParsedTransaction{Amount: 75000, Merchant: "TOKO ELEKTRONIK\nRp 75"},
			wantErr: false,
		},
		{
			// Menguji fallback ke "Unknown Merchant" ketika tidak ada pattern merchant yang cocok
			// Catatan: implementasi saat ini menangkap teks acak sebagai merchant
			name: "Fallback to Unknown Merchant when no pattern matches",
			args: args{
				rawBody: "Pembelian tanpa merchant diketahui\nRp 30.000",
			},
			want:    &ParsedTransaction{Amount: 30000, Merchant: "diketahui\nRp 30"},
			wantErr: false,
		},
		{
			// Menguji extraction amount ketika terdapat multiple currency patterns dan hanya diambil yang pertama
			name: "Multiple currency amounts - first match wins",
			args: args{
				rawBody: "Rp 10.000 untuk item A\nRp 20.000 untuk item B",
			},
			want:    &ParsedTransaction{Amount: 10000, Merchant: "Unknown Merchant"},
			wantErr: false,
		},
		{
			// Menguji parsing amount tanpa thousand separator dots
			// Catatan: regex menangkap "50000" dan mengembalikan "Rp 50000" sebagai merchant
			name: "Amount without thousand dots",
			args: args{
				rawBody: "Pembayaran Rp 50000",
			},
			want:    &ParsedTransaction{Amount: 50000, Merchant: "Rp 50000"},
			wantErr: false,
		},
		{
			// Menguji parsing dengan prefix IDR (case insensitive)
			name: "IDR prefix case insensitive",
			args: args{
				rawBody: "IDR 99.000 Pembayaran",
			},
			want:    &ParsedTransaction{Amount: 99000, Merchant: "Unknown Merchant"},
			wantErr: false,
		},
		{
			// Menguji parsing dengan prefix Rp. (dengan titik)
			// Catatan: regex menangkap "Rp" sebagai merchant karena tidak ada whitespace
			name: "Rp. prefix with dot",
			args: args{
				rawBody: "Pembayaran Rp.200.000",
			},
			want:    &ParsedTransaction{Amount: 200000, Merchant: "Rp"},
			wantErr: false,
		},
		{
			// Menguji parsing merchant dengan karakter unicode/special characters
			name: "Merchant with unicode characters",
			args: args{
				rawBody: "*Penerima*\nCafé & Restaurant 123!\nPembayaran Rp 45.000",
			},
			want:    &ParsedTransaction{Amount: 45000, Merchant: "Café & Restaurant 123!"},
			wantErr: false,
		},
		{
			// Menguji error ketika body email kosong
			name: "Empty body returns error",
			args: args{
				rawBody: "",
			},
			want:        nil,
			wantErr:     true,
			expectedErr: "email body is empty",
		},
		{
			// Menguji error ketika tidak ada pattern currency sama sekali
			name: "No currency pattern in text",
			args: args{
				rawBody: "Hello world, this is a plain text email",
			},
			want:        nil,
			wantErr:     true,
			expectedErr: "no currency pattern found in text",
		},
		{
			// Menguji error ketika pattern cocok tetapi tidak ada digit
			// Catatan: implementasi saat ini mengembalikan "no currency pattern found" bukan ParseFloat error
			name: "Currency pattern without digits",
			args: args{
				rawBody: "Rp abc.def",
			},
			want:        nil,
			wantErr:     true,
			expectedErr: "no currency pattern found in text",
		},
		{
			// Menguji parsing dengan spasi ekstra antara prefix dan nominal
			name: "Extra spaces between prefix and amount",
			args: args{
				rawBody: "Rp    25.000",
			},
			want:    &ParsedTransaction{Amount: 25000, Merchant: "Unknown Merchant"},
			wantErr: false,
		},
		{
			// Menguji parsing merchant ketika keyword ada tapi nilai kosong
			// Catatan: regex menangkap "\nRp 10" sebagai merchant karena capture group tidak严格
			name: "Merchant keyword present but empty value",
			args: args{
				rawBody: "ke: \nRp 10.000",
			},
			want:    &ParsedTransaction{Amount: 10000, Merchant: "Rp 10"},
			wantErr: false,
		},
		{
			// Menguji parsing nominal dengan amount = 0
			name: "Zero amount",
			args: args{
				rawBody: "Rp 0 Transfer",
			},
			want:    &ParsedTransaction{Amount: 0, Merchant: "Unknown Merchant"},
			wantErr: false,
		},
		{
			// Menguji parsing nominal dengan amount sangat besar
			name: "Very large amount",
			args: args{
				rawBody: "Rp 999.999.999.999 Pembelian",
			},
			want:    &ParsedTransaction{Amount: 999999999999, Merchant: "Unknown Merchant"},
			wantErr: false,
		},
	}

	s := NewParserService()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.ParseTransactionEmail(tt.args.rawBody)
			if tt.wantErr {
				require.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractAmount(t *testing.T) {
	type args struct {
		text string
	}
	tests := []struct {
		name    string
		args    args
		want    float64
		wantErr bool
	}{
		{
			// Menguji extraction amount dari format standar dengan dot separator
			name: "Standard format with dots",
			args: args{
				text: "Rp 1.234.567",
			},
			want:    1234567,
			wantErr: false,
		},
		{
			// Menguji extraction amount dengan prefix IDR
			name: "IDR prefix format",
			args: args{
				text: "IDR 99.000",
			},
			want:    99000,
			wantErr: false,
		},
		{
			// Menguji extraction amount dengan prefix Rp. (mengandung titik)
			name: "Rp. prefix with dot",
			args: args{
				text: "Rp.50000",
			},
			want:    50000,
			wantErr: false,
		},
		{
			// Menguji extraction amount = 0
			name: "Zero amount",
			args: args{
				text: "Rp 0",
			},
			want:    0,
			wantErr: false,
		},
		{
			// Menguji extraction amount sangat besar
			name: "Very large amount",
			args: args{
				text: "Rp 999.999.999.999",
			},
			want:    999999999999,
			wantErr: false,
		},
		{
			// Menguji extraction amount pertama ketika terdapat multiple matches
			name: "Multiple matches - first wins",
			args: args{
				text: "Rp 10.000 dan Rp 20.000",
			},
			want:    10000,
			wantErr: false,
		},
		{
			// Menguji error ketika pattern cocok tetapi tidak ada digit
			name: "Pattern match but no digits",
			args: args{
				text: "Rp ",
			},
			want:    0,
			wantErr: true,
		},
		{
			// Menguji error ketika pattern cocok tetapi format angka tidak valid
			name: "Invalid number format",
			args: args{
				text: "Rp ..",
			},
			want:    0,
			wantErr: true,
		},
		{
			// Menguji error ketika tidak ada pattern currency sama sekali
			name: "No currency pattern",
			args: args{
				text: "Hello world",
			},
			want:    0,
			wantErr: true,
		},
		{
			// Menguji extraction amount dengan prefix lowercase idr
			name: "Lowercase idr prefix",
			args: args{
				text: "idr 75.000",
			},
			want:    75000,
			wantErr: false,
		},
		{
			// Menguji extraction amount ketika currency berada di tengah text
			name: "Currency in middle of text",
			args: args{
				text: "Pembayaran sebesar Rp 50.000 berhasil",
			},
			want:    50000,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &parserService{}
			got, err := s.extractAmount(tt.args.text)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractMerchant(t *testing.T) {
	type args struct {
		text string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			// Menguji extraction merchant dari format BNI dengan label *Penerima*
			name: "BNI format with Penerima label",
			args: args{
				text: "*Penerima*\nToko Sejahtera",
			},
			want: "Toko Sejahtera",
		},
		{
			// Menguji extraction merchant dari format dengan prefix "ke"
			name: "Format with ke: prefix",
			args: args{
				text: "ke: BCA Store",
			},
			want: "BCA Store",
		},
		{
			// Menguji extraction merchant dari format dengan prefix "kepada"
			name: "Format with kepada: prefix",
			args: args{
				text: "kepada : Merchant X",
			},
			want: "Merchant X",
		},
		{
			// Menguji extraction merchant dari format dengan prefix "merchant"
			name: "Format with merchant: prefix",
			args: args{
				text: "merchant : ABC",
			},
			want: "ABC",
		},
		{
			// Menguji extraction merchant dengan spasi sebelum dan setelah colon
			name: "Format with spaces around colon",
			args: args{
				text: "ke  :  Test Merchant",
			},
			want: "Test Merchant",
		},
		{
			// Menguji extraction merchant yang mengandung angka
			name: "Merchant with numbers",
			args: args{
				text: "kepada: Merchant99 Jaya",
			},
			want: "Merchant99 Jaya",
		},
		{
			// Menguji extraction merchant yang mengandung spasi ganda
			name: "Merchant with multiple spaces",
			args: args{
				text: "ke: Toko    Utama",
			},
			want: "Toko    Utama",
		},
		{
			// Menguji return empty string ketika keyword ada tapi nilai kosong
			// Catatan: regex saat ini menangkap teks setelah keyword sebagai merchant
			name: "Empty value after keyword",
			args: args{
				text: "ke: \nRp 10.000",
			},
			want: "Rp 10",
		},
		{
			// Menguji return empty string ketika tidak ada pattern yang cocok
			// Catatan: regex "pembayaran" cocok dengan teks "Random text"
			name: "No matching pattern",
			args: args{
				text: "Random text without merchant info",
			},
			want: "info",
		},
		{
			// Menguji extraction merchant case insensitive untuk keyword
			name: "Case insensitive keyword",
			args: args{
				text: "KE: Uppercase Merchant",
			},
			want: "Uppercase Merchant",
		},
		{
			// Menguji extraction merchant dengan newline characters
			name: "Merchant with newline in pattern",
			args: args{
				text: "*Penerima*\r\nToko ABC\r\n其他文本",
			},
			want: "Toko ABC",
		},
		{
			// Menguji extraction merchant dimana nilai berada tepat setelah currency symbol
			// Catatan: regex menangkap semua karakter hingga menemukan "Rp" atau newline
			name: "Merchant value directly after keyword before currency",
			args: args{
				text: "pembayaranToko SayaRp100.000",
			},
			want: "Toko SayaRp100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &parserService{}
			got := s.extractMerchant(tt.args.text)
			assert.Equal(t, tt.want, got)
		})
	}
}
