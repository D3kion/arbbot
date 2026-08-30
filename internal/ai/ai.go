package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func buildPrompt(r *SignalRequest) string {
	tf := r.Timeframe
	if tf == "" {
		tf = "4h"
	}
	return fmt.Sprintf(`You are an experienced crypto trader with 10 years of experience. Your task is to give a trading signal for %s based on the provided data.

Current data (%s timeframe):
- Price: $%v
- RSI (14): %v (scale: <30 oversold, >70 overbought)
- MACD histogram: %v, MACD signal line: %v (bullish crossover if MACD > signal)
- EMA 9: $%v, EMA 21: $%v (price above both — bullish trend)
- Volume (current %s candle): %v vs average volume: %v
- Market regime: %s

Decision rules:
1. Weigh all indicators, do not rely on a single one.
2. Assess risk: if RSI > 70 and price is stretched — a correction signal.
3. If MACD shows a bullish crossover but volume is low — the signal is weak.
4. If the market is ranging — prefer HOLD, or wait for a breakout.
5. If confidence < 60%% — issue HOLD.

Respond strictly in JSON format:
{
  "signal": "BUY" or "SELL" or "HOLD",
  "confidence": integer from 0 to 100,
  "reason": "brief explanation in %s",
  "stop_loss": price for stop loss (optional),
  "take_profit": price for take profit (optional)
}

Only JSON, no additional text.`, r.Symbol, tf, r.Price, r.RSI, r.MACD, r.MACDSignal, r.EMA9, r.EMA21, tf, r.Volume, r.AvgVolume, r.MarketRegime, r.Lang)
}

type SignalRequest struct {
	Symbol       string
	Lang         string
	Price        float64
	RSI          float64
	MACD         float64
	MACDSignal   float64
	EMA9         float64
	EMA21        float64
	Volume       float64
	AvgVolume    float64
	MarketRegime string
	Timeframe    string
}

type SignalResponse struct {
	Signal     string   `json:"signal"`
	Confidence int      `json:"confidence"`
	Reason     string   `json:"reason"`
	StopLoss   *float64 `json:"stop_loss,omitempty"`
	TakeProfit *float64 `json:"take_profit,omitempty"`
}

type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client
}

func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		http:    &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *Client) GenerateSignal(ctx context.Context, req *SignalRequest) (*SignalResponse, error) {
	prompt := buildPrompt(req)

	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature":     0.3,
		"response_format": map[string]string{"type": "json_object"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	start := time.Now()
	slog.Info("llm request", "model", c.model, "symbol", req.Symbol)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	var raw struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("llm response decode (status %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || len(raw.Choices) == 0 {
		msg := ""
		if raw.Error != nil {
			msg = raw.Error.Message
		}
		return nil, fmt.Errorf("llm status %d: %s", resp.StatusCode, msg)
	}

	out, err := Parse([]byte(raw.Choices[0].Message.Content))
	if err != nil {
		return nil, fmt.Errorf("parse llm content: %w", err)
	}
	slog.Info("llm signal", "symbol", req.Symbol, "signal", out.Signal,
		"confidence", out.Confidence, "latency_ms", time.Since(start).Milliseconds())
	return out, nil
}

func Parse(data []byte) (*SignalResponse, error) {
	var r SignalResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	r.Signal = strings.ToUpper(strings.TrimSpace(r.Signal))
	switch r.Signal {
	case "BUY", "SELL", "HOLD":
	default:
		return nil, fmt.Errorf("invalid signal %q", r.Signal)
	}
	r.Confidence = min(max(r.Confidence, 0), 100)
	return &r, nil
}

func FallbackSignal(req *SignalRequest) *SignalResponse {
	switch {
	case req.RSI > 70:
		return &SignalResponse{Signal: "SELL", Confidence: 55,
			Reason: fmt.Sprintf("Rule-based fallback: RSI %.1f indicates overbought conditions.", req.RSI)}
	case req.RSI < 30:
		return &SignalResponse{Signal: "BUY", Confidence: 55,
			Reason: fmt.Sprintf("Rule-based fallback: RSI %.1f indicates oversold conditions.", req.RSI)}
	default:
		return &SignalResponse{Signal: "HOLD", Confidence: 60,
			Reason: fmt.Sprintf("Rule-based fallback: RSI %.1f is neutral.", req.RSI)}
	}
}
