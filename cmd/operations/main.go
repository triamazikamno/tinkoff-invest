package main

import (
	"context"
	"net/http"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jackc/pgx"
	"github.com/rs/zerolog"
	"github.com/triamazikamno/tinkoff-invest/internal/bot"
	"github.com/triamazikamno/tinkoff-invest/internal/db"
	"gopkg.in/alecthomas/kingpin.v2"
)

var apiKey = kingpin.Flag("api-key", "Default API key for anonymous requests").String()
var logPath = kingpin.Flag("log", "Log file path").Default("/var/log/tinkoffbot.log").String()
var tgBotApiKey = kingpin.Flag("tg-bot-api-key", "Telegram bot API key").String()
var listen = kingpin.Flag("listen", "HTTP listen host:port").String()
var hostURL = kingpin.Flag("host-url", "External host URL").String()
var postgresUser = kingpin.Flag("postgres-user", "Postgresql user name").String()
var postgresPassword = kingpin.Flag("postgres-password", "Postgresql password").String()
var postgresHost = kingpin.Flag("postgres-host", "Postgresql host").String()
var postgresDatabase = kingpin.Flag("postgres-db", "Postgresql database").String()

func main() {
	kingpin.Parse()
	f, err := os.OpenFile(*logPath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		panic("failed to open log file for writing")
	}
	log := zerolog.New(f).With().Timestamp().Logger()
	if *tgBotApiKey != "" {
		tbot, err := tgbotapi.NewBotAPI(*tgBotApiKey)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to start telegram bot")
		}
		var updates tgbotapi.UpdatesChannel
		pgConfig := new(pgx.ConnConfig)
		pgConfig.TLSConfig = nil
		connPoolConfig := pgx.ConnPoolConfig{
			ConnConfig: pgx.ConnConfig{
				Host:     *postgresHost,
				User:     *postgresUser,
				Password: *postgresPassword,
				Database: *postgresDatabase,
			},
			MaxConnections: 15,
		}
		pg, err := pgx.NewConnPool(connPoolConfig)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to connect to db")
		}
		database := db.NewDatabase(pg)

		if *hostURL != "" && *listen != "" {
			_, err = tbot.SetWebhook(tgbotapi.NewWebhook(*hostURL + *tgBotApiKey))
			if err != nil {
				log.Fatal().Err(err).Msg("failed to send webhook")
			}
			updates = tbot.ListenForWebhook("/" + *tgBotApiKey)
			s := &http.Server{
				Addr: *listen,
			}
			go func() {
				_ = s.ListenAndServe()
			}()
		} else {
			u := tgbotapi.NewUpdate(0)
			u.Timeout = 60
			updates, err = tbot.GetUpdatesChan(u)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to subscribe to tg updates")
			}
		}
		botapi := bot.NewBot(database, tbot, log, *apiKey)
		botapi.Start()
		allPriceWatchers, err := database.PriceWatchList(0)
		if err != nil {
			log.Error().Err(err).Msg("failed to get price watchers")
		} else {
			seen := make(map[int64]bool)
			for _, pw := range allPriceWatchers {
				if _, ok := seen[pw.ChatID]; ok {
					continue
				}
				seen[pw.ChatID] = true
				log.Info().Int64("chatID", pw.ChatID).Msg("starting streaming")
				botapi.StreamingWorker(pw.ChatID)
			}
		}

		for update := range updates {
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
				_, _ = tbot.AnswerCallbackQuery(tgbotapi.NewCallback(update.CallbackQuery.ID, update.CallbackQuery.Data))
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

			log.Info().
				Interface("user", user).
				Interface("chat", chat).
				Str("command", command).
				Strs("args", args).
				Msg("incoming command")

			switch command {
			case "start", "help":
				botapi.HandleHelp(chatID)
			case "stop":
				botapi.HandleStop(chatID)
			case "apikey":
				botapi.HandleApiKey(context.Background(), chatID, args)
			case "gainers", "g":
				botapi.HandleGainers(context.Background(), chatID, args)
			case "losers", "l":
				botapi.HandleLosers(context.Background(), chatID, args)
			case "watchglobal", "wg":
				botapi.HandleWatchGlobal(context.Background(), chatID, args)
			case "watch", "w":
				botapi.HandleWatch(context.Background(), chatID, args)
			case "watchlist", "wl":
				botapi.HandleWatchList(context.Background(), chatID)
			case "watchdelete", "wd":
				botapi.HandleWatchDelete(context.Background(), chatID, args)
			case "sum", "summary":
				botapi.HandlePortfolioSummary(context.Background(), chatID)
			case "full", "fullreport":
				botapi.HandlePortfolioDetails(context.Background(), chatID)
			case "i", "info":
				botapi.HandleInfo(context.Background(), chatID, args)
			default:
				log.Warn().Int64("chatID", chatID).Str("command", command).Strs("args", args).Msg("unknown command")
			}
		}
	}
}
