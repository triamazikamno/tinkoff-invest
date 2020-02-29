package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	sdk "github.com/TinkoffCreditSystems/invest-openapi-go-sdk"
	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
)

func main() {
	apiKey, ok := os.LookupEnv("API_KEY")
	if !ok || apiKey == "" {
		log.Fatal("please specify API_KEY variable")
		return
	}
	ti := sdk.NewRestClient(apiKey)
	positions, err := ti.Portfolio(context.Background(), sdk.DefaultAccount)
	if err != nil {
		log.Fatalf("failed to get portfolio: %v", err)
		return
	}
	var totalRUB, totalUSD float64
	curTime := time.Now()
	colorRUB, colorUSD := "\033[32m", "\033[32m"
	data := map[string][]sdk.PositionBalance{
		"USD": make([]sdk.PositionBalance, 0),
		"RUB": make([]sdk.PositionBalance, 0),
		"EUR": make([]sdk.PositionBalance, 0),
	}
	for _, pos := range positions.Positions {
		if pos.InstrumentType == "Currency" {
			continue
		}
		if _, ok := data[string(pos.ExpectedYield.Currency)]; ok {
			data[string(pos.ExpectedYield.Currency)] = append(data[string(pos.ExpectedYield.Currency)], pos)
		}
		switch pos.ExpectedYield.Currency {
		case "USD":
			totalUSD += pos.ExpectedYield.Value
		case "RUB":
			totalRUB += pos.ExpectedYield.Value
		}
	}
	if totalUSD < 0 {
		colorUSD = "\033[31m"
	}
	if totalRUB < 0 {
		colorRUB = "\033[31m"
	}
	if wd := curTime.Weekday(); (curTime.Hour() >= 2 && curTime.Hour() < 10) || wd == time.Saturday || wd == time.Sunday {
		colorUSD = "\033[0m"
		colorRUB = "\033[0m"
	}
	fmt.Printf("%s₽%.2f\033[0m %s$%.2f\033[0m\n", colorRUB, totalRUB, colorUSD, totalUSD)
	for currency, items := range data {
		items := items
		sort.Slice(items, func(i, j int) bool {
			return items[i].ExpectedYield.Value > items[j].ExpectedYield.Value
		})
		fmt.Println("---")
		for _, pos := range items {
			if pos.InstrumentType == "Currency" {
				continue
			}

			color := "green"
			switch {
			case pos.ExpectedYield.Value < 0 && pos.ExpectedYield.Value > -0.8:
				color = "white"
			case pos.ExpectedYield.Value < 0:
				color = "red"
			}

			sign := tinkoffinvest.Currency(currency).Sign()
			fmt.Printf("%s: %s%.2f | color=%s\n", pos.Ticker, sign, pos.ExpectedYield.Value, color)
		}
	}
}
