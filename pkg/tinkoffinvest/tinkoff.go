package tinkoffinvest

import (
	"context"
	"math/rand"
	"time"

	sdk "github.com/TinkoffCreditSystems/invest-openapi-go-sdk"
)

type TinkoffInvest struct {
	RestClient *sdk.RestClient
}

func NewAPI(apiKey string) *TinkoffInvest {
	t := &TinkoffInvest{
		RestClient: sdk.NewRestClient(apiKey),
	}

	return t
}

func (ti *TinkoffInvest) InstrumentByTicker(ctx context.Context, ticker string) (sdk.SearchInstrument, error) {
	instruments, err := ti.RestClient.SearchInstrumentByTicker(ctx, ticker)
	if err != nil {
		return sdk.SearchInstrument{}, err
	}
	for _, instrument := range instruments {
		if instrument.Ticker == ticker {
			return instrument, nil
		}
	}
	return sdk.SearchInstrument{}, nil
}

type Trade struct {
	Date     time.Time
	Type     string
	Profit   float64
	ProfitPc float64
}

var Currencies = []Currency{
	RUB,
	USD,
	EUR,
}

type Currency string

const (
	RUB Currency = "RUB"
	USD Currency = "USD"
	EUR Currency = "EUR"
)

func (c Currency) Sign() string {
	switch c {
	case RUB:
		return "₽"
	case USD:
		return "$"
	case EUR:
		return "€"
	default:
		return string(c)
	}
}

func (c Currency) String() string {
	return string(c)
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func requestID() string {
	b := make([]rune, 12)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}

	return string(b)
}
