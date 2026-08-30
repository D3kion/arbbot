package market

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	ccxt "github.com/ccxt/ccxt/go/v4"
)

const cacheTTL = 10 * time.Second

type Candle struct {
	Time  time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
	Vol   float64
}

type Exchange struct {
	b        *ccxt.Binance
	mu       sync.Mutex
	prices   map[string]cachedPrice
	priceTTL time.Duration
}

type cachedPrice struct {
	value float64
	at    time.Time
}

func NewExchange() (*Exchange, error) {
	b := ccxt.NewBinance(nil)
	if _, err := b.LoadMarkets(); err != nil {
		return nil, fmt.Errorf("load markets: %w", err)
	}
	return &Exchange{b: b, prices: make(map[string]cachedPrice), priceTTL: cacheTTL}, nil
}

func (e *Exchange) GetPrice(symbol string) (float64, error) {
	symbol = NormalizeSymbol(symbol)
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.prices[symbol]; ok && time.Since(c.at) < e.priceTTL {
		return c.value, nil
	}

	t, err := e.b.FetchTicker(symbol)
	if err != nil {
		return 0, fmt.Errorf("fetch ticker %s: %w", symbol, err)
	}
	var p float64
	switch {
	case t.Last != nil:
		p = *t.Last
	case t.Close != nil:
		p = *t.Close
	default:
		return 0, fmt.Errorf("ticker %s: no price", symbol)
	}
	e.prices[symbol] = cachedPrice{value: p, at: time.Now()}
	return p, nil
}

func (e *Exchange) GetOHLCV(symbol, timeframe string, limit int) ([]Candle, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	raw, err := e.b.FetchOHLCV(NormalizeSymbol(symbol),
		ccxt.WithFetchOHLCVTimeframe(timeframe),
		ccxt.WithFetchOHLCVLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("fetch ohlcv %s %s: %w", symbol, timeframe, err)
	}
	out := make([]Candle, 0, len(raw))
	for _, r := range raw {
		out = append(out, Candle{
			Time:  time.UnixMilli(r.Timestamp),
			Open:  r.Open,
			High:  r.High,
			Low:   r.Low,
			Close: r.Close,
			Vol:   r.Volume,
		})
	}
	return out, nil
}

func NormalizeSymbol(input string) string {
	s := strings.ToUpper(strings.TrimSpace(input))
	if !strings.Contains(s, "/") && !strings.Contains(s, ":") {
		s += "/USDT"
	}
	return s
}

func FormatPrice(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func IsRu(lang string) bool { return strings.HasPrefix(strings.ToLower(lang), "ru") }

func IsValidTimeframe(tf string) bool { return tf == "1h" || tf == "4h" || tf == "1d" }
