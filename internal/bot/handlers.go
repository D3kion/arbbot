package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	tele "gopkg.in/telebot.v4"

	"arbbot/internal/market"
	"arbbot/internal/service"
	"arbbot/internal/storage"
)

const disclaimerEN = "\n\n⚠️ Not financial advice. Crypto trading involves high risk. You are fully responsible for your trading decisions."

const helpEN = `Available commands:

/start — register and start
/help — show this help
/menu — main menu
/price <SYMBOL> — current price (e.g. /price BTC)
/signal <SYMBOL> — AI trading signal (5 free per day)
/history [SYMBOL] — recent signals
/set_timeframe <TF> — set timeframe (1h, 4h, 1d)
/stats — your signal statistics (premium)
/my_subscription — check your subscription status
/subscribe — premium plans info`

const helpRU = `Доступные команды:

/start — регистрация и запуск
/help — показать эту справку
/menu — главное меню
/price <СИМВОЛ> — текущая цена (например, /price BTC)
/signal <СИМВОЛ> — AI-сигнал (5 бесплатных в день)
/history [СИМВОЛ] — последние сигналы
/set_timeframe <TF> — установить таймфрейм (1h, 4h, 1d)
/stats — статистика ваших сигналов (премиум)
/my_subscription — статус вашей подписки
/subscribe — информация о премиум-тарифах`

const usageSignal = "Usage: /signal <SYMBOL> (e.g. /signal BTC)\nИспользование: /signal <СИМВОЛ> (например, /signal BTC)"

const limitReachedEN = "🚫 Daily free limit reached (%d signals). Premium with unlimited signals is coming soon."
const limitReachedRU = "🚫 Дневной бесплатный лимит исчерпан (%d сигналов). Премиум с безлимитом скоро появится."

const premiumOnlyEN = "🔒 /stats is available for premium users. Type /subscribe to learn more."
const premiumOnlyRU = "🔒 /stats доступен только премиум-пользователям. Напишите /subscribe, чтобы узнать больше."

const subscribeTextEN = `💎 Premium — coming soon:
• Unlimited signals + scheduled digests
Payments in USDT (TRC20) will be enabled soon.`

const subscribeTextRU = `💎 Премиум — скоро:
• Безлимитные сигналы + плановые рассылки
Оплата в USDT (TRC20) появится скоро.`

const (
	cbDetails = "sig_details"
	cbRisks   = "sig_risks"
	cbEntered = "sig_entered"
	cbSkipped = "sig_skipped"
	cbMenuSignal = "menu_signal"
	cbMenuSub    = "menu_sub"
	cbMenuStats  = "menu_stats"
	cbMenuHelp   = "menu_help"
)

// signalCache: messageID -> outcome (for inline button callbacks)
// ponytail: bounded to 1000 entries, clear on overflow; per-message state doesn't need persistence
const maxSignalCache = 1000

var (
	signalCache   = make(map[int64]*service.Outcome)
	signalCacheMu sync.Mutex
)

func cacheOutcome(msgID int64, out *service.Outcome) {
	signalCacheMu.Lock()
	defer signalCacheMu.Unlock()
	if len(signalCache) >= maxSignalCache {
		clear(signalCache)
	}
	signalCache[msgID] = out
}

func lookupOutcome(msgID int64) (*service.Outcome, bool) {
	signalCacheMu.Lock()
	defer signalCacheMu.Unlock()
	out, ok := signalCache[msgID]
	return out, ok
}

func (b *Bot) isAdmin(id int64) bool {
	for _, a := range b.cfg.AdminIDs {
		if a == id {
			return true
		}
	}
	return false
}

func (b *Bot) registerHandlers() {
	b.tb.Handle("/start", b.onStart)
	b.tb.Handle("/help", b.onHelp)
	b.tb.Handle("/menu", b.onMenu)
	b.tb.Handle("/price", b.onPrice)
	b.tb.Handle("/signal", b.onSignal)
	b.tb.Handle("/history", b.onHistory)
	b.tb.Handle("/set_timeframe", b.onSetTimeframe)
	b.tb.Handle("/stats", b.onStats)
	b.tb.Handle("/my_subscription", b.onMySubscription)
	b.tb.Handle("/subscribe", b.onSubscribe)
	b.tb.Handle("/grant", b.onGrant)
	b.tb.Handle("/feedback_stats", b.onFeedbackStats)

	// inline button callbacks (unique strings must match btn.Data second arg)
	b.tb.Handle("\f"+cbDetails, b.onSignalCb)
	b.tb.Handle("\f"+cbRisks, b.onSignalCb)
	b.tb.Handle("\f"+cbEntered, b.onSignalCb)
	b.tb.Handle("\f"+cbSkipped, b.onSignalCb)
	b.tb.Handle("\f"+cbMenuSignal, b.onMenuCb)
	b.tb.Handle("\f"+cbMenuSub, b.onMenuCb)
	b.tb.Handle("\f"+cbMenuStats, b.onMenuCb)
	b.tb.Handle("\f"+cbMenuHelp, b.onMenuCb)
}

func tr(ru bool, en, ruStr string) string {
	if ru {
		return ruStr
	}
	return en
}

func welcomeText(lang string) string {
	if market.IsRu(lang) {
		return helpRU + disclaimerEN
	}
	return helpEN + disclaimerEN
}

func (b *Bot) onStart(c tele.Context) error {
	u := b.ensureUser(c)
	if u == nil {
		return nil
	}
	slog.Info("user registered", "id", u.ID, "username", u.Username)
	return c.Send("👋 Welcome to Crypto Signals Bot!\n\n" + welcomeText(u.LanguageCode))
}

func (b *Bot) onHelp(c tele.Context) error {
	u := c.Sender()
	lang := ""
	if u != nil {
		lang = u.LanguageCode
	}
	return c.Send(welcomeText(lang))
}

func (b *Bot) onMenu(c tele.Context) error {
	sender := b.ensureUser(c)
	if sender == nil {
		return nil
	}
	ru := market.IsRu(sender.LanguageCode)
	text := tr(ru, "🏠 Main menu", "🏠 Главное меню")

	markup := &tele.ReplyMarkup{}
	signalBtn := markup.Data(tr(ru, "Signal", "Сигнал"), "menu_signal", "signal")
	subBtn := markup.Data(tr(ru, "Subscription", "Подписка"), "menu_sub", "sub")
	statsBtn := markup.Data(tr(ru, "Stats", "Статистика"), "menu_stats", "stats")
	helpBtn := markup.Data(tr(ru, "Help", "Помощь"), "menu_help", "help")

	markup.Inline(
		tele.Row{signalBtn, subBtn},
		tele.Row{statsBtn, helpBtn},
	)

	return c.Send(text, markup)
}

func (b *Bot) onPrice(c tele.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return c.Send("Usage: /price <SYMBOL> (e.g. /price BTC)\nИспользование: /price <СИМВОЛ> (например, /price BTC)")
	}
	symbol := market.NormalizeSymbol(args[0])

	price, err := b.exch.GetPrice(symbol)
	if err != nil {
		slog.Error("get price", "symbol", symbol, "err", err)
		return c.Send("❌ Failed to fetch price for " + symbol + ". Check the symbol and try again.")
	}
	return c.Send(fmt.Sprintf("%s: $%s", symbol, market.FormatPrice(price)))
}

func (b *Bot) ensureUser(c tele.Context) *tele.User {
	u := c.Sender()
	if u == nil {
		return nil
	}
	err := b.users.CreateOrUpdate(&storage.User{
		ID:           u.ID,
		Username:     u.Username,
		FirstName:    u.FirstName,
		LanguageCode: u.LanguageCode,
	})
	if err != nil {
		slog.Error("register user", "id", u.ID, "err", err)
	}
	return u
}

func (b *Bot) onSignal(c tele.Context) error {
	args := c.Args()
	sender := b.ensureUser(c)
	if sender == nil {
		return c.Send("❌ Could not identify user.")
	}

	var input string
	if len(args) > 0 {
		input = args[0]
	} else {
		// default to last used symbol
		user, err := b.users.GetByID(sender.ID)
		if err == nil && user.LastSymbol != "" {
			input = user.LastSymbol
		}
	}
	if input == "" {
		return c.Send(usageSignal)
	}
	slog.Info("command /signal", "user", sender.ID, "username", sender.Username, "input", input)

	premium, remaining, err := b.svc.CheckQuota(sender.ID)
	if err != nil {
		slog.Error("check quota", "user", sender.ID, "err", err)
		return c.Send("❌ Internal error, try again later.")
	}
	ru := market.IsRu(sender.LanguageCode)
	if !premium && remaining <= 0 {
		slog.Info("quota exhausted", "user", sender.ID)
		return c.Send(fmt.Sprintf(tr(ru, limitReachedEN, limitReachedRU), b.cfg.FreeSignalsLimit))
	}
	// atomic reserve before LLM to avoid race exceeding limit
	if !premium {
		ok, err := b.svc.TryConsumeFreeSignal(sender.ID)
		if err != nil {
			slog.Error("consume quota", "user", sender.ID, "err", err)
			return c.Send("❌ Internal error, try again later.")
		}
		if !ok {
			slog.Info("quota exhausted (race)", "user", sender.ID)
			return c.Send(fmt.Sprintf(tr(ru, limitReachedEN, limitReachedRU), b.cfg.FreeSignalsLimit))
		}
	}

	_ = c.Notify(tele.Typing)

	out, err := b.svc.MakeSignal(context.Background(), sender.ID, input, sender.LanguageCode)
	if err != nil {
		slog.Error("make signal", "user", sender.ID, "input", input, "err", err)
		return c.Send("❌ Failed to generate signal for " + market.NormalizeSymbol(input) + ". Try again later.")
	}

	sent, err := c.Bot().Send(c.Chat(), formatBrief(out, ru), b.signalMarkup(out.Symbol, ru))
	if err != nil {
		return c.Send(formatBrief(out, ru))
	}
	cacheOutcome(int64(sent.ID), out)
	b.postToChannel(out)
	_ = b.users.SetLastSymbol(sender.ID, out.Symbol)
	return nil
}

func (b *Bot) postToChannel(out *service.Outcome) {
	if b.cfg.ChannelID == 0 {
		return
	}
	text := formatDetails(out, false)
	if _, err := b.tb.Send(tele.ChatID(b.cfg.ChannelID), text); err != nil {
		slog.Error("channel post", "err", err)
	}
}

func (b *Bot) signalMarkup(symbol string, ru bool) *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	detailBtn := markup.Data("📋 "+tr(ru, "Details", "Детали"), cbDetails, symbol)
	risksBtn := markup.Data("⚠️ "+tr(ru, "Risks", "Риски"), cbRisks, symbol)
	enteredBtn := markup.Data("✅ "+tr(ru, "Entered", "Вошёл"), cbEntered, symbol)
	skippedBtn := markup.Data("❌ "+tr(ru, "Skipped", "Пропустил"), cbSkipped, symbol)
	markup.Inline(
		tele.Row{detailBtn, risksBtn},
		tele.Row{enteredBtn, skippedBtn},
	)
	return markup
}

func (b *Bot) onSignalCb(c tele.Context) error {
 cb := c.Callback()
 if cb == nil {
 	return nil
 }
 symbol := cb.Data

 sender := c.Sender()
 ru := sender != nil && market.IsRu(sender.LanguageCode)

 switch cb.Unique {
 case cbEntered, cbSkipped:
 	out, ok := lookupOutcome(int64(c.Message().ID))
 	if ok {
 		action := tr(cb.Unique == cbSkipped, "entered", "not_entered")
 		if err := b.logs.SetFeedback(out.SignalID, action); err != nil {
 			slog.Error("save feedback", "err", err)
 		}
 	}
 	return c.Respond()

 case cbDetails, cbRisks:
 	out, ok := lookupOutcome(int64(c.Message().ID))
 	if !ok {
 		return c.Respond()
 	}
 	text := tr(cb.Unique == cbDetails, formatRisks(out, ru), formatDetails(out, ru))
 	_, err := c.Bot().Edit(c.Message(), text, b.signalMarkup(symbol, ru))
 	if err != nil {
 		slog.Error("edit signal message", "err", err)
 	}
 	return c.Respond()
 }
 return c.Respond()
}

func (b *Bot) onMenuCb(c tele.Context) error {
	cb := c.Callback()
	if cb == nil {
		return nil
	}
	_ = c.Respond()

	switch cb.Unique {
	case cbMenuSignal:
		return c.Send("Используйте /signal <СИМВОЛ>\nUse /signal <SYMBOL>")
	case cbMenuSub:
		return b.onSubscribe(c)
	case cbMenuStats:
		return b.onStats(c)
	case cbMenuHelp:
		return b.onHelp(c)
	}
	return nil
}

func (b *Bot) onSetTimeframe(c tele.Context) error {
	sender := b.ensureUser(c)
	if sender == nil {
		return nil
	}
	ru := market.IsRu(sender.LanguageCode)
	args := c.Args()
	if len(args) == 0 {
		return c.Send(tr(ru, "Usage: /set_timeframe <1h|4h|1d>", "Использование: /set_timeframe <1h|4h|1d>\nТекущий: "+sender.LanguageCode))
	}
	tf := strings.ToLower(args[0])
	if !market.IsValidTimeframe(tf) {
		return c.Send(tr(ru, "Valid timeframes: 1h, 4h, 1d", "Допустимые значения: 1h, 4h, 1d"))
	}
	if err := b.users.SetTimeframe(sender.ID, tf); err != nil {
		slog.Error("set timeframe", "user", sender.ID, "err", err)
		return c.Send("❌ Internal error, try again later.")
	}
	return c.Send(fmt.Sprintf(tr(ru, "✅ Timeframe set to %s", "✅ Таймфрейм установлен: %s"), tf))
}

func (b *Bot) onHistory(c tele.Context) error {
	sender := b.ensureUser(c)
	if sender == nil {
		return nil
	}
	ru := market.IsRu(sender.LanguageCode)
	args := c.Args()

	var symbol string
	if len(args) > 0 {
		symbol = market.NormalizeSymbol(args[0])
	}

	signals, err := b.logs.RecentByUser(sender.ID, symbol, 5)
	if err != nil {
		slog.Error("history", "user", sender.ID, "err", err)
		return c.Send("❌ Internal error, try again later.")
	}
	if len(signals) == 0 {
		return c.Send(tr(ru, "📭 No recent signals.", "📭 Нет сигналов за последнее время."))
	}

	var bld strings.Builder
	fmt.Fprintf(&bld, tr(ru, "📜 Recent signals (%s):\n\n", "📜 Последние сигналы (%s):\n\n"), symbol)
	for _, s := range signals {
		outcome := ""
		if s.Outcome != "" {
			outcome = " · " + s.Outcome
		}
		fmt.Fprintf(&bld, "%s %s — %s · %d%% · $%s%s\n",
			s.CreatedAt.Format("01-02 15:04"),
			s.Symbol, s.Signal, s.Confidence,
			market.FormatPrice(s.PriceAtTime), outcome)
	}
	return c.Send(bld.String())
}

func (b *Bot) onStats(c tele.Context) error {
	sender := b.ensureUser(c)
	if sender == nil {
		return nil
	}
	slog.Info("command /stats", "user", sender.ID)
	ru := market.IsRu(sender.LanguageCode)

	premium, _, err := b.svc.CheckQuota(sender.ID)
	if err != nil {
		slog.Error("check premium", "user", sender.ID, "err", err)
		return c.Send("❌ Internal error, try again later.")
	}
	if !premium {
		return c.Send(tr(ru, premiumOnlyEN, premiumOnlyRU))
	}

	st, err := b.logs.StatsByUser(sender.ID)
	if err != nil {
		slog.Error("user stats", "user", sender.ID, "err", err)
		return c.Send("❌ Internal error, try again later.")
	}
	return c.Send(fmt.Sprintf(tr(ru,
		"📈 Your statistics:\n\nTotal signals: %d\nBUY: %d · SELL: %d · HOLD: %d\nAvg confidence: %.0f%%",
		"📈 Ваша статистика:\n\nВсего сигналов: %d\nBUY: %d · SELL: %d · HOLD: %d\nСредняя уверенность: %.0f%%"),
		st.Total, st.Buy, st.Sell, st.Hold, st.AvgConf))
}

func (b *Bot) onSubscribe(c tele.Context) error {
	sender := b.ensureUser(c)
	if sender == nil {
		return nil
	}
	slog.Info("command /subscribe", "user", sender.ID)
	return c.Send(tr(market.IsRu(sender.LanguageCode), subscribeTextEN, subscribeTextRU) + "\n\n" + disclaimerEN)
}

func (b *Bot) onMySubscription(c tele.Context) error {
	sender := b.ensureUser(c)
	if sender == nil {
		return nil
	}
	slog.Info("command /my_subscription", "user", sender.ID)
	ru := market.IsRu(sender.LanguageCode)

	info, err := b.subs.GetActive(sender.ID)
	if err != nil {
		slog.Error("get subscription", "user", sender.ID, "err", err)
		return c.Send("❌ Internal error, try again later.")
	}
	if info == nil {
		return c.Send(tr(ru,
			"📭 You have no active subscription.\n\nType /subscribe to learn about plans.",
			"📭 У вас нет активной подписки.\n\nНапишите /subscribe, чтобы узнать о тарифах."))
	}
	remaining := time.Until(info.ExpiresAt)
	days := int(remaining.Hours() / 24)
	return c.Send(fmt.Sprintf(tr(ru,
		"💎 Your subscription:\n\nPlan: %s\nValid until: %s\nRemaining: ~%d days",
		"💎 Ваша подписка:\n\nТариф: %s\nДействует до: %s\nОсталось: ~%d дн."),
		info.Tier, info.ExpiresAt.Format("2006-01-02"), days))
}

func (b *Bot) onGrant(c tele.Context) error {
	sender := c.Sender()
	if sender == nil {
		return nil
	}
	slog.Info("command /grant", "user", sender.ID)
	if !b.isAdmin(sender.ID) {
		slog.Warn("grant denied, not admin", "user", sender.ID)
		return c.Send("❌ Admin only.")
	}
	args := c.Args()
	if len(args) == 0 {
		return c.Send("Usage: /grant <USER_ID> [DAYS] (default 30)")
	}
	userID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Invalid USER_ID.")
	}
	days := 30
	if len(args) > 1 {
		if d, err := strconv.Atoi(args[1]); err == nil && d > 0 && d <= 365 {
			days = d
		}
	}
	until, err := b.svc.GrantPremium(userID, days)
	if err != nil {
		slog.Error("grant premium", "target", userID, "err", err)
		return c.Send("❌ Failed to activate subscription.")
	}
	slog.Info("premium granted", "admin", sender.ID, "target", userID, "days", days)

	existing, _ := b.subs.GetActive(userID)
	if existing != nil && existing.ExpiresAt.After(time.Now()) {
		return c.Send(fmt.Sprintf("✅ Subscription extended for %d until %s (%s).",
			userID, until.Format("2006-01-02"), existing.Tier))
	}
	return c.Send(fmt.Sprintf("✅ Premium activated for %d until %s (trader).",
		userID, until.Format("2006-01-02")))
}

func (b *Bot) onFeedbackStats(c tele.Context) error {
	sender := c.Sender()
	if sender == nil || !b.isAdmin(sender.ID) {
		return c.Send("❌ Admin only.")
	}
	slog.Info("command /feedback_stats", "user", sender.ID)

	fs, err := b.logs.FeedbackStats()
	if err != nil {
		slog.Error("feedback stats", "err", err)
		return c.Send("❌ Internal error, try again later.")
	}
	total, wins, losses, pending, err := b.logs.OutcomeStats()
	if err != nil {
		slog.Error("outcome stats", "err", err)
		return c.Send("❌ Internal error, try again later.")
	}

	return c.Send(fmt.Sprintf(
		"📊 Feedback:\nTotal signals: %d\nEntered: %d\nNot entered: %d\nNo feedback: %d\n\n📈 Outcomes (24h+):\nTotal tracked: %d\nWins: %d\nLosses: %d\nPending: %d",
		fs.Total, fs.Entered, fs.NotEntered, fs.Pending,
		total, wins, losses, pending))
}
