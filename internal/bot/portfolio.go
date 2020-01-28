package bot

import (
	"context"
	"fmt"

	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
)

func (bot *Bot) handleApiKey(ctx context.Context, chatID int64, args []string) {
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

func (bot *Bot) handlePortfolioSummary(ctx context.Context, chatID int64) {
	apiKey := bot.fetchApiKey(chatID, true)
	if apiKey == "" {
		return
	}
	ti := tinkoffinvest.NewAPI(apiKey)
	accounts, err := ti.RestClient.Accounts(ctx)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Ошибка получения счетов(%v)", err))
		return
	}
	for _, acc := range accounts {
		portfolio, err := ti.Portfolio(ctx, acc.ID)
		if err != nil {
			bot.sendError(chatID, fmt.Sprintf("Ошибка получения информации о портфеле(%v)", err))
			return
		}

		bot.sendText(chatID, string(acc.Type)+":\n"+markDownEscape.Replace(portfolio.Summary()), true)
	}
}

func (bot *Bot) handlePortfolioDetails(ctx context.Context, chatID int64) {
	apiKey := bot.fetchApiKey(chatID, true)
	if apiKey == "" {
		return
	}
	ti := tinkoffinvest.NewAPI(apiKey)
	accounts, err := ti.RestClient.Accounts(ctx)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Ошибка получения счетов(%v)", err))
		return
	}
	for _, acc := range accounts {
		portfolio, err := ti.Portfolio(ctx, acc.ID)
		if err != nil {
			bot.sendError(chatID, fmt.Sprintf("Ошибка получения информации о портфеле(%v)", err))
			return
		}
		msg := string(acc.Type) + ":\n"
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
}
