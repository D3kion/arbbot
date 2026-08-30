package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSubscriptions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subs := NewSubRepo(db)
	users := NewUserRepo(db)

	if active, _ := subs.HasActive(1); active {
		t.Fatal("no sub expected initially")
	}

	if err := users.CreateOrUpdate(&User{ID: 1}); err != nil {
		t.Fatal(err)
	}

	until, err := subs.Activate(1, "trader", 30)
	if err != nil {
		t.Fatal(err)
	}
	if !until.After(time.Now()) {
		t.Fatalf("expires in future: %v", until)
	}
	if active, _ := subs.HasActive(1); !active {
		t.Fatal("sub should be active")
	}

	if err := users.CreateOrUpdate(&User{ID: 1, LanguageCode: "ru"}); err != nil {
		t.Fatal(err)
	}
	list, err := subs.ListActive()
	if err != nil || len(list) != 1 || list[0].ID != 1 || list[0].LanguageCode != "ru" {
		t.Fatalf("ListActive = %v, %v", list, err)
	}

	first := until
	until2, err := subs.Activate(1, "trader", 30)
	if err != nil {
		t.Fatal(err)
	}
	want := first.AddDate(0, 0, 30)
	if until2.Unix() != want.Unix() {
		t.Fatalf("extension: got %v want %v", until2, want)
	}
}

func TestSignalStats(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logs := NewSignalRepo(db)
	users := NewUserRepo(db)

	if err := users.CreateOrUpdate(&User{ID: 7}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []Signal{
		{UserID: 7, Symbol: "BTC/USDT", Signal: "BUY", Confidence: 60},
		{UserID: 7, Symbol: "BTC/USDT", Signal: "HOLD", Confidence: 80},
		{UserID: 7, Symbol: "ETH/USDT", Signal: "SELL", Confidence: 70},
	} {
		cp := s
		if err := logs.Create(&cp); err != nil {
			t.Fatal(err)
		}
	}

	st, err := logs.StatsByUser(7)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 3 || st.Buy != 1 || st.Sell != 1 || st.Hold != 1 {
		t.Fatalf("stats: %+v", st)
	}
	if st.AvgConf < 69 || st.AvgConf > 71 {
		t.Fatalf("avg conf: %v", st.AvgConf)
	}
}
