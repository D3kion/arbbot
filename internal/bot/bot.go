package bot

import (
	"time"

	tele "gopkg.in/telebot.v4"

	"arbbot/internal/config"
	"arbbot/internal/market"
	"arbbot/internal/service"
	"arbbot/internal/storage"
)

type Bot struct {
	tb    *tele.Bot
	users *storage.UserRepo
	logs  *storage.SignalRepo
	subs  *storage.SubRepo
	exch  *market.Exchange
	svc   *service.Service
	cfg   *config.Config
}

func New(cfg *config.Config, users *storage.UserRepo, logs *storage.SignalRepo,
	subs *storage.SubRepo, exch *market.Exchange, svc *service.Service) (*Bot, error) {
	tb, err := tele.NewBot(tele.Settings{
		Token:  cfg.TelegramToken,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return nil, err
	}

	b := &Bot{tb: tb, users: users, logs: logs, subs: subs, exch: exch, svc: svc, cfg: cfg}
	b.registerHandlers()
	cmds := []tele.Command{
		{Text: "start", Description: "Register and launch the bot"},
		{Text: "help", Description: "Show available commands"},
		{Text: "menu", Description: "Main menu"},
		{Text: "price", Description: "Current price, e.g. /price BTC"},
		{Text: "signal", Description: "AI trading signal, e.g. /signal BTC"},
		{Text: "history", Description: "Recent signals"},
		{Text: "set_timeframe", Description: "Set timeframe (1h, 4h, 1d)"},
		{Text: "stats", Description: "Your signal statistics (premium)"},
		{Text: "my_subscription", Description: "Check your subscription status"},
		{Text: "subscribe", Description: "Premium plans info"},
	}
	if err := tb.SetCommands(cmds); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Bot) Start() { b.tb.Start() }

func (b *Bot) Stop() { b.tb.Stop() }

func (b *Bot) Send(to tele.Recipient, what interface{}, opts ...interface{}) (*tele.Message, error) {
	return b.tb.Send(to, what, opts...)
}


