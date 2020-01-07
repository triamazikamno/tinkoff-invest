package main

import (
	"context"
	"fmt"
	"log"
	"os"
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
	ctx := context.Background()
	allStocks, err := ti.Stocks(ctx)
	if err != nil || len(allStocks) == 0 {
		log.Fatalf("failed to get stocks: %v", err)
		return
	}
	stocks := make(map[sdk.Currency]map[string]sdk.Instrument)
	for _, stock := range allStocks {
		curStocks, ok := stocks[stock.Currency]
		if !ok {
			curStocks = make(map[string]sdk.Instrument)
		}
		curStocks[stock.Ticker] = stock
		stocks[stock.Currency] = curStocks
	}
	for currency, curStocks := range stocks {
		fmt.Printf("%s: %d\n", currency, len(curStocks))
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	usdStocks, ok := stocks["USD"]
	if !ok {
		log.Fatal("no usd stocks found")
		return
	}
	for stockTicker, stock := range usdStocks {
		for {
			<-ticker.C
			orderBook, err := ti.Orderbook(ctx, 1, stock.FIGI)
			if err != nil {
				log.Printf("failed to get order book for %s: %v", stockTicker, err)
				continue
			}
			if len(orderBook.Bids) > 0 && len(orderBook.Asks) > 0 {
				spread := (orderBook.Asks[0].Price * 100 / orderBook.Bids[0].Price) - 100
				bidFromClose := (orderBook.Bids[0].Price * 100 / orderBook.ClosePrice) - 100
				askFromClose := (orderBook.Asks[0].Price * 100 / orderBook.ClosePrice) - 100
				if spread >= 2.5 && bidFromClose < -1.5 {
					fmt.Printf("spread=%.2f bid=%.2f ask=%.2f\t%s: %+v\n", spread, bidFromClose, askFromClose, stockTicker, orderBook)
				}
			}
			break
		}
	}
}
