package storage

import (
	"database/sql"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

type User struct {
	ID                   int64
	Username             string
	FirstName            string
	LanguageCode         string
	RegisteredAt         int64
	FreeSignalsUsed      int
	FreeSignalsResetDate string
	LastSymbol           string
	Timeframe            string
}

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db: db} }

const dayLayout = "2006-01-02"

func (r *UserRepo) CreateOrUpdate(u *User) error {
	_, err := r.db.Exec(`
INSERT INTO users (user_id, username, first_name, language_code, registered_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(user_id) DO UPDATE SET
    username = excluded.username,
    first_name = excluded.first_name,
    language_code = excluded.language_code`,
		u.ID, u.Username, u.FirstName, u.LanguageCode, time.Now().Unix())
	return err
}

func (r *UserRepo) GetByID(id int64) (*User, error) {
	u := &User{}
	err := r.db.QueryRow(`
SELECT user_id, COALESCE(username, ''), COALESCE(first_name, ''), COALESCE(language_code, ''),
       registered_at, free_signals_used, COALESCE(free_signals_reset_date, ''),
       COALESCE(last_symbol, ''), COALESCE(timeframe, '4h')
FROM users WHERE user_id = ?`, id).
		Scan(&u.ID, &u.Username, &u.FirstName, &u.LanguageCode,
			&u.RegisteredAt, &u.FreeSignalsUsed, &u.FreeSignalsResetDate,
			&u.LastSymbol, &u.Timeframe)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) GetFresh(id int64) (*User, error) {
	today := time.Now().UTC().Format(dayLayout)
	// atomic daily reset — single UPDATE covers race, then re-read
	if _, err := r.db.Exec(
		`UPDATE users SET free_signals_used = 0, free_signals_reset_date = ? WHERE user_id = ? AND (free_signals_reset_date IS NULL OR free_signals_reset_date != ?)`,
		today, id, today); err != nil {
		return nil, err
	}
	return r.GetByID(id)
}

func (r *UserRepo) IncrementFreeSignals(id int64) error {
	_, err := r.db.Exec(`UPDATE users SET free_signals_used = free_signals_used + 1 WHERE user_id = ?`, id)
	return err
}

// TryConsumeFreeSignal atomically increments if limit not reached. Returns true if consumed.
func (r *UserRepo) TryConsumeFreeSignal(id int64, limit int) (bool, error) {
	// ensure daily reset first
	if _, err := r.GetFresh(id); err != nil {
		return false, err
	}
	res, err := r.db.Exec(`UPDATE users SET free_signals_used = free_signals_used + 1 WHERE user_id = ? AND free_signals_used < ?`, id, limit)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *UserRepo) SetLastSymbol(id int64, symbol string) error {
	_, err := r.db.Exec(`UPDATE users SET last_symbol = ? WHERE user_id = ?`, symbol, id)
	return err
}

func (r *UserRepo) SetTimeframe(id int64, tf string) error {
	_, err := r.db.Exec(`UPDATE users SET timeframe = ? WHERE user_id = ?`, tf, id)
	return err
}
