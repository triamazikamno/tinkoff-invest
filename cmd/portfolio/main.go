package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
)

func main() {
	apiKey, ok := os.LookupEnv("API_KEY")
	if !ok || apiKey == "" {
		log.Fatal("please specify API_KEY variable")
		return
	}
	ti := tinkoffinvest.NewAPI(apiKey)
	positions, err := ti.Portfolio()
	if err != nil {
		log.Fatalf("failed to get portfolio: %v", err)
		return
	}
	var totalRUB, totalUSD float64
	curTime := time.Now()
	colorRUB, colorUSD := "\033[32m", "\033[32m"
	for _, pos := range positions {
		switch pos.Currency {
		case "USD":
			totalUSD += pos.Profit
		case "RUB":
			totalRUB += pos.Profit
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
	sort.Slice(positions, func(i, j int) bool {
		return positions[i].Profit > positions[j].Profit
	})
	for _, pos := range positions {
		color := "green"
		switch {
		case pos.Profit < 0 && pos.Profit > -0.8:
			color = "white"
		case pos.Profit < 0:
			color = "red"
		}

		sign := "$"
		if pos.Currency == "RUB" {
			sign = "₽"
		}
		fmt.Printf("%s: %s%.2f | color=%s\n", pos.Ticker, sign, pos.Profit, color)
	}

}
