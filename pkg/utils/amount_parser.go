package utils

import (
	"regexp"
	"strconv"
	"strings"
)

// amountOnlyRegex matches a bare numeric token (no required merchant prefix).
// Supported formats:
//   - "500k"     → 500000
//   - "500.000"  → 500000  (dot as thousands separator)
//   - "500,000"  → 500000  (comma as thousands separator)
//   - "500000"   → 500000
//   - "1.5k"     → 1500
var amountOnlyRegex = regexp.MustCompile(
	`(?i)^\s*([\d]+(?:[.,]\d{3})*(?:\.\d+)?|[\d]+(?:\.\d+)?)k?\s*$`,
)

// ParseAmount tries to extract a single monetary amount from a bare string such
// as "500k", "500.000", or "500000". It returns (amount, true) on success and
// (0, false) if the text is not a recognisable number.
//
// This is intentionally separate from ParseTransactionMessage (which requires a
// merchant prefix) so that conversational reply handlers can validate isolated
// amount strings without constructing a fake merchant.
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
