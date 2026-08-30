package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS users (
    user_id INTEGER PRIMARY KEY,
    username TEXT,
    first_name TEXT,
    language_code TEXT,
    registered_at INTEGER,
    free_signals_used INTEGER DEFAULT 0,
    free_signals_reset_date TEXT,
    last_symbol TEXT DEFAULT '',
    timeframe TEXT DEFAULT '4h'
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    tier TEXT NOT NULL,
    status TEXT DEFAULT 'active',
    expires_at INTEGER,
    created_at INTEGER,
    FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS signal_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    symbol TEXT,
    signal TEXT,
    confidence INTEGER,
    price_at_time REAL,
    target_price REAL,
    stop_loss REAL,
    outcome TEXT,
    feedback_action TEXT,
    created_at INTEGER,
    FOREIGN KEY (user_id) REFERENCES users(user_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_signal_logs_user_created ON signal_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_signal_logs_pending ON signal_logs(outcome, target_price, created_at) WHERE outcome IS NULL;
CREATE INDEX IF NOT EXISTS idx_subs_active ON subscriptions(user_id, status, expires_at);
`)
	return err
}
