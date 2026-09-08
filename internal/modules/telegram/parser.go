package telegram

import (
	"regexp"
	"strconv"
	"strings"
)

// ParsedTransaction holds the result of parsing a free-text transaction message.
type ParsedTransaction struct {
	Merchant string
	Amount   float64
}

// txRegex matches an optional leading text (merchant) followed by a numeric amount.
// Supported amount formats:
//   - "20k"        → 20000
//   - "20.000"     → 20000  (dot as thousands separator)
//   - "20,000"     → 20000  (comma as thousands separator)
//   - "20000"      → 20000
//   - "20.5k"      → 20500
//
// The number must appear at or near the end of the string so that merchant
// names with numbers are not accidentally consumed.
var txRegex = regexp.MustCompile(
	`(?i)^(.+?)\s+([\d]+(?:[.,]\d{3})*(?:\.\d+)?|[\d]+(?:\.\d+)?)k?\s*$`,
)

// amountOnlyRegex matches a bare numeric token (no required merchant prefix).
var amountOnlyRegex = regexp.MustCompile(
	`(?i)^\s*([\d]+(?:[.,]\d{3})*(?:\.\d+)?|[\d]+(?:\.\d+)?)k?\s*$`,
)

// ParseTransactionMessage attempts to extract a merchant name and amount from a
// free-text message such as "Makan warteg 20k" or "Beli bensin 50.000".
// Returns nil when the message does not match the expected format.
func ParseTransactionMessage(text string) *ParsedTransaction {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Normalise: collapse multiple spaces.
	spaceRe := regexp.MustCompile(`\s+`)
	text = spaceRe.ReplaceAllString(text, " ")

	matches := txRegex.FindStringSubmatch(text)
	if len(matches) != 3 {
		return nil
	}

	merchant := strings.TrimSpace(matches[1])
	rawAmount := matches[2]

	// Detect "k" multiplier (the regex ends before k but we need to check
	// the original string suffix).
	multiplier := 1.0
	lc := strings.ToLower(text)
	if strings.HasSuffix(strings.TrimSpace(lc), "k") {
		multiplier = 1000.0
	}

	// Strip dots and commas used as thousands separators before parsing.
	// We treat a lone dot/comma followed by exactly 3 digits as a thousands
	// separator; anything else (e.g. "20.5") is a decimal.
	cleaned := cleanAmount(rawAmount)

	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || amount <= 0 {
		return nil
	}

	return &ParsedTransaction{
		Merchant: merchant,
		Amount:   amount * multiplier,
	}
}

// ParseAmount tries to extract a single monetary amount from a bare string such
// as "500k", "500.000", or "500000". It returns (amount, true) on success and
// (0, false) if the text is not a recognisable number.
func ParseAmount(text string) (float64, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}

	matches := amountOnlyRegex.FindStringSubmatch(text)
	if len(matches) < 2 {
		return 0, false
	}

	rawAmount := matches[1]

	multiplier := 1.0
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(text)), "k") {
		multiplier = 1000.0
	}

	cleaned := cleanAmount(rawAmount)
	amount, err := strconv.ParseFloat(cleaned, 64)
	if err != nil || amount <= 0 {
		return 0, false
	}

	return amount * multiplier, true
}

// cleanAmount strips thousands separators (dot or comma before 3-digit groups)
// while preserving decimal notation.
func cleanAmount(s string) string {
	// Replace commas used as thousands separators.
	commaThousands := regexp.MustCompile(`(\d),(\d{3})`)
	s = commaThousands.ReplaceAllString(s, "$1$2")

	// Replace dots used as thousands separators (dot followed by exactly 3 digits
	// and then either end-of-string or another dot/comma).
	dotThousands := regexp.MustCompile(`(\d)\.(\d{3})(?:[.,]|$)`)
	for dotThousands.MatchString(s) {
		s = dotThousands.ReplaceAllStringFunc(s, func(m string) string {
			inner := dotThousands.FindStringSubmatch(m)
			if len(inner) < 3 {
				return m
			}
			suffix := ""
			if len(m) > len(inner[1])+1+len(inner[2]) {
				suffix = string(m[len(m)-1])
			}
			return inner[1] + inner[2] + suffix
		})
	}

	return s
}
