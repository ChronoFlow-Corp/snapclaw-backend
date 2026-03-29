package minor

import (
	"fmt"

	"github.com/shopspring/decimal"
)

var currencyScale = map[string]int32{
	"RUB": 2,
	"USD": 2,
	"EUR": 2,
	"JPY": 0,
	"KWD": 3,
}

func StringToMinor(amount string, currency string) (int64, error) {
	d, err := decimal.NewFromString(amount)
	if err != nil {
		return 0, fmt.Errorf("parse amount: %w", err)
	}

	scale, ok := currencyScale[currency]
	if !ok {
		return 0, fmt.Errorf("unknown currency: %s", currency)
	}

	multiplier := decimal.New(1, scale) // 10^scale
	minor := d.Mul(multiplier)

	if !minor.Equal(minor.Truncate(0)) {
		return 0, fmt.Errorf("too many fractional digits for scale=%d", scale)
	}

	return minor.IntPart(), nil
}

func MinorToString(amount int64, currency string) (string, error) {
	scale, ok := currencyScale[currency]
	if !ok {
		return "", fmt.Errorf("unknown currency: %s", currency)
	}

	d := decimal.NewFromInt(amount).Div(decimal.New(1, scale))

	return d.StringFixed(scale), nil
}
