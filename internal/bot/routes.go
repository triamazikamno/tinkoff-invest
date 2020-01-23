package bot

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

func (bot *Bot) listenUpdates(ch tgbotapi.UpdatesChannel) {
	for update := range ch {
		var command string
		var args []string
		var chatID int64
		var chat *tgbotapi.Chat
		var user *tgbotapi.User
		switch {
		case update.Message != nil:
			command = update.Message.Command()
			args = strings.Fields(update.Message.CommandArguments())
			chatID = update.Message.Chat.ID
			chat = update.Message.Chat
			user = update.Message.From
		case update.CallbackQuery != nil:
			command = update.CallbackQuery.Data
			chatID = update.CallbackQuery.Message.Chat.ID
			_, _ = bot.tg.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data))
		}
		if command == "" {
			if msg := update.EditedMessage; msg != nil {
				command = msg.Command()
				args = strings.Fields(msg.CommandArguments())
				chatID = msg.Chat.ID
				chat = msg.Chat
				user = msg.From
			}
		}

		if command == "" {
			continue
		}

		bot.log.Info().
			Interface("user", user).
			Interface("chat", chat).
			Str("command", command).
			Strs("args", args).
			Msg("incoming command")

		switch command {
		case "start", "help":
			bot.handleHelp(chatID)
		case "stop":
			bot.handleStop(chatID)
		case "apikey":
			bot.handleApiKey(context.Background(), chatID, args)
		case "gainers", "g":
			bot.handleGainers(context.Background(), chatID, args)
		case "losers", "l":
			bot.handleLosers(context.Background(), chatID, args)
		case "watchglobal", "wg":
			bot.handleWatchGlobal(context.Background(), chatID, args)
		case "watch", "w":
			bot.handleWatch(context.Background(), chatID, args)
		case "watchlist", "wl":
			bot.handleWatchList(context.Background(), chatID)
		case "watchdelete", "wd":
			bot.handleWatchDelete(context.Background(), chatID, args)
		case "sum", "summary":
			bot.handlePortfolioSummary(context.Background(), chatID)
		case "full", "fullreport":
			bot.handlePortfolioDetails(context.Background(), chatID)
		case "i", "info":
			bot.handleInfo(context.Background(), chatID, args)
		default:
			bot.log.Warn().Int64("chatID", chatID).Str("command", command).Strs("args", args).Msg("unknown command")
		}
	}
}
