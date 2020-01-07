package pricewatch

import (
	"fmt"
	"strings"

	"github.com/dustin/go-humanize"
)

type Signer interface {
	String() string
	Sign() string
}

type PriceWatch struct {
	ID            int64
	ChatID        int64
	FIGI          string
	Ticker        string
	TickerURL     string
	LastValue     float64
	CurrentValue  float64
	Threshold     float64
	PortfolioGain float64
	Currency      Signer
	IsPc          bool
	IsPermanent   bool
}

func (p PriceWatch) Pc() float64 {
	pc := 0.0
	lastValue := p.LastValue
	if !p.IsPc {
		lastValue = p.Threshold
	}
	if lastValue != 0 && p.CurrentValue != 0 {
		pc = p.CurrentValue*100/lastValue - 100
	}
	return pc
}

func (p PriceWatch) String() string {
	pc := p.Pc()
	var portfolioGain string
	if p.PortfolioGain != 0 {
		portfolioGain = fmt.Sprintf("(%s%.2f%%)", numSign(p.PortfolioGain), p.PortfolioGain)
	}
	ticker := "$" + strings.ToUpper(p.Ticker)
	if p.TickerURL != "" {
		ticker = p.TickerURL
	}
	return fmt.Sprintf(
		"%s\n`     %-6s %-7s %s`",
		ticker,
		numSign(pc)+humanize.FormatFloat("", pc)+"%",
		p.Currency.Sign()+humanize.Commaf(p.CurrentValue),
		portfolioGain,
	)
}

// Arrow returns up arrow for positive value, down for negative and sideways for 0
func Arrow(num float64) (a string) {
	a = "⬌"
	if num > 0 {
		a = "⬆"
	} else if num < 0 {
		a = "⬇"
	}
	return
}

func numSign(val float64) string {
	if val > 0 {
		return "+"
	}
	if val == 0 {
		return " "
	}
	return ""
}
