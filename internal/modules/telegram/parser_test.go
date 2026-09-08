package telegram

import (
	"testing"
)

func TestParseTransactionMessage(t *testing.T) {
	cases := []struct {
		input          string
		wantMerchant   string
		wantAmount     float64
		wantNil        bool
	}{
		{"Makan warteg 20k", "Makan warteg", 20000, false},
		{"Beli bensin 50000", "Beli bensin", 50000, false},
		{"Kopi 15.000", "Kopi", 15000, false},
		{"Makan siang 25.500", "Makan siang", 25500, false},
		{"Grab 35k", "Grab", 35000, false},
		{"Netflix 54.000", "Netflix", 54000, false},
		{"/start", "", 0, true},
		{"hello", "", 0, true},
		{"just text no number", "", 0, true},
	}

	for _, tc := range cases {
		result := ParseTransactionMessage(tc.input)
		if tc.wantNil {
			if result != nil {
				t.Errorf("input %q: expected nil, got merchant=%q amount=%.0f", tc.input, result.Merchant, result.Amount)
			}
			continue
		}
		if result == nil {
			t.Errorf("input %q: expected non-nil result", tc.input)
			continue
		}
		if result.Merchant != tc.wantMerchant {
			t.Errorf("input %q: merchant got %q, want %q", tc.input, result.Merchant, tc.wantMerchant)
		}
		if result.Amount != tc.wantAmount {
			t.Errorf("input %q: amount got %.0f, want %.0f", tc.input, result.Amount, tc.wantAmount)
		}
	}
}
