package bot

import (
	"context"
	"fmt"
	"image/color"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	sdk "github.com/TinkoffCreditSystems/invest-openapi-go-sdk"
	"github.com/bseolized/shadowmaker/util"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
	"github.com/pplcc/plotext"
	"github.com/pplcc/plotext/custplotter"
	"github.com/triamazikamno/tinkoff-invest/pkg/tinkoffinvest"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/vg"
	"gonum.org/v1/plot/vg/draw"
	"gonum.org/v1/plot/vg/vgimg"
)

var periodRe = regexp.MustCompile("^[0-9]")

const (
	allStocksURL = "https://api.tinkoff.ru/trading/stocks/list?sortType=ByName&orderType=Asc&country=All"
)

type dataCache struct {
	stocks sync.Map
	bonds  sync.Map
	etfs   sync.Map
}

type instrumentType string

const (
	typeStocks instrumentType = "stocks"
	typeBonds  instrumentType = "bonds"
	typeEtfs   instrumentType = "etfs"
)

//nolint: nakedret
func (d *dataCache) get(query string, exact bool) (instrument sdk.Instrument, instrumentType instrumentType, found bool) {
	if item, ok := d.stocks.Load(query); ok {
		return item.(sdk.Instrument), typeStocks, true
	}
	if item, ok := d.bonds.Load(query); ok {
		return item.(sdk.Instrument), typeBonds, true
	}
	if item, ok := d.etfs.Load(query); ok {
		return item.(sdk.Instrument), typeEtfs, true
	}
	if exact {
		return
	}
	d.stocks.Range(func(key, val interface{}) bool {
		if item, ok := val.(sdk.Instrument); ok {
			if item.FIGI == query || strings.Contains(strings.ToUpper(item.Name), query) {
				found = true
				instrument = item
				instrumentType = typeStocks
				return false
			}
		}
		return true
	})
	if found {
		return
	}
	d.bonds.Range(func(key, val interface{}) bool {
		if item, ok := val.(sdk.Instrument); ok {
			if item.FIGI == query || strings.Contains(strings.ToUpper(item.Name), query) {
				found = true
				instrument = item
				instrumentType = typeBonds
				return false
			}
		}
		return true
	})
	if found {
		return
	}
	d.etfs.Range(func(key, val interface{}) bool {
		if item, ok := val.(sdk.Instrument); ok {
			if item.FIGI == query || strings.Contains(strings.ToUpper(item.Name), query) {
				found = true
				instrument = item
				instrumentType = typeEtfs
				return false
			}
		}
		return true
	})
	return
}

func (bot *Bot) Start() {
	go bot.dataCacheWorker()
	go bot.priceWatcherDailyWorker()
}

func (bot *Bot) dataCacheWorker() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	ti := tinkoffinvest.NewAPI(bot.defaultApiKey)
	for ; ; <-ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		stocks, err := ti.RestClient.Stocks(ctx)
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to get stocks while refreshing dataCache")
		}
		for _, item := range stocks {
			bot.dataCache.stocks.Store(item.Ticker, item)
		}
		bonds, err := ti.RestClient.Bonds(ctx)
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to get bonds while refreshing dataCache")
		}
		for _, item := range bonds {
			bot.dataCache.bonds.Store(item.Ticker, item)
		}
		etfs, err := ti.RestClient.ETFs(ctx)
		if err != nil {
			bot.log.Error().Err(err).Msg("failed to get etfs while refreshing dataCache")
		}
		for _, item := range etfs {
			bot.dataCache.etfs.Store(item.Ticker, item)
		}
		cancel()
	}
}

func (bot *Bot) HandleInfo(ctx context.Context, chatID int64, args []string) {
	if len(args) < 1 {
		bot.sendText(chatID, "Ошибка: не указан тикер\\.\nПример: */info AAPL*", true)
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
	var err error
	query := strings.ToUpper(args[0])
	var item sdk.Instrument
	var instrumentType instrumentType
	var found bool
	if strings.HasPrefix(query, "$") {
		item, instrumentType, _ = bot.dataCache.get(strings.TrimPrefix(query, "$"), true)
	} else {
		item, instrumentType, found = bot.dataCache.get(query, false)
		if !found {
			if len(query) > 8 {
				item, _ = ti.RestClient.SearchInstrumentByFIGI(ctx, query)
			}
			if item.FIGI == "" {
				instruments, _ := ti.RestClient.SearchInstrumentByTicker(ctx, query)
				if len(instruments) > 0 {
					item = instruments[0]
				}
			}
			_, instrumentType, _ = bot.dataCache.get(item.Ticker, false)
		}
	}
	if item.Ticker == "" {
		bot.sendError(chatID, "Инструмент не найден")
		return
	}
	ob, err := ti.RestClient.Orderbook(ctx, 1, item.FIGI)
	if err != nil {
		bot.sendError(chatID, fmt.Sprintf("Ошибка получения стакана (%v)", err))
		return
	}
	msg := fmt.Sprintf(
		"Тикер: [%s](https://www.tinkoff.ru/invest/%s/%s/)\n",
		item.Ticker,
		instrumentType,
		item.Ticker,
	)
	msg += markDownEscape.Replace(fmt.Sprintf(
		`Название: %s
FIGI: %s
ISIN: %s
Валюта: %s
Последняя цена: %.2f
`,
		item.Name,
		item.FIGI,
		item.ISIN,
		item.Currency,
		ob.LastPrice,
	))
	bot.sendText(chatID, msg, true)

	var label string
	periodArg := "60d"
	if len(args) > 1 {
		periodArg = args[1]
	}
	interval := sdk.CandleInterval1Day
	periods := make([]Period, 0)
	if periodRe.MatchString(periodArg) {
		label = periodArg
		switch periodArg {
		case "1mb":
			interval = sdk.CandleInterval1Min
			periods = []Period{newPeriod(40, time.Now())}
		case "2mb":
			interval = sdk.CandleInterval2Min
			periods = []Period{newPeriod(hour, time.Now())}
		case "3mb":
			interval = sdk.CandleInterval3Min
			periods = []Period{newPeriod(2*hour, time.Now())}
		case "5mb":
			interval = sdk.CandleInterval5Min
			periods = []Period{newPeriod(4*hour, time.Now())}
		case "10mb":
			interval = sdk.CandleInterval10Min
			periods = []Period{newPeriod(8*hour, time.Now())}
		case "15mb":
			interval = sdk.CandleInterval15Min
			periods = []Period{newPeriod(15*hour, time.Now())}
		case "30mb":
			interval = sdk.CandleInterval30Min
			periods = []Period{newPeriod(23*hour, time.Now())}
		case "1hb":
			interval = sdk.CandleInterval1Hour
			periods = []Period{newPeriod(7*day-hour, time.Now())}
		case "1db":
			interval = sdk.CandleInterval1Day
			periods = []Period{newPeriod(60*day, time.Now())}
		case "1wb":
			interval = sdk.CandleInterval1Week
			periods = []Period{newPeriod(50*week, time.Now())}
		case "1mob":
			interval = sdk.CandleInterval1Month
			periods = []Period{newPeriod(50*month, time.Now())}
		default:
			rawPeriod, err := util.ParseDuration(periodArg, "h")
			period := int64(rawPeriod / 60000)
			if period < 5 {
				bot.sendError(chatID, fmt.Sprintf("Неверно задан период(%v)", err))
				return
			}
			switch {
			case period >= 10*year:
				interval = sdk.CandleInterval1Month
				periods = splitPeriod(period, 10*year)
			case period >= 2*year:
				interval = sdk.CandleInterval1Month
			case period >= 140*day:
				interval = sdk.CandleInterval1Week
			case period > 40*day:
				interval = sdk.CandleInterval1Day
			case period > 7*day:
				interval = sdk.CandleInterval1Hour
				periods = splitPeriod(period, 7*day)
			case period >= 30*hour:
				interval = sdk.CandleInterval1Hour
			case period >= 15*hour:
				interval = sdk.CandleInterval30Min
			case period >= 8*hour:
				interval = sdk.CandleInterval15Min
			case period >= 4*hour:
				interval = sdk.CandleInterval10Min
			case period >= 2*hour:
				interval = sdk.CandleInterval5Min
			case period >= hour:
				interval = sdk.CandleInterval3Min
			case period >= 40:
				interval = sdk.CandleInterval2Min
			default:
				interval = sdk.CandleInterval1Min
			}
			if len(periods) == 0 {
				periods = []Period{newPeriod(period, time.Now())}
			}
		}
		allCandles := make([]sdk.Candle, 0)
		for _, period := range periods {
			rawCandles, err := ti.RestClient.Candles(ctx, period.from, period.to, interval, item.FIGI)
			if err != nil {
				bot.sendError(chatID, fmt.Sprintf("Ошибка получения свечей (%v)", err))
				return
			}
			allCandles = append(allCandles, rawCandles...)
		}
		if len(allCandles) == 0 {
			return
		}
		cd := NewCandles(item.Ticker, allCandles)
		fi, err := cd.Chart(label)
		if err != nil {
			bot.sendError(chatID, fmt.Sprintf("Ошибка генерации графика (%v)", err))
			return
		}

		_, _ = bot.tg.Send(
			tgbotapi.NewPhotoUpload(
				chatID, tgbotapi.FileReader{Name: item.Name, Reader: fi, Size: -1}),
		)
	}
}

type Period struct {
	from time.Time
	to   time.Time
}

func newPeriod(p int64, to time.Time) Period {
	return Period{
		from: to.Add(-1 * time.Duration(p) * time.Minute),
		to:   to,
	}
}

func splitPeriod(period, partSize int64) []Period {
	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Duration(period) * time.Minute)
	periods := make([]Period, 0)
	for i := int64(0); i < (period/partSize)+1; i++ {
		to := startTime.Add(time.Duration(partSize) * time.Minute)
		if to.After(endTime) {
			to = endTime
		}
		if startTime.Equal(to) {
			break
		}
		periods = append(periods, Period{
			from: startTime,
			to:   to,
		})
		startTime = to
	}
	return periods
}

type candles struct {
	data   []sdk.Candle
	symbol string
}

func NewCandles(symbol string, data []sdk.Candle) candles {
	cd := candles{
		symbol: symbol,
		data:   data,
	}
	return cd
}

// Len returns the size of the data
func (cd candles) Len() int {
	return len(cd.data)
}

// TOHLCV returns TOHLCV tuple for specified index
func (cd candles) TOHLCV(i int) (float64, float64, float64, float64, float64, float64) {
	// time, open, high, low, close, volume
	return float64(i), cd.data[i].OpenPrice, cd.data[i].HighPrice, cd.data[i].LowPrice, cd.data[i].ClosePrice, cd.data[i].Volume
}

func (cd candles) Chart(label string) (io.Reader, error) {
	var maxVolume float64
	for _, d := range cd.data {
		if d.Volume > maxVolume {
			maxVolume = d.Volume
		}
	}
	plot.DefaultFont = "Helvetica"
	p1, err := plot.New()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new plot")
	}
	p1.Title.Font.Size = 16

	p1.Title.Text = fmt.Sprintf("%s (%s)", cd.symbol, label)
	p1.X.Tick.Marker = plot.TimeTicks{Format: " "}
	candlesticks, err := custplotter.NewCandlesticks(cd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create candlesticks")
	}

	p2, err := plot.New()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create new plot")
	}
	p2.X.Tick.Marker = plot.TimeTicks{
		Format: "2006-01-02\n15:04",
		Time: func(t float64) time.Time {
			if t < 0 || int(t) >= len(cd.data) {
				return time.Unix(0, 0)
			}
			return cd.data[int(t)].TS.In(loc)
		},
	}

	vBars, err := custplotter.NewVBars(cd)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create vbars")
	}
	vBars.Width = vg.Points(4)
	switch {
	case len(cd.data) >= 50:
		candlesticks.CandleWidth = vg.Length(3)
	default:
		candlesticks.CandleWidth = vg.Length(4)
	}

	candlesticks.LineStyle.Width = vg.Length(0.3)
	candlesticks.ColorDown = color.RGBA{R: 255, G: 128, B: 128, A: 255}
	vBars.ColorUp = color.RGBA{R: 128, G: 192, B: 128, A: 255}
	vBars.ColorDown = color.RGBA{R: 255, G: 128, B: 128, A: 255}

	p1.Add(candlesticks)
	p2.Add(vBars)

	plotext.UniteAxisRanges([]*plot.Axis{&p1.X, &p2.X})
	var table plotext.Table
	var plots [][]*plot.Plot
	if maxVolume > 0 {
		table = plotext.Table{
			RowHeights: []float64{4, 1},
			ColWidths:  []float64{1},
			PadBottom:  5,
			PadRight:   5,
			PadLeft:    5,
			PadTop:     5,
		}
		plots = [][]*plot.Plot{[]*plot.Plot{p1}, []*plot.Plot{p2}}
	} else {
		table = plotext.Table{
			RowHeights: []float64{1},
			ColWidths:  []float64{1},
		}

		plots = [][]*plot.Plot{[]*plot.Plot{p1}}
	}

	img := vgimg.New(387, 258)
	dc := draw.New(img)
	canvases := table.Align(plots, dc)
	plots[0][0].Draw(canvases[0][0])
	if maxVolume > 0 {
		plots[1][0].Draw(canvases[1][0])
	}
	// p1.Draw(dc)
	png := vgimg.PngCanvas{Canvas: img}
	r, w := io.Pipe()
	go func(w *io.PipeWriter) {
		_, _ = png.WriteTo(w)
		w.Close()
	}(w)

	return r, nil
}

const (
	year  = 365 * day
	month = 31 * day
	week  = 7 * day
	day   = 24 * hour
	hour  = 60
)
