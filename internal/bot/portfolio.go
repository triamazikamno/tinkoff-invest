package bot

import (
	"context"
	"fmt"

	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
)

func (bot *Bot) HandleApiKey(ctx context.Context, chatID int64, args []string) {
	if len(args) == 0 {
		bot.sendText(chatID, "Ошибка: отсутствует API ключ\\. [Инструкция по получению](https://tinkoffcreditsystems.github.io/invest-openapi/auth/#_2)\\.\nПример: */apikey t\\.xwbNB\\-uFZ4DSHG3Hk5kFkdk2kGDOpW4*", true)
		return
	}
	if bot.db.IsSet() {
		err := bot.db.SetApiKey(chatID, args[0])
		if err != nil {
			bot.sendError(chatID, fmt.Sprintf("Ошибка записи ключа: %v", err))
			return
		}
	}

	bot.sendText(chatID, "Принято", false)
}

func (bot *Bot) HandlePortfolioSummary(ctx context.Context, chatID int64) {
	apiKey := bot.fetchApiKey(chatID, true)
	if apiKey == "" {
		return
	}
	ti := tinkoffinvest.NewAPI(apiKey)
	portfolio, err := ti.Portfolio(ctx)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Ошибка получения информации о портфеле(%v)", err))
		return
	}

	bot.sendText(chatID, markDownEscape.Replace(portfolio.Summary()), true)
}

func (bot *Bot) HandlePortfolioDetails(ctx context.Context, chatID int64) {
	apiKey := bot.fetchApiKey(chatID, true)
	if apiKey == "" {
		return
	}
	ti := tinkoffinvest.NewAPI(apiKey)
	portfolio, err := ti.Portfolio(ctx)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Ошибка получения информации о портфеле(%v)", err))
		return
	}
	var msg string
	for _, item := range portfolio.Items {
		if item.Ticker == "" {
			continue
		}
		details := item.Details() + "\n"
		if len(msg)+len(details) >= 4096 {
			bot.sendText(chatID, msg, true)
			msg = ""
		}
		msg += details
	}
	if msg != "" {
		bot.sendText(chatID, msg, true)
	}
}