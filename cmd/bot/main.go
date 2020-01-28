package main

import (
	"net/http"
	"os"
	"runtime"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jackc/pgx"
	"github.com/rs/zerolog"
	"github.com/triamazikamno/tinkoff-invest/internal/bot"
	"github.com/triamazikamno/tinkoff-invest/internal/db"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	apiKey           = kingpin.Flag("api-key", "Default API key for anonymous requests").String()
	logPath          = kingpin.Flag("log", "Log file path").Default("/var/log/tinkoffbot.log").String()
	tgBotApiKey      = kingpin.Flag("tg-bot-api-key", "Telegram bot API key").String()
	listen           = kingpin.Flag("listen", "HTTP listen host:port").String()
	hostURL          = kingpin.Flag("host-url", "External host URL").String()
	postgresUser     = kingpin.Flag("postgres-user", "Postgresql user name").String()
	postgresPassword = kingpin.Flag("postgres-password", "Postgresql password").String()
	postgresHost     = kingpin.Flag("postgres-host", "Postgresql host").String()
	postgresDatabase = kingpin.Flag("postgres-db", "Postgresql database").String()
)

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
		botapi.Start(updates)
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
	}
	runtime.Goexit()
}
