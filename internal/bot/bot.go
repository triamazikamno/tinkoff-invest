package bot

import (
	"context"
	"fmt"
	"strings"
	"sync"
	time "time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	pgx "github.com/jackc/pgx"
	"github.com/rs/zerolog"
	db "github.com/triamazikamno/tinkoff-invest/internal/db"
	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
)

var loc *time.Location

func init() {
	var err error
	loc, err = time.LoadLocation("Europe/Moscow")
	if err != nil {
		loc = time.UTC
	}
}

var markDownEscape = strings.NewReplacer("+", `\+`, ".", `\.`, "(", `\(`, ")", `\)`, "-", `\-`)

type TinkoffAPI interface {
	FIGI(ctx context.Context, ticker string) (string, error)
}

type Bot struct {
	tg                 *tgbotapi.BotAPI
	db                 db.Database
	streamingClients   map[int64]*tinkoffinvest.StreamingClient
	streamingClientsMu sync.Mutex
	log                zerolog.Logger
	defaultApiKey      string
	dataCache          dataCache
	earners            earners
}

func NewBot(db db.Database, tbot *tgbotapi.BotAPI, log zerolog.Logger, defaultApiKey string) *Bot {
	bot := &Bot{
		db:               db,
		tg:               tbot,
		streamingClients: make(map[int64]*tinkoffinvest.StreamingClient),
		log:              log,
		defaultApiKey:    defaultApiKey,
	}
	return bot
}

func (bot *Bot) HandleHelp(chatID int64) {
	bot.sendText(
		chatID,
		`Добро пожаловать\. Если есть вопросы или пожелания, обращайтесь к @unixowl\.

Список команд:

*/w \<тикер\> \<порог\>* \- Добавить инструмент в список отслеживания
	Примеры использования:
		*/w AAPL 1%*  _Будет присылать уведомление каждый раз, когда цена на акцию Apple изменится на 1%_
		*/w TWTR \=30* _Пришлет уведомление, когда цена на акцию Twitter достигнет или пересечет $30_

*/wl* \- Список отслеживаемых инструментов

*/wd \<тикер\>* \- Удалить инструмент из отслеживания
Примеры использованя:
*/wd AAPL* _Удалит все отслеживания за ценой на акции Apple_

*/watchglobal \<порог%\>* \- Отслеживать все акции, уведомлять о росте и падении любой акции в пределах торговой сессии\.

*/gainers \[число результатов\|порог%\]* \- Вывести список выросших акций за текущий день\. По умолчанию выводит топ 15\.
	Примеры использования:
	  */g 20* _Выведет топ 20 выросших акций_
	  */g 5%* _Выведет все акции, выросшие как минимум на 5%_

*/losers \[число результатов\|порог%\]* \- Вывести список упавших акций за текущий день\. По умолчанию выводит топ 15\.
	Примеры использования:
	  */l 20* _Выведет топ 20 выросших акций_
	  */l 5%* _Выведет все акции, упавшие как минимум на 5%_

*/info \<тикер\|figi\|название\> \[период\]* \- Вывести базовую информацию об инструменте и график изменения цены за указанный период
	Примеры использования:
		*/i AAPL* _Выведет базовую информацию об акциях Apple_
		*/i macy 90d* _Найдет инструмент M по подстроке из названия\(Macy\'s\) и выведет базовую информацию с графиком за 90 дней_
	Примеры задания периода:
		1h \- 1 час
		2d \- 1 дня
		3w \- 3 недели
		1mb, 2mb, 3mb, 5mb, 10mb, 15mb, 30mb \- размерность бара в 1, 2, 3, \.\.\. минут соответственно
		1hb \- размерность бара в 1 час
		1db \- размерность бара в 1 день
		1wb \- размерность бара в 1 неделю
		1mob \- размерность бара в 1 месяц

*Команды, требующие указание ключа API для Тинькофф Инвестиций:*
_На данный момент Тинькофф не предоставляет возможности разграничивать права доступа для ключей, а также поддерживается работа только с основным счетом, без ИИС\.
Используйте на свой страх и риск \(ключ позволяет создавать заявки на покупку/продажу, теоретически можно напакостить, если ключ попадет в плохие руки\)_

*/apikey* \- Задать API ключ \( см\. как получить на https://tinkoffcreditsystems\.github\.io/invest\-openapi/auth/\#\_2 \)

*/summary* \- Сводка по прибыли в портфеле

*/fullreport* \- Детальные данные прибыли по открытым и закрытым позициям
`,
		true,
	)
}

func (bot *Bot) HandleStop(chatID int64) {
	if err := bot.db.DeleteApiKey(chatID); err != nil {
		bot.sendError(chatID, fmt.Sprintf("Не удалось удалить API ключ: %v", err))
	}
	client := bot.StreamingWorker(chatID)
	if client != nil {
		pricewatchers, err := bot.db.PriceWatchList(chatID)
		if err != nil {
			bot.sendError(chatID, fmt.Sprintf("Не удалось удалить получить список отслеживания: %v", err))
		}
		for _, pw := range pricewatchers {
			client.UnsubscribeCandles(pw.FIGI, chatID)
		}
	}
	if err := bot.db.PriceWatchDeleteAll(chatID); err != nil {
		bot.sendError(chatID, fmt.Sprintf("Не удалось удалить списки отслеживания: %v", err))
	}
	if err := bot.db.UnSubscribePriceDaily(chatID); err != nil {
		bot.sendError(chatID, fmt.Sprintf("Не удалось удалить глобальное отслеживание: %v", err))
	}
	bot.sendText(chatID, "Данные удалены", false)
}

func (bot *Bot) sendText(chatID int64, msg string, isMarkdown bool) {
	_, err := bot.tg.Send(botMessage(tgbotapi.NewMessage(chatID, msg), isMarkdown))
	if err != nil {
		bot.log.Err(err).Int64("chatID", chatID).Msg("failed to send telegram message")
	}
}

func (bot *Bot) sendError(chatID int64, msg string) {
	_, err := bot.tg.Send(botMessage(tgbotapi.NewMessage(chatID, msg), false))
	if err != nil {
		bot.log.Err(err).Msg("failed to send telegram message")
	}
}

func (bot *Bot) fetchApiKey(chatID int64, reportError bool) string {
	var apiKey string
	var err error
	if bot.db.IsSet() {
		apiKey, err = bot.db.ApiKey(chatID)
	}
	if reportError && (err != nil || apiKey == "") {
		if err == pgx.ErrNoRows {
			bot.sendError(chatID, "API ключ не задан. Задайте с помощью команды /apikey")
		} else {
			bot.sendError(chatID, fmt.Sprintf("API ключ не задан(%v). Задайте с помощью команды /apikey", err))
		}
	}
	return apiKey
}

func botMessage(botMsg tgbotapi.MessageConfig, isMarkdown bool) tgbotapi.MessageConfig {
	if isMarkdown {
		botMsg.ParseMode = "MarkdownV2"
	}
	botMsg.DisableWebPagePreview = true
	botMsg.DisableNotification = true
	return botMsg
}
