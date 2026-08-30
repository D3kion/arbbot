package tracker

import (
	"context"
	"log/slog"
	"time"

	"arbbot/internal/market"
	"arbbot/internal/storage"
)

const checkInterval = 1 * time.Hour

// ponytail: 24h minimum age before checking TP/SL; signals need time to play out
const minOutcomeAge = 24 * time.Hour

// ponytail: 7d max pending — stale signals become loss, avoid forever-pending
const maxPendingAge = 7 * 24 * time.Hour

type Tracker struct {
	signals *storage.SignalRepo
	exch    *market.Exchange
}

func New(signals *storage.SignalRepo, exch *market.Exchange) *Tracker {
	return &Tracker{signals: signals, exch: exch}
}

func (t *Tracker) Run(ctx context.Context) {
	slog.Info("tracker started")
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("tracker stopped")
			return
		case <-ticker.C:
			t.check()
		}
	}
}

func (t *Tracker) check() {
	pending, err := t.signals.PendingOutcome(minOutcomeAge)
	if err != nil {
		slog.Error("tracker: fetch pending", "err", err)
		return
	}
	if len(pending) == 0 {
		return
	}
	slog.Info("tracker: checking signals", "count", len(pending))

	for _, sig := range pending {
		price, err := getPriceWithTimeout(sig.Symbol, t.exch)
		if err != nil {
			slog.Error("tracker: get price", "symbol", sig.Symbol, "err", err)
			continue
		}
		outcome := evaluateOutcome(sig, price)
		if err := t.signals.SetOutcome(sig.ID, outcome); err != nil {
			slog.Error("tracker: set outcome", "id", sig.ID, "err", err)
			continue
		}
		slog.Info("tracker: outcome recorded", "id", sig.ID, "symbol", sig.Symbol,
			"signal", sig.Signal, "price", price, "outcome", outcome)
	}
}

func getPriceWithTimeout(symbol string, exch *market.Exchange) (float64, error) {
	ch := make(chan struct {
		p   float64
		err error
	}, 1)
	go func() {
		p, err := exch.GetPrice(symbol)
		ch <- struct {
			p   float64
			err error
		}{p, err}
	}()
	select {
	case r := <-ch:
		return r.p, r.err
	case <-time.After(15 * time.Second):
		return 0, context.DeadlineExceeded
	}
}

func evaluateOutcome(sig storage.Signal, currentPrice float64) string {
	if sig.TargetPrice == nil {
		return "pending"
	}
	// stale pending -> loss to avoid forever-pending
	if !sig.CreatedAt.IsZero() && time.Since(sig.CreatedAt) > maxPendingAge {
		return "loss"
	}
	tp := *sig.TargetPrice

	switch sig.Signal {
	case "BUY":
		if currentPrice >= tp {
			return "win"
		}
		if sig.StopLoss != nil && currentPrice <= *sig.StopLoss {
			return "loss"
		}
	case "SELL":
		if currentPrice <= tp {
			return "win"
		}
		if sig.StopLoss != nil && currentPrice >= *sig.StopLoss {
			return "loss"
		}
	}
	return "pending"
}
