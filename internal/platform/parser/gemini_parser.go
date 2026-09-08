package parser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"walletx-be/internal/platform/logger"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

var dateRegex1 = regexp.MustCompile(`(?i)\b(\d{2}/\d{2}/\d{4}\s+\d{2}:\d{2}:\d{2})\b`)
var dateRegex2 = regexp.MustCompile(`(?i)\b(\d{2}\s+[A-Za-z]{3,}\s+\d{4}\s+\d{2}:\d{2}:\d{2})\b`)

type geminiEmailParser struct {
	logger logger.Logger
	apiKey string
}

type geminiResponse struct {
	Merchant string   `json:"merchant"`
	Amount   *float64 `json:"amount,omitempty"`
	Date     string   `json:"date,omitempty"`
}

func NewGeminiEmailParser(logger logger.Logger, apiKey string) EmailParser {
	return &geminiEmailParser{
		logger: logger,
		apiKey: apiKey,
	}
}

func (p *geminiEmailParser) Parse(rawBody string) (*ParsedTransactionData, error) {
	if rawBody == "" {
		return nil, errors.New("email body is empty")
	}

	amount, err := p.extractAmount(rawBody)
	if err != nil {
		return nil, err
	}

	merchant, geminiResp, err := p.extractMerchant(rawBody)
	if err != nil {
		return nil, err
	}

	txDate := p.extractDate(rawBody)
	if txDate == nil && geminiResp != nil && geminiResp.Date != "" {
		if parsed, err := time.Parse(time.RFC3339, geminiResp.Date); err == nil {
			txDate = &parsed
		}
	}

	p.logger.WithFields(map[string]interface{}{
		"amount":   amount,
		"merchant": merchant,
		"tx_date":  txDate,
	}).Info("Email parsing completed")

	return &ParsedTransactionData{
		Amount:          amount,
		Merchant:        merchant,
		TransactionDate: txDate,
	}, nil
}

func (p *geminiEmailParser) extractAmount(text string) (float64, error) {
	matches := amountRegex.FindStringSubmatch(text)
	if len(matches) < 2 {
		return 0, errors.New("amount pattern not found in email")
	}

	raw := matches[1]
	raw = strings.ReplaceAll(raw, ".", "")
	if idx := strings.Index(raw, ","); idx != -1 {
		raw = raw[:idx]
	}

	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}

	return amount, nil
}

func (p *geminiEmailParser) extractDate(text string) *time.Time {
	datePatterns := []struct {
		re     *regexp.Regexp
		layout string
	}{
		{
			re:     dateRegex1,
			layout: "02/01/2006 15:04:05",
		},
		{
			re:     dateRegex2,
			layout: "02 Jan 2006 15:04:05",
		},
	}

	for _, pattern := range datePatterns {
		matches := pattern.re.FindStringSubmatch(text)
		if len(matches) < 2 {
			continue
		}
		raw := strings.TrimSpace(matches[1])
		if pattern.layout == "02 Jan 2006 15:04:05" {
			raw = normalizeIndonesianMonth(raw)
		}
		if t, err := time.ParseInLocation(pattern.layout, raw, time.Local); err == nil {
			return &t
		}
	}

	return nil
}

func normalizeIndonesianMonth(text string) string {
	monthMap := map[string]string{
		"jan": "Jan",
		"feb": "Feb",
		"mar": "Mar",
		"apr": "Apr",
		"mei": "May",
		"jun": "Jun",
		"jul": "Jul",
		"agu": "Aug",
		"agt": "Aug",
		"sep": "Sep",
		"okt": "Oct",
		"nov": "Nov",
		"des": "Dec",
	}
	words := strings.Fields(text)
	for i, word := range words {
		lower := strings.ToLower(word)
		if eng, ok := monthMap[lower]; ok {
			words[i] = eng
			break
		}
	}
	return strings.Join(words, " ")
}

func (p *geminiEmailParser) extractMerchant(text string) (string, *geminiResponse, error) {
	for _, rule := range merchantRules {
		matches := rule.Regex.FindStringSubmatch(text)
		if len(matches) >= 2 {
			name := strings.TrimSpace(matches[1])
			if name != "" {
				p.logger.WithFields(map[string]interface{}{
					"bank":     rule.BankName,
					"merchant": name,
				}).Info("Merchant extracted via regex")
				return name, nil, nil
			}
		}
	}

	p.logger.Warn("Regex extraction failed, falling back to Gemini AI. Review email to add regex pattern.")
	geminiResp, err := p.extractWithGemini(text)
	if err != nil {
		return "", nil, err
	}
	return geminiResp.Merchant, geminiResp, nil
}

func (p *geminiEmailParser) extractWithGemini(text string) (*geminiResponse, error) {
	if p.apiKey == "" {
		return nil, errors.New("GEMINI_API_KEY not configured in environment")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(p.apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel("gemini-1.5-flash")
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{
			genai.Text("Extract merchant name and transaction date from bank notification email. Respond with ONLY valid JSON: {\"merchant\": \"name\", \"date\": \"RFC3339\"}. The date field should contain the actual transaction date/time in RFC3339/ISO8601 format (example: \"2026-04-30T23:42:58+07:00\"). If no merchant found, use \"Unknown Merchant\". If no date found, omit the date field."),
		},
	}

	maxTokens := int32(128)
	model.GenerationConfig = genai.GenerationConfig{
		MaxOutputTokens: &maxTokens,
		Temperature:     ptr(float32(0)),
	}

	p.logger.Warn("Calling Gemini AI API (timeout: 5s). Check logs to create regex pattern for this email format.")

	resp, err := model.GenerateContent(ctx, genai.Text(text))
	if err != nil {
		return nil, fmt.Errorf("Gemini API call failed: %w", err)
	}

	if resp == nil || len(resp.Candidates) == 0 {
		return nil, errors.New("Gemini returned empty response")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, errors.New("Gemini candidate has no content")
	}

	resultText, ok := candidate.Content.Parts[0].(genai.Text)
	if !ok {
		return nil, errors.New("Gemini response type not recognized")
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal([]byte(string(resultText)), &geminiResp); err != nil {
		p.logger.WithError(err).Warn("Failed to parse Gemini JSON response, using raw text")
		merchant := strings.TrimSpace(string(resultText))
		if merchant == "" {
			return nil, errors.New("Gemini returned empty merchant name")
		}
		return &geminiResponse{Merchant: merchant}, nil
	}

	if geminiResp.Merchant == "" {
		return nil, errors.New("Gemini returned empty merchant name")
	}

	p.logger.WithField("merchant", geminiResp.Merchant).Info("Gemini extracted merchant successfully")
	return &geminiResp, nil
}

func ptr[T any](v T) *T {
	return &v
}
