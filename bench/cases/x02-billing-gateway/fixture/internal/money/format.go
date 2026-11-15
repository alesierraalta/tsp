package money

import (
	"fmt"
	"strconv"
	"strings"
)

// minorDigits is the number of decimal digits in the minor unit of a currency. The service bills in
// currencies whose minor unit is two digits; a currency with none needs a different reader and is
// refused rather than formatted wrongly.
func minorDigits(currency string) (int, error) {
	switch currency {
	case "EUR", "USD", "GBP", "CHF", "SEK", "NOK", "DKK", "PLN", "CZK", "BRL", "CAD", "AUD":
		return 2, nil
	case "JPY", "KRW", "CLP":
		return 0, nil
	}
	return 0, fmt.Errorf("%w: no minor unit is defined for %q", ErrCurrency, currency)
}

// formatMinor renders a count of minor units with its currency and grouping: 123456 EUR becomes
// "1,234.56 EUR". A group separator is a display decision and never enters a stored value.
func formatMinor(minor int64, currency string) string {
	digits, err := minorDigits(currency)
	if err != nil {
		return strconv.FormatInt(minor, 10) + " " + currency
	}
	sign := ""
	value := minor
	if value < 0 {
		sign = "-"
		// -value overflows for the smallest int64; the digits are rendered from a uint64 instead.
		return sign + formatMinorUnsigned(uint64(-(value+1))+1, digits) + " " + currency
	}
	return sign + formatMinorUnsigned(uint64(value), digits) + " " + currency
}

// formatMinorUnsigned renders the absolute count of minor units with grouping.
func formatMinorUnsigned(value uint64, digits int) string {
	whole := value
	frac := uint64(0)
	if digits > 0 {
		scale := uint64(1)
		for i := 0; i < digits; i++ {
			scale *= 10
		}
		whole = value / scale
		frac = value % scale
	}
	text := strconv.FormatUint(whole, 10)
	grouped := group(text)
	if digits == 0 {
		return grouped
	}
	fraction := strconv.FormatUint(frac, 10)
	for len(fraction) < digits {
		fraction = "0" + fraction
	}
	return grouped + "." + fraction
}

// group inserts a comma every three digits from the right.
func group(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
		if len(digits) > lead {
			b.WriteString(",")
		}
	}
	for i := lead; i < len(digits); i += 3 {
		b.WriteString(digits[i : i+3])
		if i+3 < len(digits) {
			b.WriteString(",")
		}
	}
	return b.String()
}

// Parse reads an amount a client wrote as text: "12.34 EUR", "12,34 EUR" or "1234 EUR" in a currency
// with no minor unit. A value with more decimals than the currency carries is refused rather than
// rounded, because a client that wrote three decimals meant something the service cannot store.
func Parse(text string) (Amount, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Amount{}, fmt.Errorf("money: an amount is not empty")
	}
	fields := strings.Fields(trimmed)
	if len(fields) != 2 {
		return Amount{}, fmt.Errorf("money: %q is not an amount and a currency", text)
	}
	number, currency := fields[0], fields[1]
	if strings.HasSuffix(number, ",") {
		return Amount{}, fmt.Errorf("money: %q ends with a separator", text)
	}
	digits, err := minorDigits(strings.ToUpper(currency))
	if err != nil {
		return Amount{}, err
	}
	// A comma is a decimal separator in most of the currencies this service bills in.
	number = strings.ReplaceAll(number, ",", ".")
	negative := strings.HasPrefix(number, "-")
	number = strings.TrimPrefix(number, "-")
	whole, fraction, found := strings.Cut(number, ".")
	if whole == "" {
		whole = "0"
	}
	if found && len(fraction) > digits {
		return Amount{}, fmt.Errorf("money: %q carries more than %d decimals", text, digits)
	}
	for len(fraction) < digits {
		fraction += "0"
	}
	units, err := strconv.ParseUint(whole, 10, 64)
	if err != nil {
		return Amount{}, fmt.Errorf("money: %q is not a number", text)
	}
	frac := uint64(0)
	if fraction != "" {
		frac, err = strconv.ParseUint(fraction, 10, 64)
		if err != nil {
			return Amount{}, fmt.Errorf("money: %q is not a number", text)
		}
	}
	scale := uint64(1)
	for i := 0; i < digits; i++ {
		scale *= 10
	}
	if units > uint64(maxInt64)/scale {
		return Amount{}, ErrOverflow
	}
	minor := units*scale + frac
	if minor > uint64(maxInt64) {
		return Amount{}, ErrOverflow
	}
	stored, err := New(int64(minor), currency)
	if err != nil {
		return Amount{}, err
	}
	if negative {
		return stored.Neg()
	}
	return stored, nil
}
