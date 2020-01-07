package tinkoffinvest

import (
	context "context"
	"sync"
	"time"

	sdk "github.com/TinkoffCreditSystems/invest-openapi-go-sdk"
	"github.com/rs/zerolog"
)

type StreamingClient struct {
	sync.Mutex
	ctx           context.Context
	ctxCancel     context.CancelFunc
	client        *sdk.StreamingClient
	events        chan interface{}
	commands      chan interface{}
	subscriptions sync.Map
	apiKey        string
	log           *zerolog.Logger
}

type CommandSubscribeCandle struct {
	FIGI     string
	Interval sdk.CandleInterval
}

type CommandUnsubscribeCandle struct {
	FIGI     string
	Interval sdk.CandleInterval
}

func NewStreamingClient(apiKey string, log zerolog.Logger) *StreamingClient {
	c := &StreamingClient{
		apiKey:   apiKey,
		events:   make(chan interface{}, 1000),
		commands: make(chan interface{}, 100),
		log:      &log,
	}

	c.ctx, c.ctxCancel = context.WithCancel(context.Background())
	go c.commandPipe()

	return c
}

func (c *StreamingClient) SetApiKey(apiKey string) {
	c.apiKey = apiKey
}

func (c *StreamingClient) Events() <-chan interface{} {
	return c.events
}

func (c *StreamingClient) StreamingClient() *sdk.StreamingClient {
	c.Lock()
	defer c.Unlock()
	if c.client != nil {
		return c.client
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ; ; <-ticker.C {
		var err error
		c.client, err = sdk.NewStreamingClient(c.log, c.apiKey)
		if err != nil {
			c.log.Printf("failed to connect to ws: %v\n", err)
			continue
		}
		go func() {
			err := c.client.RunReadLoop(func(event interface{}) error {
				select {
				case <-c.ctx.Done():
					return nil
				case c.events <- event:
				}
				return nil
			})
			c.log.Printf("readloop exited with err=%v\n", err)
			c.Lock()
			c.client.Close()
			c.client = nil
			c.Unlock()
			c.subscriptions.Range(func(key, value interface{}) bool {
				sub, ok := value.(*subscription)
				if !ok || sub == nil {
					return false
				}
				c.commands <- sub.cmd
				return true
			})
		}()
		return c.client
	}
}

func (c *StreamingClient) commandPipe() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case cmd := <-c.commands:
			// c.log.Printf("processing command: %+v\n", cmd)
			switch command := cmd.(type) {
			case CommandSubscribeCandle:
				if err := c.StreamingClient().SubscribeCandle(command.FIGI, command.Interval, requestID()); err != nil {
					c.log.Printf("subscribe candle command failed: %v\n", err)
				}
			case CommandUnsubscribeCandle:
				if err := c.StreamingClient().UnsubscribeCandle(command.FIGI, command.Interval, requestID()); err != nil {
					c.log.Printf("unsubscribe candle command failed: %v\n", err)
				}
			default:
				c.log.Printf("unsupported command type: %+v\n", cmd)
			}
		}
	}
}

type subscription struct {
	subscribers sync.Map
	cmd         interface{}
}

func (c *StreamingClient) SubscribeCandles(figi string, chatID int64) {
	s, _ := c.subscriptions.LoadOrStore(
		"candles-"+figi,
		&subscription{cmd: CommandSubscribeCandle{FIGI: figi, Interval: sdk.CandleInterval1Min}},
	)
	sub, ok := s.(*subscription)
	if !ok || sub == nil {
		return
	}
	if _, ok := sub.subscribers.LoadOrStore(chatID, struct{}{}); ok {
		return
	}
	select {
	case <-c.ctx.Done():
		return
	case c.commands <- sub.cmd:
	}
}

func (c *StreamingClient) UnsubscribeCandles(figi string, chatID int64) {
	s, ok := c.subscriptions.Load("candles-" + figi)
	if !ok {
		return
	}
	sub, ok := s.(*subscription)
	if !ok || sub == nil {
		return
	}
	_, ok = sub.subscribers.Load(chatID)
	if !ok {
		return
	}
	sub.subscribers.Delete(chatID)

	select {
	case <-c.ctx.Done():
		return
	case c.commands <- CommandUnsubscribeCandle{FIGI: figi, Interval: sdk.CandleInterval1Min}:
	}
}

func (c *StreamingClient) StreamingClientClose() {
	c.ctxCancel()
	c.Lock()
	defer c.Unlock()
	if c.client != nil {
		c.client.Close()
	}
	close(c.commands)
	close(c.events)
}
