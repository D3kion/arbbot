package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"arbbot/internal/ai"
	"arbbot/internal/config"
	"arbbot/internal/market"
	"arbbot/internal/storage"
)

const priceTimeout = 15 * time.Second
const ohlcvTimeout = 20 * time.Second

type Outcome struct {
	SignalID int64
	Symbol   string
	Req      *ai.SignalRequest
	Resp     *ai.SignalResponse
	Source   string
}

type Service struct {
	exch  *market.Exchange
	ai    *ai.Client
	logs  *storage.SignalRepo
	users *storage.UserRepo
	subs  *storage.SubRepo
	cfg   *config.Config
}

func New(cfg *config.Config, exch *market.Exchange, aiClient *ai.Client,
	logs *storage.SignalRepo, users *storage.UserRepo, subs *storage.SubRepo) *Service {
	return &Service{exch: exch, ai: aiClient, logs: logs, users: users, subs: subs, cfg: cfg}
}

func (s *Service) CheckQuota(userID int64) (premium bool, remaining int, err error) {
	u, err := s.users.GetFresh(userID)
	if err != nil {
		return false, 0, err
	}
	premium, err = s.subs.HasActive(userID)
	if err != nil {
		return false, 0, err
	}
	if premium {
		return true, 0, nil
	}
	return false, s.cfg.FreeSignalsLimit - u.FreeSignalsUsed, nil
}

func (s *Service) TryConsumeFreeSignal(userID int64) (bool, error) {
	premium, err := s.subs.HasActive(userID)
	if err != nil {
		return false, err
	}
	if premium {
		return true, nil
	}
	return s.users.TryConsumeFreeSignal(userID, s.cfg.FreeSignalsLimit)
}

func (s *Service) GrantPremium(userID int64, days int) (time.Time, error) {
	if _, err := s.users.GetByID(userID); err == storage.ErrNotFound {
		if err := s.users.CreateOrUpdate(&storage.User{ID: userID}); err != nil {
			return time.Time{}, err
		}
		slog.Info("stub user created for grant", "user", userID)
	} else if err != nil {
		return time.Time{}, err
	}
	return s.subs.Activate(userID, "trader", days)
}

func (s *Service) tfFor(userID int64) string {
	u, err := s.users.GetByID(userID)
	if err != nil || !market.IsValidTimeframe(u.Timeframe) {
		return "4h"
	}
	return u.Timeframe
}

func (s *Service) MakeSignal(ctx context.Context, userID int64, input, languageCode string) (*Outcome, error) {
	symbol := market.NormalizeSymbol(input)
	tf := s.tfFor(userID)
	out, err := s.coreSignal(ctx, symbol, tf, promptLang(languageCode))
	if err != nil {
		return nil, err
	}
	// per-user log
	entry := &storage.Signal{
		UserID:      userID,
		Symbol:      symbol,
		Signal:      out.Resp.Signal,
		Confidence:  out.Resp.Confidence,
		PriceAtTime: out.Req.Price,
		TargetPrice: out.Resp.TakeProfit,
		StopLoss:    out.Resp.StopLoss,
	}
	if err := s.logs.Create(entry); err != nil {
		slog.Error("save signal log", "err", err)
	} else {
		out.SignalID = entry.ID
		slog.Info("signal logged", "id", entry.ID, "user", userID, "symbol", symbol,
			"signal", out.Resp.Signal, "source", out.Source)
	}
	return out, nil
}

// fetchPrice with timeout respecting ctx
func (s *Service) fetchPrice(ctx context.Context, symbol string) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, priceTimeout)
	defer cancel()
	type res struct {
		p   float64
		err error
	}
	ch := make(chan res, 1)
	go func() {
		p, err := s.exch.GetPrice(symbol)
		ch <- res{p, err}
	}()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case r := <-ch:
		return r.p, r.err
	}
}

func (s *Service) fetchOHLCV(ctx context.Context, symbol, tf string, limit int) ([]market.Candle, error) {
	ctx, cancel := context.WithTimeout(ctx, ohlcvTimeout)
	defer cancel()
	type res struct {
		c   []market.Candle
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := s.exch.GetOHLCV(symbol, tf, limit)
		ch <- res{c, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.c, r.err
	}
}

// coreSignal builds Req/Resp without DB logging; shared by /signal and scheduler.
func (s *Service) coreSignal(ctx context.Context, symbol, tf, lang string) (*Outcome, error) {
	if !market.IsValidTimeframe(tf) {
		tf = "4h"
	}
	price, err := s.fetchPrice(ctx, symbol)
	if err != nil {
		return nil, err
	}
	candles, err := s.fetchOHLCV(ctx, symbol, tf, 200)
	if err != nil {
		return nil, err
	}
	if len(candles) < 60 {
		return nil, fmt.Errorf("%s: not enough history (%d candles)", symbol, len(candles))
	}
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	var volSum float64
	for _, c := range candles[max(0, len(candles)-6):] {
		volSum += c.Vol
	}
	rsi := market.RSI(closes, 14)
	macdHist, macdSignal, _ := market.MACD(closes)
	ema9 := market.EMA(closes, 9)
	ema21 := market.EMA(closes, 21)
	regime := regimeOf(candles, ema9, ema21, price)
	req := &ai.SignalRequest{
		Symbol: symbol, Lang: lang, Price: price, RSI: rsi, MACD: macdHist, MACDSignal: macdSignal,
		EMA9: ema9, EMA21: ema21, Volume: candles[len(candles)-1].Vol, AvgVolume: volSum / 6,
		MarketRegime: regime, Timeframe: tf,
	}
	slog.Info("indicators ready", "symbol", symbol, "price", price, "rsi", rsi,
		"macd_hist", macdHist, "ema9", ema9, "ema21", ema21, "regime", regime, "tf", tf)
	out := &Outcome{Symbol: symbol, Req: req, Source: "ai"}
	resp, err := s.ai.GenerateSignal(ctx, req)
	if err != nil {
		slog.Warn("llm unavailable, falling back to rules", "symbol", symbol, "err", err)
		resp = ai.FallbackSignal(req)
		out.Source = "fallback"
	}
	out.Resp = resp
	return out, nil
}

// OutcomeForBroadcast is coreSignal for scheduler: cached per (symbol,tf,lang).
func (s *Service) OutcomeForBroadcast(ctx context.Context, symbol, tf, lang string) (*Outcome, error) {
	return s.coreSignal(ctx, market.NormalizeSymbol(symbol), tf, lang)
}

func (s *Service) LogForUser(userID int64, out *Outcome) error {
	entry := &storage.Signal{
		UserID: userID, Symbol: out.Symbol, Signal: out.Resp.Signal, Confidence: out.Resp.Confidence,
		PriceAtTime: out.Req.Price, TargetPrice: out.Resp.TakeProfit, StopLoss: out.Resp.StopLoss,
	}
	return s.logs.Create(entry)
}

func FormatSignalLine(out *Outcome) string {
	return fmt.Sprintf("📊 %s — %s · %d%% · $%s (%s)", out.Symbol, out.Resp.Signal, out.Resp.Confidence, market.FormatPrice(out.Req.Price), out.Req.MarketRegime)
}

// ponytail: naive regime thresholds (2% per-4h move = volatile, 0.4% EMA spread = trend),
// tune against real data later
func regimeOf(candles []market.Candle, ema9, ema21, price float64) string {
	n := len(candles)
	start := max(n-31, 1)
	var sum float64
	var cnt int
	for i := start; i < n; i++ {
		prev := candles[i-1].Close
		if prev > 0 {
			sum += math.Abs(candles[i].Close-prev) / prev
			cnt++
		}
	}
	if cnt > 0 && sum/float64(cnt) > 0.02 {
		return "high volatility"
	}
	if price > 0 && math.Abs(ema9-ema21)/price > 0.004 {
		return "trending"
	}
	return "ranging"
}

func promptLang(languageCode string) string {
	if market.IsRu(languageCode) {
		return "Russian"
	}
	return "English"
}
