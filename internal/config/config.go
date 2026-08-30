package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken    string
	LLMBaseURL       string
	LLMAPIKey        string
	LLMModel         string
	DBPath           string
	LogLevel         string
	SignalInterval   time.Duration
	FreeSignalsLimit int
	Symbols          []string
	AdminIDs         []int64
	ChannelID        int64
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	interval, err := time.ParseDuration(getEnv("SIGNAL_INTERVAL", "4h"))
	if err != nil {
		return nil, err
	}
	limit, err := strconv.Atoi(getEnv("FREE_SIGNALS_LIMIT", "5"))
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		TelegramToken:    os.Getenv("TELEGRAM_BOT_TOKEN"),
		LLMBaseURL:       getEnv("LLM_BASE_URL", "https://api.groq.com/openai/v1"),
		LLMAPIKey:        os.Getenv("LLM_API_KEY"),
		LLMModel:         getEnv("LLM_MODEL", "llama-3.3-70b-versatile"),
		DBPath:           getEnv("DATABASE_PATH", "./bot.db"),
		LogLevel:         getEnv("LOG_LEVEL", "info"),
		SignalInterval:   interval,
		FreeSignalsLimit: limit,
		Symbols:          strings.Split(getEnv("SUPPORTED_SYMBOLS", "BTC/USDT,ETH/USDT,SOL/USDT"), ","),
	}

	for _, id := range strings.Split(getEnv("ADMIN_IDS", ""), ",") {
		if v, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err == nil {
			cfg.AdminIDs = append(cfg.AdminIDs, v)
		}
	}

	if ch := getEnv("CHANNEL_ID", ""); ch != "" {
		if v, err := strconv.ParseInt(ch, 10, 64); err == nil {
			cfg.ChannelID = v
		}
	}

	if cfg.TelegramToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
