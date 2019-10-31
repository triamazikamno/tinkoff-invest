package tinkoffinvest

import (
	"encoding/json"
	"net/http"

	"github.com/pkg/errors"
)

type TinkoffInvest struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

type Position struct {
	Ticker   string
	Currency string
	Profit   float64
}

func NewAPI(apiKey string) *TinkoffInvest {
	t := &TinkoffInvest{
		apiKey:     apiKey,
		endpoint:   "https://api-invest.tinkoff.ru/openapi",
		httpClient: http.DefaultClient,
	}
	return t
}

func (t *TinkoffInvest) Portfolio() ([]Position, error) {
	req, err := http.NewRequest(http.MethodGet, t.endpoint+"/portfolio", nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create req")
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Accept", "application/json")
	res, err := t.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make request")
	}
	defer res.Body.Close()
	var result struct {
		Payload struct {
			Positions []struct {
				Ticker         string
				InstrumentType string
				Balance        float64
				ExpectedYield  struct {
					Currency string
					Value    float64
				}
				AveragePositionPrice struct {
					Currency string
					Value    float64
				}
			}
		}
	}
	err = json.NewDecoder(res.Body).Decode(&result)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}
	positions := make([]Position, 0)
	for _, pos := range result.Payload.Positions {
		if pos.InstrumentType == "Currency" {
			continue
		}
		positions = append(positions, Position{
			Ticker:   pos.Ticker,
			Currency: pos.ExpectedYield.Currency,
			Profit:   pos.ExpectedYield.Value,
		})
	}
	return positions, nil
}
