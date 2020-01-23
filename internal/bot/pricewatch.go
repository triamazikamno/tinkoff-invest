package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	time "time"

	sdk "github.com/TinkoffCreditSystems/invest-openapi-go-sdk"
	"github.com/dustin/go-humanize"
	"github.com/triamazikamno/tinkoff-invest/pkg/pricewatch"
	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
)

func (bot *Bot) handleWatch(ctx context.Context, chatID int64, args []string) {
	if len(args) < 2 {
		bot.sendText(chatID, "Ошибка: не указан тикер или порог\\.\nПримеры:\n*/w AAPL 1%*\n*/w TWTR \\=30*", true)
		return
	}
	apiKey := bot.fetchApiKey(chatID, false)

	if apiKey == "" {
		apiKey = bot.defaultApiKey
	}

	if apiKey == "" {
		return
	}

	ti := tinkoffinvest.NewAPI(apiKey)
	instrument, err := ti.InstrumentByTicker(context.Background(), strings.ToUpper(args[0]))
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Тикер не найден(%v)", err))
		return
	}

	isPc := true
	var thresholdStr string
	if strings.HasPrefix(args[1], "=") {
		isPc = false
		thresholdStr = strings.TrimPrefix(args[1], "=")
	} else {
		thresholdStr = strings.TrimSuffix(args[1], "%")
	}
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		bot.sendError(chatID, "Не удалось интерпретировать порог. Пример: 1.25%")
		return
	}
	ob, err := ti.RestClient.Orderbook(ctx, 1, instrument.FIGI)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Не удалось получить стакан: %v", err))
		return
	}
	pw := pricewatch.PriceWatch{
		FIGI:         instrument.FIGI,
		Ticker:       instrument.Ticker,
		CurrentValue: ob.LastPrice,
		LastValue:    ob.LastPrice,
		IsPc:         isPc,
		IsPermanent:  true,
		Threshold:    threshold,
		Currency:     tinkoffinvest.Currency(instrument.Currency),
	}
	err = bot.db.PriceWatchAdd(chatID, pw)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Не удалось добавить отслеживание(%v)", err))
		return
	}
	if client := bot.StreamingWorker(chatID); client != nil {
		client.SubscribeCandles(pw.FIGI, chatID)
	}
	bot.sendText(chatID, "Принято", false)
}

func (bot *Bot) handleWatchGlobal(ctx context.Context, chatID int64, args []string) {
	if len(args) < 1 {
		bot.sendText(chatID, "Ошибка: не указан порог\\.\nПример:\n*/wg 10%*", true)
		return
	}
	threshold, err := strconv.ParseFloat(strings.TrimSuffix(args[0], "%"), 64)
	if err != nil {
		bot.sendError(chatID, "Не удалось интерпретировать порог. Пример: 10.25%")
		return
	}
	if threshold == 0 {
		err = bot.db.UnSubscribePriceDaily(chatID)
		if err != nil {
			bot.sendError(chatID, fmt.Sprintf("Не удалось удалить отслеживание(%v)", err))
			return
		}
	} else {
		err = bot.db.SubscribePriceDaily(chatID, threshold)
		if err != nil {
			bot.sendError(chatID, fmt.Sprintf("Не удалось добавить отслеживание(%v)", err))
			return
		}
	}
	bot.sendText(chatID, "Принято", false)
}

func (bot *Bot) handleWatchDelete(ctx context.Context, chatID int64, args []string) {
	if len(args) < 1 {
		bot.sendText(chatID, "Ошибка: не указан тикер\\.\nПример: */wd AAPL*", true)
		return
	}
	apiKey := bot.fetchApiKey(chatID, false)
	if apiKey == "" {
		apiKey = bot.defaultApiKey
	}
	if apiKey == "" {
		return
	}
	ti := tinkoffinvest.NewAPI(apiKey)
	instrument, err := ti.InstrumentByTicker(context.Background(), strings.ToUpper(args[0]))
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Тикер не найден(%v)", err))
		return
	}
	err = bot.db.PriceWatchDelete(chatID, instrument.FIGI)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Ошибка удаления отслеживания(%v)", err))
		return
	}
	priceWatchers, err := bot.db.PriceWatchListByFIGI(chatID, instrument.FIGI)
	if err == nil && len(priceWatchers) == 0 {
		client := bot.StreamingWorker(chatID)
		if client != nil {
			client.UnsubscribeCandles(instrument.FIGI, chatID)
		}
	}
	bot.sendText(chatID, "Удаление успешно", false)
}

func (bot *Bot) handleWatchList(ctx context.Context, chatID int64) {
	items, err := bot.db.PriceWatchList(chatID)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Ошибка получения списка отслеживания(%v)", err))
		return
	}
	apiKey := bot.fetchApiKey(chatID, false)
	var portfolio map[string]sdk.PositionBalance
	if apiKey != "" {
		ti := tinkoffinvest.NewAPI(apiKey)
		portfolio, err = ti.PortfolioPositions(context.Background())
		if err != nil {
			bot.log.Error().Err(err).Int64("chatID", chatID).Msg("failed to get portfolio")
		}
	}
	if portfolio != nil {
		for i, pw := range items {
			if position, ok := portfolio[pw.FIGI]; ok && position.AveragePositionPrice.Value > 0 {
				positionValue := position.AveragePositionPrice.Value * float64(position.Lots)
				currentValue := positionValue + position.ExpectedYield.Value

				pw.PortfolioGain = currentValue*100/positionValue - 100
				items[i] = pw
			}
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].PortfolioGain == 0 && items[j].PortfolioGain == 0 {
				return items[i].Pc() > items[j].Pc()
			}
			return items[i].PortfolioGain > items[j].PortfolioGain
		})
	}
	var msg string
	for _, pw := range items {
		if _, t, ok := bot.dataCache.get(pw.Ticker, true); ok {
			pw.TickerURL = fmt.Sprintf("[$%s](%s)", pw.Ticker, tickerURL(pw.Ticker, t))
		}
		msg += pw.String() + "\n"
	}
	bot.sendText(chatID, msg, true)
}

func tickerURL(ticker string, t instrumentType) string {
	return fmt.Sprintf("https://www.tinkoff.ru/invest/%s/%s/", t, ticker)
}

func (bot *Bot) StreamingWorker(chatID int64) *tinkoffinvest.StreamingClient {
	bot.streamingClientsMu.Lock()
	defer bot.streamingClientsMu.Unlock()
	if client, ok := bot.streamingClients[chatID]; ok {
		return client
	}
	isPrivateAccount := true
	apiKey := bot.fetchApiKey(chatID, false)
	if apiKey == "" {
		apiKey = bot.defaultApiKey
		isPrivateAccount = false
	}
	if apiKey == "" {
		bot.log.Error().Int64("chatID", chatID).Msg("failed to get api key")
		return nil
	}
	var client *tinkoffinvest.StreamingClient
	if !isPrivateAccount {
		client = bot.streamingClients[0]
	}
	if client == nil {
		client = tinkoffinvest.NewStreamingClient(
			apiKey, bot.log.With().Str("module", "streaming").Int64("chatID", chatID).Bool("private", isPrivateAccount).Logger(),
		)
		if !isPrivateAccount {
			bot.streamingClients[0] = client

		}
	}
	if isPrivateAccount {
		bot.streamingClients[chatID] = client
	}
	allPriceWatchers, err := bot.db.PriceWatchList(chatID)
	if err != nil {
		bot.log.Error().Int64("chatID", chatID).Msg("failed to get price watchers")
	} else {
		for _, pw := range allPriceWatchers {
			client.SubscribeCandles(pw.FIGI, chatID)
		}
	}
	go func() {
		ti := tinkoffinvest.NewAPI(apiKey)
		for event := range client.Events() {
			switch eventData := event.Data.(type) {
			case sdk.CandleEvent:
				items, err := bot.db.PriceWatchListByFIGI(chatID, eventData.Candle.FIGI)
				if err != nil {
					bot.log.Error().Int64("chatID", chatID).Interface("event", event).Msg("failed to get price watch list")
					continue
				}
				for _, pw := range items {
					if pw.CurrentValue != eventData.Candle.ClosePrice {
						err = bot.db.PriceWatchSetCurrentValue(pw.FIGI, eventData.Candle.ClosePrice)
						if err != nil {
							bot.log.Error().Interface("pw", pw).Interface("event", event).Msg("failed to set current value")
						}
						pw.CurrentValue = eventData.Candle.ClosePrice
					}
					if pw.IsPc {
						pc := pw.Pc()
						if math.Abs(pc) >= pw.Threshold {
							var portfolio map[string]sdk.PositionBalance
							if isPrivateAccount {
								portfolio, err = ti.PortfolioPositions(context.Background())
								if err != nil {
									bot.log.Error().Interface("pw", pw).Interface("event", event).Msg("failed to get portfolio")
								}
							}
							if portfolio != nil {
								if position, ok := portfolio[pw.FIGI]; ok && position.AveragePositionPrice.Value > 0 {
									pw.PortfolioGain = pw.CurrentValue*100/position.AveragePositionPrice.Value - 100
								}
							}
							err = bot.db.PriceWatchSetLastValue(pw.ID, pw.CurrentValue)
							if err != nil {
								bot.log.Error().Interface("pw", pw).Interface("event", event).Msg("failed to set last value")
							}
							if _, t, ok := bot.dataCache.get(pw.Ticker, true); ok {
								pw.TickerURL = fmt.Sprintf("[$%s](%s)", pw.Ticker, tickerURL(pw.Ticker, t))
							}
							bot.log.Info().
								Int64("chatID", pw.ChatID).Interface("event", event).Str("msg", pw.String()).Msg("sending price watch alarm")
							bot.sendText(pw.ChatID,
								pw.String(),
								true,
							)
						}
					} else if (pw.Threshold > pw.LastValue && pw.CurrentValue >= pw.Threshold) ||
						(pw.Threshold < pw.LastValue && pw.CurrentValue <= pw.Threshold) {
						err = bot.db.PriceWatchDeleteByID(pw.ID)
						if err != nil {
							bot.log.Error().Interface("pw", pw).Interface("event", event).Msg("failed to delete fixed price watcher")
						}
						bot.log.Info().
							Int64("chatID", pw.ChatID).Interface("event", event).Str("msg", pw.String()).Msg("sending price watch alarm")

						bot.sendText(pw.ChatID, pw.String(), true)
					}
				}
			default:
				bot.log.Error().Interface("event", event).Msg("unsupported event type")
				continue
			}
		}
	}()
	return client
}

func (bot *Bot) priceWatcherDailyWorker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for ; ; <-ticker.C {
		curDay := time.Now().Day()
		res, err := http.Get(allStocksURL)
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to get daily prices")
			continue
		}
		var result struct {
			Payload struct {
				Values []struct {
					Earnings struct {
						Relative float64
					}
					Symbol struct {
						Ticker   string
						ShowName string
					}
				}
			}
		}

		err = json.NewDecoder(res.Body).Decode(&result)
		res.Body.Close()
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to decode daily prices")
			continue
		}
		earners := make([]earner, 0, len(result.Payload.Values))
		seen := make(map[string]struct{})
		for _, item := range result.Payload.Values {
			earners = append(
				earners,
				earner{Ticker: item.Symbol.Ticker, Name: item.Symbol.ShowName, Earning: item.Earnings.Relative * 100},
			)
			seen[item.Symbol.Ticker] = struct{}{}
		}

		res, err = http.Post(
			"https://query2.finance.yahoo.com/v1/finance/screener?lang=en-US&formatted=false",
			"application/json",
			strings.NewReader(`{"offset":0,"size":100,"sortField":"percentchange","sortType":"DESC","quoteType":"EQUITY","query":{"operator":"AND","operands":[{"operator":"GT","operands":["percentchange",3]},{"operator":"or","operands":[{"operator":"EQ","operands":["region","us"]},{"operator":"EQ","operands":["region","ru"]}]},{"operator":"or","operands":[{"operator":"BTWN","operands":["intradaymarketcap",2000000000,10000000000]},{"operator":"BTWN","operands":["intradaymarketcap",10000000000,100000000000]},{"operator":"GT","operands":["intradaymarketcap",100000000000]}]},{"operator":"gt","operands":["dayvolume",15000]}]},"userId":"","userIdType":"guid"}`),
		)
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to get daily prices from yahoo")
		} else {
			var result struct {
				Finance struct {
					Result []struct {
						Quotes []struct {
							RegularMarketChangePercent float64
							RegularMarketTime          int64
							Symbol                     string
							Name                       string `json:"shortName"`
						}
					}
				}
			}
			err = json.NewDecoder(res.Body).Decode(&result)
			res.Body.Close()
			if err != nil || len(result.Finance.Result) == 0 {
				bot.log.Error().Err(err).Msg("failed to decode daily yahoo prices")
			} else {
				for _, item := range result.Finance.Result[0].Quotes {
					if curDay != time.Unix(item.RegularMarketTime, 0).Day() {
						continue
					}
					ticker := strings.TrimSuffix(item.Symbol, ".ME")
					if _, ok := seen[ticker]; ok {
						continue
					}
					earners = append(
						earners,
						earner{Ticker: ticker, Earning: item.RegularMarketChangePercent, IsExternal: true, Name: item.Name},
					)
				}
			}
		}

		res, err = http.Post(
			"https://query2.finance.yahoo.com/v1/finance/screener?lang=en-US&formatted=false",
			"application/json",
			strings.NewReader(`{"offset":0,"size":100,"sortField":"percentchange","sortType":"DESC","quoteType":"EQUITY","query":{"operator":"AND","operands":[{"operator":"LT","operands":["percentchange",-2]},{"operator":"or","operands":[{"operator":"EQ","operands":["region","us"]},{"operator":"EQ","operands":["region","ru"]}]},{"operator":"or","operands":[{"operator":"BTWN","operands":["intradaymarketcap",2000000000,10000000000]},{"operator":"BTWN","operands":["intradaymarketcap",10000000000,100000000000]},{"operator":"GT","operands":["intradaymarketcap",100000000000]}]},{"operator":"gt","operands":["dayvolume",15000]}]},"userId":"","userIdType":"guid"}`),
		)
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to get daily prices from yahoo")
		} else {
			var result struct {
				Finance struct {
					Result []struct {
						Quotes []struct {
							RegularMarketChangePercent float64
							RegularMarketTime          int64
							Symbol                     string
							Name                       string `json:"shortName"`
						}
					}
				}
			}
			err = json.NewDecoder(res.Body).Decode(&result)
			res.Body.Close()
			if err != nil || len(result.Finance.Result) == 0 {
				bot.log.Error().Err(err).Msg("failed to decode daily yahoo prices")
			} else {
				for _, item := range result.Finance.Result[0].Quotes {
					if curDay != time.Unix(item.RegularMarketTime, 0).Day() {
						continue
					}
					ticker := strings.TrimSuffix(item.Symbol, ".ME")
					if _, ok := seen[ticker]; ok {
						continue
					}
					earners = append(
						earners,
						earner{Ticker: ticker, Earning: item.RegularMarketChangePercent, IsExternal: true, Name: item.Name},
					)
				}
			}
		}
		sort.Slice(earners, func(i, j int) bool {
			return earners[i].Earning > earners[j].Earning
		})

		subs, err := bot.db.SubscriptionsPriceDaily()
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to get daily price subscriptions")
		}
		var minThreshold float64
		for _, v := range subs {
			if v > minThreshold {
				minThreshold = v
			}
		}
		if minThreshold != 0 {
			for _, earner := range earners {
				if earning := math.Abs(earner.Earning); earning >= minThreshold {
					for chatID, threshold := range subs {
						if earning >= threshold {
							bot.notifyPriceDaily(chatID, earner)
						}
					}
				}
			}
		}
		bot.earners = earners
	}
}

func (bot *Bot) notifyPriceDaily(chatID int64, item earner) {
	alreadyNotified, err := bot.db.PriceDailyMarkNotified(chatID, item.Ticker)
	if err != nil {
		bot.log.Error().Err(err).Int64("chatID", chatID).Interface("item", item).Msg("failed to notify daily price")
	}
	if alreadyNotified {
		return
	}
	ticker := item.Ticker
	if _, t, ok := bot.dataCache.get(ticker, true); ok {
		ticker = fmt.Sprintf("[$%s %s](%s)", item.Ticker, markDownEscape.Replace(item.Name), tickerURL(item.Ticker, t))
	} else {
		ticker = fmt.Sprintf(
			"[\\*%s %s](https://finance.yahoo.com/quote/%s)",
			item.Ticker, markDownEscape.Replace(item.Name), item.Ticker,
		)
	}
	bot.log.Info().Int64("chatID", chatID).Interface("item", item).Msg("Sending global watch alarm")
	bot.sendText(
		chatID,
		fmt.Sprintf("`%-8s `%s\n", numSign(item.Earning)+humanize.FormatFloat("", item.Earning)+"%", ticker),
		true,
	)
}

func (bot *Bot) handleGainers(ctx context.Context, chatID int64, args []string) {
	var threshold float64
	var n int
	if len(args) > 0 {
		arg := args[0]
		var err error
		if strings.HasSuffix(arg, "%") {
			threshold, err = strconv.ParseFloat(strings.TrimSuffix(arg, "%"), 64)
			if err != nil {
				bot.sendError(chatID, fmt.Sprintf("Неправильно задан порог: %v", err))
				return
			}
		} else {
			n, err = strconv.Atoi(arg)
			if err != nil {
				bot.sendError(chatID, fmt.Sprintf("Неправильно задан порог: %v", err))
				return
			}
		}
	}
	if threshold == 0 && n == 0 {
		n = 15
	}
	items := bot.earners.Gainers(n, threshold)
	var msg string
	for _, item := range items {
		ticker := item.Ticker
		if _, t, ok := bot.dataCache.get(ticker, true); ok {
			ticker = fmt.Sprintf("[$%s %s](%s)", item.Ticker, markDownEscape.Replace(item.Name), tickerURL(item.Ticker, t))
		} else {
			ticker = fmt.Sprintf(
				"[\\*%s %s](https://finance.yahoo.com/quote/%s)",
				item.Ticker, markDownEscape.Replace(item.Name), item.Ticker,
			)
		}
		entry := fmt.Sprintf("`%-8s `%s\n", numSign(item.Earning)+humanize.FormatFloat("", item.Earning)+"%", ticker)
		if len(msg)+len(entry) >= 3000 {
			bot.sendText(chatID, msg, true)
			msg = ""
		}
		msg += entry
	}
	if msg != "" {
		bot.sendText(chatID, msg, true)
	}
}

func (bot *Bot) handleLosers(ctx context.Context, chatID int64, args []string) {
	var threshold float64
	var n int
	if len(args) > 0 {
		arg := args[0]
		var err error
		if strings.HasSuffix(arg, "%") {
			threshold, err = strconv.ParseFloat(strings.TrimSuffix(arg, "%"), 64)
			if err != nil {
				bot.sendError(chatID, fmt.Sprintf("Неправильно задан порог: %v", err))
				return
			}
		} else {
			n, err = strconv.Atoi(arg)
			if err != nil {
				bot.sendError(chatID, fmt.Sprintf("Неправильно задан порог: %v", err))
				return
			}
		}
	}
	if threshold == 0 && n == 0 {
		n = 15
	}

	items := bot.earners.Losers(n, threshold)
	var msg string
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		ticker := item.Ticker
		if _, t, ok := bot.dataCache.get(ticker, true); ok {
			ticker = fmt.Sprintf("[$%s %s](%s)", item.Ticker, markDownEscape.Replace(item.Name), tickerURL(item.Ticker, t))
		} else {
			ticker = fmt.Sprintf(
				"[\\*%s %s](https://finance.yahoo.com/quote/%s)",
				item.Ticker, markDownEscape.Replace(item.Name), item.Ticker,
			)
		}
		entry := fmt.Sprintf("`%-8s `%s\n", numSign(item.Earning)+humanize.FormatFloat("", item.Earning)+"%", ticker)
		if len(msg)+len(entry) >= 3000 {
			bot.sendText(chatID, msg, true)
			msg = ""
		}
		msg += entry
	}
	if msg != "" {
		bot.sendText(chatID, msg, true)
	}
}

type earners []earner

type earner struct {
	Ticker     string
	Earning    float64
	Name       string
	IsExternal bool
}

func (e earners) Gainers(n int, threshold float64) []earner {
	if n == 0 && threshold == 0 {
		return nil
	}
	if len(e) == 0 {
		return nil
	}
	for i, earner := range e {
		if n > 0 && i >= n {
			return e[:i]
		}
		if threshold > 0 && earner.Earning < threshold {
			if i == 0 {
				return nil
			}
			return e[:i-1]
		}
	}
	return nil
}

func (e earners) Losers(n int, threshold float64) []earner {
	if n == 0 && threshold == 0 {
		return nil
	}
	if len(e) == 0 {
		return nil
	}
	for i := len(e) - 1; i >= 0; i-- {
		if n > 0 && len(e)-i >= n {
			return e[i:]
		}
		if threshold > 0 && -1*e[i].Earning < threshold {
			return e[i:]
		}
	}
	return nil
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
