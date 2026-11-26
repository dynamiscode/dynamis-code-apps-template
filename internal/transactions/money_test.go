package transactions

import (
	"math"
	"testing"

	"example.com/dynamis-code/apps-template/internal/currency"
)

func TestCurrencyScale(t *testing.T) {
	for _, tc := range []struct {
		code  string
		scale int
		known bool
	}{
		{"USD", 2, true}, {"JPY", 0, true}, {"KWD", 3, true},
		{"MRU", 2, true}, {"SLE", 2, true}, {"VED", 2, true}, {"VES", 2, true}, {"ZWG", 2, true},
		{"CLF", 4, true}, {"ZZZ", 0, false}, {"AAA", 0, false},
		{"XXX", 0, false}, {"XAU", 0, false}, {"XFO", 0, false},
		{"DEM", 2, true}, {"FRF", 2, true}, {"ITL", 0, true},
		{"BEF", 0, true}, {"XEU", 0, false}, {"US", 0, false}, {"123", 0, false},
	} {
		scale, known := CurrencyScale(tc.code)
		if scale != tc.scale || known != tc.known {
			t.Errorf("CurrencyScale(%q) = %d, %t; want %d, %t", tc.code, scale, known, tc.scale, tc.known)
		}
	}
}

func TestAvailableCurrenciesUseCanonicalTransactionScale(t *testing.T) {
	for _, item := range currency.Available() {
		if !CurrencyAvailable(item) {
			t.Errorf("currency %s with precision %d is not available to transactions", item.Code, item.Decimals)
		}
	}
}

func TestAmountLocale(t *testing.T) {
	english := AmountLocale("en-US")
	if english.Digits != [10]string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"} || english.Decimal != "." || english.Group != "," || english.Primary != 3 || english.Secondary != 3 || english.MinGroup != 4 {
		t.Fatalf("English amount metadata = %+v", english)
	}
	spanish := AmountLocale("es")
	if spanish.Decimal != "," || spanish.Group != "." {
		t.Fatalf("Spanish amount separators = %q, %q", spanish.Decimal, spanish.Group)
	}
}

func TestFormatMoney(t *testing.T) {
	for _, tc := range []struct {
		minor, code, locale, want string
	}{
		{"123456", "USD", "en-US", "USD 1,234.56"},
		{"1234", "EUR", "es", "EUR 12,34"},
		{"123456", "JPY", "en-US", "JPY 123,456"},
		{"-123456", "KWD", "en-US", "-KWD 123.456"},
		{"1", "KWD", "en-US", "KWD 0.001"},
		{"123456789012345678901234567890", "USD", "en-US", "USD 1,234,567,890,123,456,789,012,345,678.90"},
		{"-123456789012345678901", "ZZZ", "en-US", "ZZZ -123456789012345678901 minor units"},
		{"-123456789012345678901", "ZZZ", "es-MX", "ZZZ -123456789012345678901 unidades menores"},
		{"123", "DEM", "es", "DEM 1,23"},
		{"123", "FRF", "es", "FRF 1,23"},
		{"123", "MRU", "en-US", "MRU 1.23"},
		{"123", "SLE", "en-US", "SLE 1.23"},
		{"123", "VED", "en-US", "VED 1.23"},
		{"123", "VES", "en-US", "VES 1.23"},
		{"123", "ZWG", "en-US", "ZWG 1.23"},
		{"123", "ITL", "en-US", "ITL 123"},
		{"123", "XEU", "en-US", "XEU 123 minor units"},
		{"broken", "USD", "en-US", "USD broken minor units"},
	} {
		if got := FormatTotal(tc.minor, tc.code, tc.locale); got != tc.want {
			t.Errorf("FormatTotal(%q, %q, %q) = %q; want %q", tc.minor, tc.code, tc.locale, got, tc.want)
		}
	}
	if got := FormatAmount(math.MinInt64, "USD", "en-US"); got != "-USD 92,233,720,368,547,758.08" {
		t.Errorf("FormatAmount(MinInt64) = %q", got)
	}
}

func TestParseAmount(t *testing.T) {
	for _, tc := range []struct {
		input, code, locale string
		want                int64
		invalid             bool
	}{
		{"1,234.56", "USD", "en-US", 123456, false},
		{"1234,5", "EUR", "es", 123450, false},
		{"123", "JPY", "en-US", 123, false},
		{"-0.001", "KWD", "en-US", -1, false},
		{"1.23", "MRU", "en-US", 123, false},
		{"1.23", "SLE", "en-US", 123, false},
		{"1.23", "VED", "en-US", 123, false},
		{"1.23", "VES", "en-US", 123, false},
		{"1.23", "ZWG", "en-US", 123, false},
		{"92233720368547758.07", "USD", "en-US", math.MaxInt64, false},
		{"-92233720368547758.08", "USD", "en-US", math.MinInt64, false},
		{"92233720368547758.08", "USD", "en-US", 0, true},
		{"-92233720368547758.09", "USD", "en-US", 0, true},
		{"1.001", "USD", "en-US", 0, true},
		{"1.0", "JPY", "en-US", 0, true},
		{"12,34.00", "USD", "en-US", 0, true},
		{"1.234,56", "USD", "en-US", 0, true},
		{"1", "ZZZ", "en-US", 0, true},
		{"1.23", "DEM", "en-US", 123, false},
		{"1.23", "FRF", "en-US", 123, false},
		{"123", "ITL", "en-US", 123, false},
		{"1.23", "ITL", "en-US", 0, true},
		{"", "USD", "en-US", 0, true},
	} {
		got, err := ParseAmount(tc.input, tc.code, tc.locale)
		if (err != nil) != tc.invalid || (!tc.invalid && got != tc.want) {
			t.Errorf("ParseAmount(%q, %q, %q) = %d, %v; want %d, invalid=%t", tc.input, tc.code, tc.locale, got, err, tc.want, tc.invalid)
		}
	}
}
