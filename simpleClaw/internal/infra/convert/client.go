package convert

import (
	"context"
	"encoding/json"
	"net/http"
)

var apiUrl = "https://open.er-api.com/v6/latest/USD"

func getExchange(ctx context.Context) (exchangeRate, error) {
	cl := http.DefaultClient

	rq, err := http.NewRequestWithContext(ctx, "GET", apiUrl, nil)
	if err != nil {
		return exchangeRate{}, nil
	}

	resp, err := cl.Do(rq)
	if err != nil {
		return exchangeRate{}, err
	}
	defer resp.Body.Close()

	var rates exchangeRate

	err = json.NewDecoder(resp.Body).Decode(&rates)
	if err != nil {
		return exchangeRate{}, err
	}

	return rates, nil
}
