package storage

import (
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

type SubRepo struct {
	db *sql.DB
}

func NewSubRepo(db *sql.DB) *SubRepo { return &SubRepo{db: db} }

func (r *SubRepo) HasActive(userID int64) (bool, error) {
	info, err := r.GetActive(userID)
	if err != nil {
		return false, err
	}
	return info != nil, nil
}

func (r *SubRepo) Activate(userID int64, tier string, days int) (time.Time, error) {
	now := time.Now().UTC()
	expires := now.AddDate(0, 0, days)

	var curID sql.NullInt64
	var curExpires sql.NullInt64
	err := r.db.QueryRow(
		`SELECT id, expires_at FROM subscriptions WHERE user_id = ? AND status = 'active' AND expires_at > ? ORDER BY expires_at DESC LIMIT 1`,
		userID, now.Unix()).Scan(&curID, &curExpires)
	if err == nil && curID.Valid && curExpires.Valid {
		expires = time.Unix(curExpires.Int64, 0).AddDate(0, 0, days).UTC()
		if _, err := r.db.Exec(`UPDATE subscriptions SET expires_at = ?, tier = ? WHERE id = ?`,
			expires.Unix(), tier, curID.Int64); err != nil {
			return time.Time{}, err
		}
		slog.Info("subscription extended", "user", userID, "tier", tier, "until", expires.Format(dayLayout))
		return expires, nil
	}
	if err != nil && err != sql.ErrNoRows {
		return time.Time{}, err
	}

	_, err = r.db.Exec(`
INSERT INTO subscriptions (user_id, tier, status, expires_at, created_at)
VALUES (?, ?, 'active', ?, ?)`,
		userID, tier, expires.Unix(), now.Unix())
	if err != nil {
		return time.Time{}, err
	}
	slog.Info("subscription activated", "user", userID, "tier", tier, "until", expires.Format(dayLayout))
	return expires, nil
}

func (r *SubRepo) ListActive() ([]User, error) {
	rows, err := r.db.Query(`
SELECT DISTINCT u.user_id, COALESCE(u.username, ''), COALESCE(u.first_name, ''),
       COALESCE(u.language_code, ''), u.registered_at, u.free_signals_used,
       COALESCE(u.free_signals_reset_date, ''), COALESCE(u.last_symbol, ''),
       COALESCE(u.timeframe, '4h')
FROM users u
JOIN subscriptions s ON s.user_id = u.user_id
WHERE s.status = 'active' AND s.expires_at > ?`,
		time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.FirstName, &u.LanguageCode,
			&u.RegisteredAt, &u.FreeSignalsUsed, &u.FreeSignalsResetDate,
			&u.LastSymbol, &u.Timeframe); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

type SubscriptionInfo struct {
	Tier      string
	ExpiresAt time.Time
}

func (r *SubRepo) GetActive(userID int64) (*SubscriptionInfo, error) {
	var tier string
	var expires int64
	err := r.db.QueryRow(`
SELECT tier, expires_at FROM subscriptions
WHERE user_id = ? AND status = 'active' AND expires_at > ?
ORDER BY expires_at DESC LIMIT 1`, userID, time.Now().Unix()).
		Scan(&tier, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &SubscriptionInfo{Tier: tier, ExpiresAt: time.Unix(expires, 0).UTC()}, nil
}
