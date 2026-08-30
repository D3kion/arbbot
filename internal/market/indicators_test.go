package market

import (
	"math"
	"testing"
)

const eps = 1e-9

func almost(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Fatalf("%s: got %v want %v", name, got, want)
	}
}

func TestEMAConstantSeries(t *testing.T) {
	prices := make([]float64, 50)
	for i := range prices {
		prices[i] = 100
	}
	almost(t, "ema9", EMA(prices, 9), 100)
	almost(t, "ema21", EMA(prices, 21), 100)
}

func TestRSITrends(t *testing.T) {
	up := make([]float64, 30)
	for i := range up {
		up[i] = float64(i + 1)
	}
	if r := RSI(up, 14); r != 100 {
		t.Fatalf("all-up rsi: got %v want 100", r)
	}

	down := make([]float64, 30)
	for i := range down {
		down[i] = float64(30 - i)
	}
	if r := RSI(down, 14); r != 0 {
		t.Fatalf("all-down rsi: got %v want 0", r)
	}

	flat := make([]float64, 30)
	for i := range flat {
		flat[i] = 42
	}
	if r := RSI(flat, 14); r != 50 {
		t.Fatalf("flat rsi: got %v want 50", r)
	}
}

func TestMACDUptrendPositiveAndAboveSignal(t *testing.T) {
	ramp := make([]float64, 60)
	for i := range ramp {
		ramp[i] = 100 + float64(i)*float64(i)
	}
	macd, signal, hist := MACD(ramp)
	if macd <= 0 || hist <= 0 || macd < signal {
		t.Fatalf("uptrend macd=%v signal=%v hist=%v", macd, signal, hist)
	}
}

func TestMACDShortSeriesZero(t *testing.T) {
	m, s, h := MACD([]float64{1, 2, 3})
	almost(t, "macd", m, 0)
	almost(t, "signal", s, 0)
	almost(t, "hist", h, 0)
}

func TestNormalizeSymbol(t *testing.T) {
	cases := map[string]string{
		"btc":      "BTC/USDT",
		" BTC ":    "BTC/USDT",
		"eth/usdt": "ETH/USDT",
		"BTC:USDT": "BTC:USDT",
	}
	for in, want := range cases {
		if got := NormalizeSymbol(in); got != want {
			t.Fatalf("NormalizeSymbol(%q)=%q want %q", in, got, want)
		}
	}
}
