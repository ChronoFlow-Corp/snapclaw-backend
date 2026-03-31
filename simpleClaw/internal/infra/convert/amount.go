package convert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"simpleClaw/internal/service/pkg/minor"

	"github.com/shopspring/decimal"
)

const (
	currencyUSD = "USD"
)

type Amount struct {
	rate exchangeRate
}

func NewAmount(ctx context.Context) *Amount {
	a := &Amount{}

	go func() {
		//err := a.updateRate(ctx)
		//if err != nil {
		//	slog.Default().Error("Error updating rate", "error", err)
		//}

		t := time.NewTicker(time.Minute * 5)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				err := a.updateRate(ctx)
				if err != nil {
					slog.Default().Error("Error updating rate", "error", err)
				}
			}
		}
	}()

	return a
}

func (a *Amount) ToMinor(
	totalCost string,
	sourceCurrency, targetCurrency string,
) (int64, error) {
	const op = "infra.convert.Amount.FromUSD"

	if sourceCurrency != currencyUSD {
		return 0, fmt.Errorf("%s: %w", op, errors.New("source currency not supported"))
	}

	scale := a.rate.Rates[targetCurrency]
	if scale == 0 {
		slog.Default().Warn("No USD rate found for this currency", "rate", a.rate)

		scale = 81.507585
	}

	d, err := decimal.NewFromString(totalCost)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	d = d.Truncate(2)

	d2 := decimal.NewFromFloat(scale)

	d2 = d2.Truncate(2)

	d = d.Mul(d2)

	d = d.Truncate(2)

	m, err := minor.StringToMinor(d.String(), targetCurrency)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", op, err)
	}

	return m, nil
}

func (a *Amount) updateRate(ctx context.Context) error {
	rate, err := getExchange(ctx)
	if err != nil {
		return err
	}

	a.rate = rate

	return nil
}
