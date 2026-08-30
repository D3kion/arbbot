package market

func emaSeries(prices []float64, period int) []float64 {
	out := make([]float64, len(prices))
	if len(prices) == 0 || period <= 0 {
		return out
	}
	if period > len(prices) {
		period = len(prices)
	}
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += prices[i]
	}
	out[period-1] = sum / float64(period)
	k := 2.0 / (float64(period) + 1)
	for i := period; i < len(prices); i++ {
		out[i] = out[i-1] + k*(prices[i]-out[i-1])
	}
	return out
}

func EMA(prices []float64, period int) float64 {
	s := emaSeries(prices, period)
	if len(s) == 0 {
		return 0
	}
	return s[len(s)-1]
}

func RSI(prices []float64, period int) float64 {
	n := len(prices)
	if n <= period || period <= 0 {
		return 50
	}
	var avgGain, avgLoss float64
	for i := 1; i <= period; i++ {
		d := prices[i] - prices[i-1]
		if d > 0 {
			avgGain += d
		} else {
			avgLoss -= d
		}
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)
	for i := period + 1; i < n; i++ {
		d := prices[i] - prices[i-1]
		gain, loss := 0.0, 0.0
		if d > 0 {
			gain = d
		} else {
			loss = -d
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
	}
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50
		}
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

func MACD(prices []float64) (macd, signal, hist float64) {
	const fast, slow, sig = 12, 26, 9
	fastE := emaSeries(prices, fast)
	slowE := emaSeries(prices, slow)
	start := slow - 1
	if len(prices) <= start {
		return 0, 0, 0
	}
	line := make([]float64, 0, len(prices)-start)
	for i := start; i < len(prices); i++ {
		line = append(line, fastE[i]-slowE[i])
	}
	macd = line[len(line)-1]
	signal = EMA(line, sig)
	hist = macd - signal
	return macd, signal, hist
}
