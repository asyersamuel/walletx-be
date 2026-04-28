package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"
)

// ParsedTransaction menampung hasil ekstraksi dari email transaksi
type ParsedTransaction struct {
	Amount   float64
	Merchant string
}

// ParserService adalah kontrak interface untuk service ini
type ParserService interface {
	ParseTransactionEmail(rawBody string) (*ParsedTransaction, error)
}

type parserService struct{}

// NewParserService membuat instance baru ParserService.
// Gemini client dibuat per-request (lazy) menggunakan GEMINI_API_KEY dari env.
func NewParserService() ParserService {
	return &parserService{}
}

// ParseTransactionEmail adalah entry point utama parser.
// Alur: ekstrak amount via Regex → ekstrak merchant via Hybrid (Regex + Gemini).
func (s *parserService) ParseTransactionEmail(rawBody string) (*ParsedTransaction, error) {
	if rawBody == "" {
		return nil, errors.New("email body is empty")
	}

	// ── 1. Ekstrak Nominal (Regex selalu cukup untuk angka bank Indonesia) ──
	amount, err := s.extractAmount(rawBody)
	if err != nil {
		logrus.WithError(err).Warn("[Parser] Gagal mengekstrak nominal — bukan email transaksi?")
		return nil, err
	}

	// ── 2. Ekstrak Merchant (Hybrid System) ──
	merchant := s.extractMerchant(rawBody)

	logrus.WithFields(logrus.Fields{
		"amount":   amount,
		"merchant": merchant,
	}).Info("[Parser] ✅ Parsing selesai")

	return &ParsedTransaction{
		Amount:   amount,
		Merchant: merchant,
	}, nil
}

// extractAmount menggunakan amountRegex dari parser_rules.go.
// Membersihkan titik ribuan Indonesia (1.000.000 → 1000000) sebelum parsing.
func (s *parserService) extractAmount(text string) (float64, error) {
	matches := amountRegex.FindStringSubmatch(text)
	if len(matches) < 2 {
		return 0, errors.New("pola nominal uang tidak ditemukan dalam email")
	}

	// Hapus pemisah ribuan (.) dan abaikan desimal koma (,xx)
	raw := matches[1]
	raw = strings.ReplaceAll(raw, ".", "")
	// Jika ada koma desimal (misal 1000000,00), potong di sana
	if idx := strings.Index(raw, ","); idx != -1 {
		raw = raw[:idx]
	}

	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("gagal parse angka '%s': %w", raw, err)
	}

	return amount, nil
}

// extractMerchant adalah implementasi Hybrid System.
// Jalur A (cepat): iterasi merchantRules dari parser_rules.go.
// Jalur B (pintar): fallback ke Gemini AI jika semua regex gagal.
func (s *parserService) extractMerchant(text string) string {
	// ── A. JALUR REGEX ──
	for _, rule := range merchantRules {
		matches := rule.Regex.FindStringSubmatch(text)
		if len(matches) >= 2 {
			name := strings.TrimSpace(matches[1])
			if name != "" {
				logrus.WithFields(logrus.Fields{
					"bank":     rule.BankName,
					"merchant": name,
				}).Info("[Parser] ✅ Regex match berhasil")
				return name
			}
		}
	}

	// ── B. JALUR GEMINI FALLBACK ──
	logrus.Warn("[Parser] ⚠️ Semua regex gagal. Menggunakan Gemini AI sebagai fallback...")
	return s.extractWithGemini(text)
}

// extractWithGemini menggunakan Gemini API (gemini-1.5-flash) untuk
// mengekstrak nama merchant ketika semua regex bank gagal.
// Autentikasi via environment variable GEMINI_API_KEY.
func (s *parserService) extractWithGemini(text string) string {
	log := logrus.WithField("source", "Gemini")

	// ── 1. Ambil API Key dari environment ──
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Error("[Parser] GEMINI_API_KEY tidak ditemukan di environment. Gemini dilewati.")
		return "Unknown Merchant"
	}

	// ── 2. Inisialisasi Gemini Client ──
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		log.WithError(err).Error("[Parser] Gagal membuat Gemini client")
		return "Unknown Merchant"
	}
	defer client.Close()

	// ── 3. Konfigurasi Model ──
	model := client.GenerativeModel("gemini-1.5-flash")

	// System instruction: perintahkan AI hanya mengembalikan nama merchant
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text("Extract the merchant name from this bank notification email. Output ONLY the merchant name as a string. If not found, return an empty string."),
		},
	}

	// Batasi output: merchant name cukup singkat
	maxTokens := int32(64)
	model.GenerationConfig = genai.GenerationConfig{
		MaxOutputTokens: &maxTokens,
	}

	// ── 4. Kirim Request ke Gemini ──
	log.Info("[Parser] 🤖 Mengirim request ke Gemini API...")
	resp, err := model.GenerateContent(ctx, genai.Text(text))
	if err != nil {
		log.WithError(err).Error("[Parser] Gemini API mengembalikan error")
		return "Unknown Merchant"
	}

	// ── 5. Ekstrak Teks dari Response ──
	if resp == nil || len(resp.Candidates) == 0 {
		log.Warn("[Parser] Gemini mengembalikan response kosong")
		return "Unknown Merchant"
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		log.Warn("[Parser] Gemini candidate tidak memiliki konten")
		return "Unknown Merchant"
	}

	// Part pertama adalah teks jawaban dari AI
	resultText, ok := candidate.Content.Parts[0].(genai.Text)
	if !ok {
		log.Warn("[Parser] Tipe response Gemini tidak dikenali (bukan teks)")
		return "Unknown Merchant"
	}

	merchant := strings.TrimSpace(string(resultText))
	if merchant == "" {
		log.Warn("[Parser] Gemini tidak dapat mengidentifikasi merchant")
		return "Unknown Merchant"
	}

	log.WithField("merchant", merchant).Info("[Parser] 🤖 Gemini berhasil mengekstrak merchant")
	return merchant
}