package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v4"

	"arbbot/internal/bot"
	"arbbot/internal/config"
	"arbbot/internal/market"
	"arbbot/internal/service"
	"arbbot/internal/storage"
)

type Scheduler struct {
	svc       *service.Service
	subs      *storage.SubRepo
	sender    *bot.Bot
	interval  time.Duration
	symbols   []string
	channelID int64
}

func New(cfg *config.Config, svc *service.Service, subs *storage.SubRepo, sender *bot.Bot) *Scheduler {
	return &Scheduler{
		svc:       svc,
		subs:      subs,
		sender:    sender,
		interval:  cfg.SignalInterval,
		symbols:   cfg.Symbols,
		channelID: cfg.ChannelID,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	slog.Info("scheduler started", "interval", s.interval, "symbols", s.symbols)
	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.broadcast(ctx)
		}
	}
}

const (
	maxRetries      = 3
	retryDelay      = 5 * time.Minute
	broadcastWorkers = 10
)

// ponytail: batched per (symbol,tf,lang) — distinct TFs * M*2 AI calls, not N*M
func (s *Scheduler) broadcast(ctx context.Context) {
	users, err := s.subs.ListActive()
	if err != nil {
		slog.Error("list premium users", "err", err)
		return
	}
	if len(users) == 0 {
		slog.Debug("broadcast: no active subscribers")
		return
	}
	slog.Info("broadcast started", "users", len(users), "symbols", len(s.symbols))

	// collect distinct timeframes among subscribers
	tfs := map[string]struct{}{}
	for _, u := range users {
		tf := u.Timeframe
		if !market.IsValidTimeframe(tf) {
			tf = "4h"
		}
		tfs[tf] = struct{}{}
	}
	if len(tfs) == 0 {
		tfs["4h"] = struct{}{}
	}

	cached := make(map[string]*service.Outcome, len(s.symbols)*len(tfs)*2)
	for tf := range tfs {
		for _, sym := range s.symbols {
			for _, lang := range []string{"English", "Russian"} {
				key := sym + "|" + tf + "|" + lang
				out, err := s.svc.OutcomeForBroadcast(ctx, sym, tf, lang)
				if err != nil {
					slog.Error("broadcast signal", "symbol", sym, "tf", tf, "lang", lang, "err", err)
					continue
				}
				cached[key] = out
			}
		}
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, broadcastWorkers)
	for _, u := range users {
		u := u
		lang := "English"
		if market.IsRu(u.LanguageCode) {
			lang = "Russian"
		}
		tf := u.Timeframe
		if !market.IsValidTimeframe(tf) {
			tf = "4h"
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var b strings.Builder
			fmt.Fprintf(&b, "⏰ Scheduled signals (%s):\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
			for _, sym := range s.symbols {
				out := cached[sym+"|"+tf+"|"+lang]
				if out == nil {
					continue
				}
				b.WriteString("\n")
				b.WriteString(service.FormatSignalLine(out))
				_ = s.svc.LogForUser(u.ID, out)
			}
			s.sendWithRetry(ctx, u.ID, b.String())
		}()
	}
	wg.Wait()

	if s.channelID != 0 {
		s.postToChannel(cached)
	}
}

func (s *Scheduler) sendWithRetry(ctx context.Context, userID int64, msg string) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, err := s.sender.Send(tele.ChatID(userID), msg)
		if err == nil {
			slog.Info("broadcast delivered", "user", userID)
			return
		}
		slog.Error("broadcast send failed", "user", userID, "attempt", attempt, "err", err)
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
		}
	}
	slog.Error("broadcast send gave up", "user", userID, "retries", maxRetries)
}

func (s *Scheduler) postToChannel(cached map[string]*service.Outcome) {
	var b strings.Builder
	fmt.Fprintf(&b, "⏰ Scheduled digest (%s):\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	for _, sym := range s.symbols {
		out := cached[sym+"|4h|English"]
		if out == nil {
			// fallback: any tf for this symbol
			for k, v := range cached {
				if len(k) > len(sym) && k[:len(sym)+1] == sym+"|" && strings.HasSuffix(k, "|English") {
					out = v
					break
				}
			}
		}
		if out == nil {
			continue
		}
		b.WriteString("\n")
		b.WriteString(service.FormatSignalLine(out))
	}
	if _, err := s.sender.Send(tele.ChatID(s.channelID), b.String()); err != nil {
		slog.Error("channel post", "err", err)
	}
}
