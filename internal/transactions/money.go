package transactions

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/text/currency"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

var currentCurrencies = func() map[currency.Unit]bool {
	units := make(map[currency.Unit]bool)
	for it := currency.Query(currency.NonTender); it.Next(); {
		units[it.Unit()] = true
	}
	return units
}()

// CurrencyScale reports the standard decimal precision when it is defined.
func CurrencyScale(code string) (int, bool) {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 0, false
	}
	// Historical ISO 4217 minor units from the BSI list reproduced by SAP:
	// https://help.sap.com/saphelp_pos22/helpdata/en/4f/efa63a9bb04d94a0ae4c32fd6cd827/content.htm
	switch unit.String() {
	case "AFA", "ATS", "AZM", "BGL", "CYP", "DEM", "EEK", "FIM", "FRF", "GHC", "GWP", "HRK", "IEP", "LTL", "LVL", "MTL", "NLG", "SIT", "SKK":
		return 2, true
	case "ADP", "BEF", "BYR", "ESP", "GRD", "ITL", "LUF", "MGF", "PTE", "TPE":
		return 0, true
	}
	if !currentCurrencies[unit] {
		return 0, false
	}
	// CLDR defaults do not define a minor unit for these ISO codes.
	switch unit.String() {
	case "XXX", "XTS", "XAU", "XAG", "XPD", "XPT", "XBA", "XBB", "XBC", "XBD", "XDR", "XSU", "XUA":
		return 0, false
	}
	scale, _ := currency.Standard.Rounding(unit)
	return scale, true
}

type moneyLocale struct {
	digits                       [10]string
	decimal, group               string
	primary, secondary, minGroup int
}

func monetaryLocale(locale string) moneyLocale {
	tag, err := language.Parse(locale)
	if err != nil {
		tag = language.English
	}
	p := message.NewPrinter(tag)
	var l moneyLocale
	for i := range l.digits {
		l.digits[i] = p.Sprint(number.Decimal(i, number.NoSeparator()))
	}
	decimal := p.Sprint(number.Decimal(1, number.Scale(1)))
	l.decimal = strings.TrimSuffix(strings.TrimPrefix(decimal, l.digits[1]), l.digits[0])
	grouped := p.Sprint(number.Decimal(int64(123456789012345)))
	for _, digit := range l.digits {
		grouped = strings.ReplaceAll(grouped, digit, "0")
	}
	for _, r := range grouped {
		if r != '0' {
			l.group = string(r)
			break
		}
	}
	l.primary, l.secondary, l.minGroup = 3, 3, 4
	if l.group != "" {
		groups := strings.Split(grouped, l.group)
		if len(groups) > 1 {
			l.primary = len(groups[len(groups)-1])
			l.secondary = len(groups[len(groups)-2])
		}
		thousand := p.Sprint(number.Decimal(1000))
		if !strings.Contains(thousand, l.group) {
			l.minGroup = 5
		}
	}
	return l
}

// FormatAmount formats an int64 amount stored in minor units.
func FormatAmount(minor int64, code, locale string) string {
	return FormatTotal(strconv.FormatInt(minor, 10), code, locale)
}

// FormatTotal formats an arbitrary-size integer total stored in minor units.
// Unrecognized currencies and malformed totals remain visible as raw minor units.
func FormatTotal(minor, code, locale string) string {
	scale, known := CurrencyScale(code)
	negative := strings.HasPrefix(minor, "-")
	digits := strings.TrimPrefix(minor, "-")
	if !known || !asciiDigits(digits) {
		label := "minor units"
		if tag, err := language.Parse(locale); err == nil {
			base, _ := tag.Base()
			if base.String() == "es" {
				label = "unidades menores"
			}
		}
		return code + " " + minor + " " + label
	}
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		digits = "0"
		negative = false
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale+1-len(digits)) + digits
	}
	whole, fraction := digits[:len(digits)-scale], digits[len(digits)-scale:]
	l := monetaryLocale(locale)
	var b strings.Builder
	if negative {
		b.WriteByte('-')
	}
	b.WriteString(code)
	b.WriteByte(' ')
	for i := range len(whole) {
		remaining := len(whole) - i
		if i > 0 && len(whole) >= l.minGroup && l.group != "" && remaining >= l.primary && (remaining-l.primary)%l.secondary == 0 {
			b.WriteString(l.group)
		}
		b.WriteString(l.digits[whole[i]-'0'])
	}
	if scale > 0 {
		b.WriteString(l.decimal)
		for i := range len(fraction) {
			b.WriteString(l.digits[fraction[i]-'0'])
		}
	}
	return b.String()
}

func asciiDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ParseAmount parses a localized decimal amount exactly into int64 minor units.
// It rejects unknown precision, malformed grouping, excess decimals and overflow.
func ParseAmount(input, code, locale string) (int64, error) {
	scale, known := CurrencyScale(code)
	if !known {
		return 0, errors.New("currency has no known minor-unit precision")
	}
	l := monetaryLocale(locale)
	input = strings.TrimSpace(input)
	negative := strings.HasPrefix(input, "-")
	if negative {
		input = input[1:]
	}
	for i, digit := range l.digits {
		input = strings.ReplaceAll(input, digit, strconv.Itoa(i))
	}
	parts := strings.Split(input, l.decimal)
	if len(parts) > 2 || (len(parts) == 2 && (scale == 0 || len(parts[1]) == 0 || len(parts[1]) > scale || !asciiDigits(parts[1]))) {
		return 0, errors.New("invalid fractional amount")
	}
	whole := parts[0]
	if l.group != "" && strings.Contains(whole, l.group) {
		groups := strings.Split(whole, l.group)
		for i, group := range groups {
			if !asciiDigits(group) || (i == 0 && (len(group) < 1 || len(group) > l.secondary)) || (i > 0 && i < len(groups)-1 && len(group) != l.secondary) || (i == len(groups)-1 && len(group) != l.primary) {
				return 0, errors.New("invalid digit grouping")
			}
		}
		whole = strings.Join(groups, "")
	}
	if !asciiDigits(whole) {
		return 0, errors.New("invalid amount")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	value := strings.TrimLeft(whole+fraction+strings.Repeat("0", scale-len(fraction)), "0")
	if value == "" {
		value = "0"
	}
	if negative {
		value = "-" + value
	}
	result, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount out of range: %w", err)
	}
	return result, nil
}
