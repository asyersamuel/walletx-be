package services

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// ParsedTransaction holds the result of the regex extraction
type ParsedTransaction struct {
	Amount   float64
	Merchant string
}

// ParserService defines the contract for parsing text data
type ParserService interface {
	ParseTransactionEmail(rawBody string) (*ParsedTransaction, error)
}

type parserService struct {
	// No repositories needed here, this is a pure utility service
}

// NewParserService creates a new instance of ParserService
func NewParserService() ParserService {
	return &parserService{}
}

// ParseTransactionEmail extracts the transaction amount and merchant from raw text
func (s *parserService) ParseTransactionEmail(rawBody string) (*ParsedTransaction, error) {
	if rawBody == "" {
		return nil, errors.New("email body is empty")
	}

	amount, err := s.extractAmount(rawBody)
	if err != nil {
		logrus.WithError(err).Warn("⚠️ [Parser] Failed to extract amount")
		return nil, err
	}

	merchant := s.extractMerchant(rawBody)
	if merchant == "" {
		// If regex fails to find a merchant, provide a fallback
		merchant = "Unknown Merchant"
		logrus.Warn("⚠️ [Parser] Merchant not found, using default")
	}

	return &ParsedTransaction{
		Amount:   amount,
		Merchant: merchant,
	}, nil
}

// extractAmount uses regex to find currency formats like "Rp 50.000" or "IDR 500,000"
func (s *parserService) extractAmount(text string) (float64, error) {
	// Regex matches "Rp", "Rp.", "IDR" followed by numbers and dots
	// Example matches: "Rp 50.000", "Rp.150.000", "IDR 25.000"
	re := regexp.MustCompile(`(?i)(?:Rp|IDR)\.?\s*([\d\.]+)`)
	matches := re.FindStringSubmatch(text)

	if len(matches) < 2 {
		return 0, errors.New("no currency pattern found in text")
	}

	// Remove the dots before converting to float
	cleanNumberStr := strings.ReplaceAll(matches[1], ".", "")

	amount, err := strconv.ParseFloat(cleanNumberStr, 64)
	if err != nil {
		return 0, err
	}

	return amount, nil
}

// extractMerchant tries to find the recipient name using common banking keywords
func (s *parserService) extractMerchant(text string) string {

	// NANTINYA BAGIAN INI HARUS DIBUAT LEBIH FLEXIBLE

	// 1. Pola Khusus BNI (wondr by BNI)
	reBNI := regexp.MustCompile(`(?i)\*Penerima\*\s*\r?\n([^\r\n]+)`)
	matchesBNI := reBNI.FindStringSubmatch(text)
	if len(matchesBNI) >= 2 {
		return strings.TrimSpace(matchesBNI[1])
	}

	// 2. Pola Umum (BCA, Mandiri, e-Wallet)
	reUmum := regexp.MustCompile(`(?i)(?:ke|kepada|merchant|pembayaran)\s*:?\s*([A-Za-z0-9\s]+)(?:Rp|\n|\.|$)`)
	matchesUmum := reUmum.FindStringSubmatch(text)
	if len(matchesUmum) >= 2 {
		return strings.TrimSpace(matchesUmum[1])
	}

	return ""
}