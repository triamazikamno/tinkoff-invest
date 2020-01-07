package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	sdk "github.com/TinkoffCreditSystems/invest-openapi-go-sdk"
)

func main() {
	apiKey, ok := os.LookupEnv("API_KEY")
	if !ok || apiKey == "" {
		log.Fatal("please specify API_KEY variable")
		return
	}
	ti := sdk.NewRestClient(apiKey)
	positions, err := ti.Portfolio(context.Background())
	if err != nil {
		log.Fatalf("failed to get portfolio: %v", err)
		return
	}
	var totalRUB, totalUSD float64
	curTime := time.Now()
	colorRUB, colorUSD := "\033[32m", "\033[32m"
	for _, pos := range positions.Positions {
		if pos.InstrumentType == "Currency" {
			continue
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
	fmt.Printf("%s₽%.2f\033[0m %s$%.2f\033[0m\n---\n", colorRUB, totalRUB, colorUSD, totalUSD)
	sort.Slice(positions.Positions, func(i, j int) bool {
		return positions.Positions[i].ExpectedYield.Value > positions.Positions[j].ExpectedYield.Value
	})
	for _, pos := range positions.Positions {
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

		sign := "$"
		if pos.ExpectedYield.Currency == "RUB" {
			sign = "₽"
		}
		fmt.Printf("%s: %s%.2f | color=%s\n", pos.Ticker, sign, pos.ExpectedYield.Value, color)
	}

}
