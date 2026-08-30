package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"arbbot/internal/ai"
	"arbbot/internal/bot"
	"arbbot/internal/config"
	"arbbot/internal/market"
	"arbbot/internal/scheduler"
	"arbbot/internal/service"
	"arbbot/internal/storage"
	"arbbot/internal/tracker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	setLogLevel(cfg.LogLevel)

	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	exch, err := market.NewExchange()
	if err != nil {
		slog.Error("init exchange", "err", err)
		os.Exit(1)
	}

	if cfg.LLMAPIKey == "" {
		slog.Warn("LLM_API_KEY is empty — /signal will use rule-based fallback only")
	}
	aiClient := ai.NewClient(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
	userRepo := storage.NewUserRepo(db)
	signalRepo := storage.NewSignalRepo(db)
	subRepo := storage.NewSubRepo(db)
	svc := service.New(cfg, exch, aiClient, signalRepo, userRepo, subRepo)

	b, err := bot.New(cfg, userRepo, signalRepo, subRepo, exch, svc)
	if err != nil {
		slog.Error("create bot", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		b.Start()
		stop()
	}()
	go scheduler.New(cfg, svc, subRepo, b).Run(ctx)
	go tracker.New(signalRepo, exch).Run(ctx)

	slog.Info("bot started", "symbols", cfg.Symbols, "interval", cfg.SignalInterval,
		"llm", cfg.LLMBaseURL+"/"+cfg.LLMModel)
	<-ctx.Done()
	slog.Info("shutting down")
	b.Stop()
}

func setLogLevel(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetLogLoggerLevel(l)
}
