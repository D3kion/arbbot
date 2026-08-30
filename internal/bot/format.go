package bot

import (
	"fmt"

	"arbbot/internal/ai"
	"arbbot/internal/market"
	"arbbot/internal/service"
)

func formatBrief(out *service.Outcome, ru bool) string {
	price := market.FormatPrice(out.Req.Price)
	conf, priceLabel := "confidence", "Price"
	if ru {
		conf, priceLabel = "уверенность", "Цена"
	}
	return fmt.Sprintf("📊 %s — %s · %s %d%%\n💰 %s: $%s",
		out.Symbol, out.Resp.Signal, conf, out.Resp.Confidence, priceLabel, price)
}

func formatDetails(out *service.Outcome, ru bool) string {
	price := market.FormatPrice(out.Req.Price)
	conf, priceLabel, indLabel, regimeLabel := "confidence", "Price", "Indicators", "Regime"
	if ru {
		conf, priceLabel, indLabel, regimeLabel = "уверенность", "Цена", "Индикаторы", "Режим"
	}
	return fmt.Sprintf("📊 %s — %s · %s %d%%\n💰 %s: $%s\n📝 %s\n\n📈 %s: RSI %.1f · MACD %.4f · EMA9 $%.2f · EMA21 $%.2f\n🌍 %s: %s",
		out.Symbol, out.Resp.Signal, conf, out.Resp.Confidence, priceLabel, price, out.Resp.Reason,
		indLabel, out.Req.RSI, out.Req.MACD, out.Req.EMA9, out.Req.EMA21,
		regimeLabel, out.Req.MarketRegime)
}

func formatRisks(out *service.Outcome, ru bool) string {
	s := formatBrief(out, ru)
	s += optionalLevels(out.Resp)
	disclaimer := "⚠️ Not financial advice. Crypto trading involves high risk."
	if ru {
		disclaimer = "⚠️ Это не финансовая рекомендация. Торговля криптовалютами сопряжена с высоким риском."
	}
	return s + "\n\n" + disclaimer
}

func optionalLevels(r *ai.SignalResponse) string {
	s := ""
	if r.StopLoss != nil {
		s += fmt.Sprintf("\n🛑 SL: $%s", market.FormatPrice(*r.StopLoss))
	}
	if r.TakeProfit != nil {
		s += fmt.Sprintf("\n🎯 TP: $%s", market.FormatPrice(*r.TakeProfit))
	}
	return s
}
