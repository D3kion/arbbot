package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestGetFreshDailyReset(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewUserRepo(db)

	if err := r.CreateOrUpdate(&User{ID: 1, Username: "u", FirstName: "U", LanguageCode: "en"}); err != nil {
		t.Fatal(err)
	}

	u, err := r.GetFresh(1)
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().UTC().Format(dayLayout)
	if u.FreeSignalsUsed != 0 || u.FreeSignalsResetDate != today {
		t.Fatalf("fresh user: used=%d date=%q", u.FreeSignalsUsed, u.FreeSignalsResetDate)
	}

	for i := 0; i < 3; i++ {
		if err := r.IncrementFreeSignals(1); err != nil {
			t.Fatal(err)
		}
	}
	u, _ = r.GetFresh(1)
	if u.FreeSignalsUsed != 3 || u.FreeSignalsResetDate != today {
		t.Fatalf("after increments: used=%d date=%q", u.FreeSignalsUsed, u.FreeSignalsResetDate)
	}
}
