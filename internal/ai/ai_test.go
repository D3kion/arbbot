package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	r, err := Parse([]byte(`{"signal":"buy","confidence":150,"reason":"up","stop_loss":100,"take_profit":120}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.Signal != "BUY" {
		t.Fatalf("signal: %q", r.Signal)
	}
	if r.Confidence != 100 {
		t.Fatalf("confidence clamp: %d", r.Confidence)
	}
	if r.StopLoss == nil || *r.StopLoss != 100 {
		t.Fatalf("stop loss: %v", r.StopLoss)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("want error for garbage")
	}
	if _, err := Parse([]byte(`{"signal":"MAYBE","confidence":50,"reason":""}`)); err == nil {
		t.Fatal("want error for unknown signal")
	}
}

func TestFallbackRules(t *testing.T) {
	cases := []struct {
		rsi  float64
		want string
	}{
		{75, "SELL"},
		{25, "BUY"},
		{50, "HOLD"},
	}
	for _, c := range cases {
		got := FallbackSignal(&SignalRequest{RSI: c.rsi})
		if got.Signal != c.want {
			t.Fatalf("rsi %.0f: got %s want %s", c.rsi, got.Signal, c.want)
		}
	}
}

func TestGenerateSignalHTTP(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"signal\":\"HOLD\",\"confidence\":60,\"reason\":\"flat\"}"}}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key", "test-model")
	out, err := c.GenerateSignal(context.Background(), &SignalRequest{Symbol: "BTC/USDT", Lang: "English", RSI: 55})
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/chat/completions" {
		t.Fatalf("path: %s", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth: %s", gotAuth)
	}
	if gotBody["model"] != "test-model" || gotBody["temperature"] != 0.3 {
		t.Fatalf("payload: %v", gotBody)
	}
	if out.Signal != "HOLD" || out.Confidence != 60 {
		t.Fatalf("out: %+v", out)
	}

	msgs := gotBody["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].(string)
	for _, want := range []string{"BTC/USDT", "55", "JSON"} {
		if !strings.Contains(content, want) {
			t.Fatalf("prompt missing %q:\n%s", want, content)
		}
	}
}

func TestGenerateSignalServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "wrong", "m")
	_, err := c.GenerateSignal(context.Background(), &SignalRequest{})
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad key") {
		t.Fatalf("err = %v", err)
	}
}
