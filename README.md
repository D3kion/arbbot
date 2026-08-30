# Crypto Signals Bot

Telegram bot that generates AI-assisted trading signals from market data.

Signals combine OHLCV indicators (RSI, MACD, EMA, regime detection) with an LLM. If the LLM is unavailable, a deterministic rule-based fallback is used. Free users are rate-limited; premium subscribers receive scheduled digests and unlimited on-demand signals.

## Features

- `/start` — register user (SQLite) and show welcome message
- `/help`, `/menu` — commands and inline navigation (EN/RU based on Telegram language)
- `/price <SYMBOL>` — spot price from Binance (10s cache, normalized: `BTC` → `BTC/USDT`)
- `/signal [SYMBOL]` — full pipeline: 200 candles → indicators → LLM → confidence/reason/SL/TP; persists to `signal_logs`; 5 free signals/day with UTC daily reset; defaults to last used symbol
- `/history [SYMBOL]` — last 5 signals with outcome
- `/set_timeframe <1h|4h|1d>` — per-user timeframe, applied to subsequent signals
- `/stats` — per-user totals and average confidence (premium only)
- `/my_subscription`, `/subscribe` — subscription status and premium information
- `/grant <USER_ID> [DAYS]` — admin-only premium grant (default 30 days, extends existing)
- `/feedback_stats` — admin aggregate feedback and 24h outcome stats
- Scheduler — every `SIGNAL_INTERVAL` generates one signal per symbol per language (batched, not per-user) and delivers to all active premium subscribers; also posts to `CHANNEL_ID` if configured
- Tracker — hourly background job checks TP/SL for signals older than 24h and records `win`/`loss`/`pending`

## Architecture

```
cmd/bot/main.go              wiring: config → db → exchange → ai → service → bot + scheduler + tracker
internal/config              env → typed Config
internal/storage             SQLite (modernc, WAL): users, subscriptions, signal_logs
internal/market/exchange.go  Binance via CCXT: GetPrice (cached), GetOHLCV, NormalizeSymbol
internal/market/indicators   RSI (Wilder, 14), MACD (12/26/9), EMA
internal/ai                  OpenAI-compatible client (Groq/OpenRouter), JSON parsing, rule fallback
internal/service             orchestration: market data → indicators → regime → AI → persistence; quota management
internal/scheduler           ticker-based broadcast, batched per (symbol, language)
internal/tracker             periodic TP/SL evaluation
internal/bot                 telebot v4 long polling, handlers, formatting, i18n
```

Database tables: `users`, `subscriptions`, `signal_logs` (with `outcome`, `feedback_action`, `stop_loss`, `target_price`, `last_symbol`, `timeframe`).

## Configuration

| Variable | Default | Description |
|---|---|---|
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token (required) |
| `LLM_BASE_URL` | `https://api.groq.com/openai/v1` | OpenAI-compatible endpoint |
| `LLM_API_KEY` | — | LLM API key; if empty, rule-based fallback only |
| `LLM_MODEL` | `llama-3.3-70b-versatile` | Model name |
| `DATABASE_PATH` | `./bot.db` | SQLite path (`/data/bot.db` in Docker) |
| `LOG_LEVEL` | `info` | `debug`/`info`/`warn`/`error` |
| `SIGNAL_INTERVAL` | `4h` | Scheduler interval |
| `FREE_SIGNALS_LIMIT` | `5` | Free signals per day |
| `SUPPORTED_SYMBOLS` | `BTC/USDT,ETH/USDT,SOL/USDT` | Symbols for scheduled digests |
| `ADMIN_IDS` | — | Comma-separated Telegram admin IDs |
| `CHANNEL_ID` | — | Public channel ID for auto-posting |

## Running

### Docker Compose (recommended)

```bash
cp .env.example .env   # set TELEGRAM_BOT_TOKEN and LLM keys
make up                # build and start
make compose-logs      # follow logs
make down              # stop
```

Data persists in Docker volume `bot_data` → `/data/bot.db`.

### Local

```bash
go mod download
cp .env.example .env
go run ./cmd/bot
```

First start takes ~6s (Binance market metadata load). DB is created automatically.

Verify:

```bash
go build ./... && go vet ./... && go test ./...
```

## Stack

Go 1.26 · telebot v4 · ccxt Go v4 (Binance) · modernc.org/sqlite (pure Go, no cgo)

## Disclaimer

Not financial advice. Crypto trading involves substantial risk. Signals are informational only; users are solely responsible for trading decisions.
