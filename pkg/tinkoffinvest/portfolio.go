package tinkoffinvest

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	sdk "github.com/TinkoffCreditSystems/invest-openapi-go-sdk"
	"github.com/pkg/errors"
)

type Portfolio struct {
	TotalFee             map[Currency]float64
	TotalProfit          map[Currency]float64
	TotalPotentialProfit map[Currency]float64
	TotalDividend        map[Currency]float64
	TotalTax             map[Currency]float64
	Items                []PortfolioItem
}

func (p Portfolio) Summary() (summary string) {
	for _, currency := range Currencies {
		if currency.String() == "" {
			continue
		}
		summary += fmt.Sprintf(
			"\n%s полученная прибыль: %s (див %.2f, ком %.2f, налог %.2f)\n",
			currency.String(),
			formatMoney(currency, p.TotalProfit[currency]),
			p.TotalDividend[currency], -1*p.TotalFee[currency], p.TotalTax[currency],
		)
		summary += fmt.Sprintf(
			"%s потенциальная прибыль: %s\n",
			currency.String(),
			formatMoney(currency, p.TotalPotentialProfit[currency]),
		)
	}
	return
}

type PortfolioItem struct {
	Ticker          string
	FIGI            string
	Currency        Currency
	Profit          float64
	Tax             float64
	Dividends       float64
	Fee             float64
	Holdings        float64
	ExpectedYield   float64
	ExpectedYieldPc float64
	Trades          []Trade
	LongPositions   []float64
	ShortPositions  []float64
}

func (item PortfolioItem) TotalProfit() float64 {
	return item.Profit + item.Dividends + item.Fee + item.Tax
}

func (item PortfolioItem) Details() string {
	var details string
	details += fmt.Sprintf("*%s* \\(%s\\)\n```\n", item.Ticker, item.FIGI)

	for _, trade := range item.Trades {
		details += fmt.Sprintf(
			"%s %s (%s%.2f%%)\n",
			trade.Date.Format("2006/01/02"),
			formatMoney(item.Currency, trade.Profit), numSign(trade.ProfitPc), trade.ProfitPc,
		)
	}
	if len(item.Trades) > 0 {
		details += "\n"
	}
	details += fmt.Sprintf("Получено: %s", formatMoney(item.Currency, item.TotalProfit()))
	profitDetails := make([]string, 0)
	if item.Dividends != 0 {
		profitDetails = append(profitDetails, fmt.Sprintf("див %s", formatMoney(item.Currency, item.Dividends)))
	}
	if item.Fee != 0 {
		profitDetails = append(profitDetails, fmt.Sprintf("ком %s", formatMoney(item.Currency, item.Fee)))
	}
	if item.Tax != 0 {
		profitDetails = append(profitDetails, fmt.Sprintf("налог %s", formatMoney(item.Currency, item.Tax)))
	}
	if len(profitDetails) > 0 {
		details += fmt.Sprintf(" (%s)\n", strings.Join(profitDetails, ", "))
	} else {
		details += "\n"
	}
	if item.Holdings > 0.00001 || item.Holdings < -0.00001 {
		details += fmt.Sprintf(
			"В портфеле: %s%.2f\nПотенциал: %s (%s%.2f%%)\n",
			item.Currency.Sign(), item.Holdings,
			formatMoney(item.Currency, item.ExpectedYield),
			numSign(item.ExpectedYieldPc), item.ExpectedYieldPc,
		)
	}
	details += "```\n"
	return details
}

func (ti *TinkoffInvest) PortfolioPositions(ctx context.Context) (map[string]sdk.PositionBalance, error) {
	allPositions, err := ti.RestClient.PositionsPortfolio(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get portfolio positions")
	}

	portfolio := make(map[string]sdk.PositionBalance)
	for _, position := range allPositions {
		portfolio[position.FIGI] = position
	}

	return portfolio, nil
}

func (ti *TinkoffInvest) Portfolio(ctx context.Context) (Portfolio, error) {
	p := Portfolio{
		TotalFee:             make(map[Currency]float64),
		TotalProfit:          make(map[Currency]float64),
		TotalPotentialProfit: make(map[Currency]float64),
		TotalDividend:        make(map[Currency]float64),
		TotalTax:             make(map[Currency]float64),
		Items:                make([]PortfolioItem, 0),
	}

	portfolio, err := ti.PortfolioPositions(ctx)
	if err != nil {
		return p, errors.Wrap(err, "failed to get portfolio positions")
	}

	allStocks, err := ti.RestClient.Stocks(ctx)
	if err != nil || len(allStocks) == 0 {
		return p, errors.Wrap(err, "failed to get stocks")
	}
	allBonds, err := ti.RestClient.Bonds(ctx)
	if err != nil || len(allBonds) == 0 {
		return p, errors.Wrap(err, "failed to get bonds")
	}
	allEtfs, err := ti.RestClient.ETFs(ctx)
	if err != nil || len(allEtfs) == 0 {
		return p, errors.Wrap(err, "failed to get etfs")
	}
	stocks := make(map[string]sdk.Instrument)
	bonds := make(map[string]struct{})
	tickers := make(map[string]string)
	for _, stock := range allStocks {
		stocks[stock.FIGI] = stock
		tickers[stock.Ticker] = stock.FIGI
	}
	for _, stock := range allBonds {
		stocks[stock.FIGI] = stock
		tickers[stock.Ticker] = stock.FIGI
		bonds[stock.FIGI] = struct{}{}
	}
	for _, stock := range allEtfs {
		stocks[stock.FIGI] = stock
		tickers[stock.Ticker] = stock.FIGI
	}
	rawOperations, err := ti.RestClient.Operations(ctx, time.Now().Add(-1*5*24*365*time.Hour), time.Now(), "")
	if err != nil {
		return p, errors.Wrap(err, "failed to get list of operations")
	}
	operations := make(map[string][]sdk.Operation)
	for _, rawOp := range rawOperations {
		if rawOp.Status != sdk.OperationStatusDone || rawOp.InstrumentType == sdk.InstrumentTypeCurrency {
			continue
		}
		var ticker string
		if stock, ok := stocks[rawOp.FIGI]; ok {
			ticker = stock.Ticker
		}
		ops, ok := operations[ticker]
		if !ok {
			ops = make([]sdk.Operation, 0)
		}
		switch rawOp.OperationType {
		case sdk.BUY, sdk.OperationTypeBuyCard, sdk.SELL, sdk.OperationTypeDividend:
			ops = append(ops, rawOp)
		case sdk.OperationTypeTax, sdk.OperationTypeTaxDividend, sdk.OperationTypeTaxBack:
			ops = append(ops, rawOp)
		case sdk.OperationTypeBrokerCommission, sdk.OperationTypePayIn, sdk.OperationTypePayOut:
			// ignore
			continue
		default:
			fmt.Printf("UNKNOWN OPERATION TYPE: %s: %+v\n", stocks[rawOp.FIGI].Ticker, rawOp)
			continue
		}
		operations[ticker] = ops
	}
	allInstruments := make([]string, 0, len(operations))
	for ticker := range operations {
		allInstruments = append(allInstruments, ticker)
	}
	sort.Strings(allInstruments)

	for _, ticker := range allInstruments {
		item := PortfolioItem{
			Ticker:         ticker,
			FIGI:           tickers[ticker],
			Currency:       Currency(stocks[tickers[ticker]].Currency),
			Trades:         make([]Trade, 0),
			LongPositions:  make([]float64, 0),
			ShortPositions: make([]float64, 0),
		}
		ops := operations[ticker]
		sort.Slice(ops, func(i, j int) bool {
			return ops[i].DateTime.Before(ops[j].DateTime)
		})

		for _, op := range ops {
			item.Fee += op.Commission.Value
			switch op.OperationType {
			case sdk.BUY, sdk.OperationTypeBuyCard:
				if len(op.Trades) == 0 {
					continue
				}
				var profitPc, profit float64
				var positionsClosed int
				for _, trade := range op.Trades {
					for i := 0; i < trade.Quantity; i++ {
						if len(item.ShortPositions) > 0 {
							positionsClosed++
							pos := item.ShortPositions[0]
							item.ShortPositions = item.ShortPositions[1:]
							profitPc += pos*100/trade.Price - 100
							item.Profit += pos - trade.Price
							profit += pos - trade.Price
						} else {
							item.LongPositions = append(item.LongPositions, trade.Price)
						}
					}
				}
				if positionsClosed > 0 {
					item.Trades = append(item.Trades, Trade{
						Date:     op.DateTime,
						Type:     "закрытие шорта",
						ProfitPc: profitPc / float64(positionsClosed),
						Profit:   profit,
					})
				}
			case sdk.SELL:
				if len(op.Trades) == 0 {
					continue
				}
				var profitPc, profit float64
				var positionsClosed int
				for _, trade := range op.Trades {
					for i := 0; i < trade.Quantity; i++ {
						if len(item.LongPositions) == 0 {
							item.ShortPositions = append(item.ShortPositions, trade.Price)
						} else {
							positionsClosed++
							pos := item.LongPositions[0]
							item.LongPositions = item.LongPositions[1:]
							profitPc += trade.Price*100/pos - 100
							item.Profit += trade.Price - pos
							profit += trade.Price - pos
						}
					}
				}
				if positionsClosed > 0 {
					item.Trades = append(item.Trades, Trade{
						Date:     op.DateTime,
						Type:     "продажа",
						ProfitPc: profitPc / float64(positionsClosed),
						Profit:   profit,
					})
				}
			case sdk.OperationTypeDividend:
				item.Dividends += op.Payment
			case sdk.OperationTypeTax, sdk.OperationTypeTaxBack, sdk.OperationTypeTaxDividend:
				p.TotalTax[Currency(op.Currency)] -= op.Payment
				if ticker != "" {
					item.Profit += op.Payment
					item.Tax += op.Payment
				}
			}
		}
		p.Items = append(p.Items, item)
	}
	for i, item := range p.Items {
		p.TotalFee[item.Currency] += item.Fee
		p.TotalProfit[item.Currency] += item.TotalProfit()
		p.TotalDividend[item.Currency] += item.Dividends
		if item.Ticker == "" {
			continue
		}
		if position, ok := portfolio[item.FIGI]; ok && position.Lots > 0 {
			item.Holdings = position.AveragePositionPrice.Value * float64(position.Lots)

			if _, ok := bonds[item.FIGI]; !ok && item.Holdings == 0 {

				for _, pos := range item.LongPositions {
					item.Holdings += pos
				}
				for _, pos := range item.ShortPositions {
					item.Holdings -= pos
				}
				orderbook, err := ti.RestClient.Orderbook(ctx, 1, item.FIGI)
				if err != nil {
					return p, errors.Wrapf(err, "failed to get order book for %s", item.Ticker)
				}
				item.ExpectedYield = float64(len(item.LongPositions))*orderbook.LastPrice - float64(len(item.ShortPositions))*orderbook.LastPrice - item.Holdings
			} else {
				item.ExpectedYield = position.ExpectedYield.Value
			}

			item.ExpectedYieldPc = item.ExpectedYield * 100 / item.Holdings
			p.TotalPotentialProfit[item.Currency] += item.ExpectedYield
			p.Items[i] = item
		}
	}
	return p, nil
}

func formatMoney(cur Currency, val float64) string {
	var sign string
	switch {
	case val > 0:
		sign = "+"
	case val < 0:
		sign = "-"
	}
	return fmt.Sprintf("%s%s%.2f", sign, cur.Sign(), math.Abs(val))
}

func numSign(val float64) string {
	if val > 0 {
		return "+"
	}
	return ""
}
