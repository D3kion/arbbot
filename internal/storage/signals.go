package storage

import (
	"database/sql"
	"time"
)

type Signal struct {
	ID           int64
	UserID       int64
	Symbol       string
	Signal       string
	Confidence   int
	PriceAtTime  float64
	TargetPrice  *float64
	StopLoss     *float64
	Outcome      string
	FeedbackAction string
	CreatedAt    time.Time
}

type SignalRepo struct {
	db *sql.DB
}

func NewSignalRepo(db *sql.DB) *SignalRepo { return &SignalRepo{db: db} }

func (r *SignalRepo) Create(s *Signal) error {
	now := time.Now().Unix()
	res, err := r.db.Exec(`
INSERT INTO signal_logs (user_id, symbol, signal, confidence, price_at_time, target_price, stop_loss, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.UserID, s.Symbol, s.Signal, s.Confidence, s.PriceAtTime,
		nullableFloat(s.TargetPrice), nullableFloat(s.StopLoss), now)
	if err != nil {
		return err
	}
	s.ID, err = res.LastInsertId()
	if err == nil {
		s.CreatedAt = time.Unix(now, 0)
	}
	return err
}

func nullableFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

// PendingOutcome returns signals older than minAge with target_price set but no outcome yet.
func (r *SignalRepo) PendingOutcome(minAge time.Duration) ([]Signal, error) {
	cutoff := time.Now().Add(-minAge).Unix()
	rows, err := r.db.Query(`
SELECT id, user_id, symbol, signal, confidence, price_at_time, target_price, stop_loss, created_at
FROM signal_logs
WHERE outcome IS NULL AND target_price IS NOT NULL AND created_at < ?`,
		cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Signal
	for rows.Next() {
		var s Signal
		var createdAt int64
		var tp, sl sql.NullFloat64
		if err := rows.Scan(&s.ID, &s.UserID, &s.Symbol, &s.Signal, &s.Confidence,
			&s.PriceAtTime, &tp, &sl, &createdAt); err != nil {
			return nil, err
		}
		if tp.Valid {
			v := tp.Float64
			s.TargetPrice = &v
		}
		if sl.Valid {
			v := sl.Float64
			s.StopLoss = &v
		}
		s.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetOutcome records win/loss/pending outcome for a signal.
func (r *SignalRepo) SetOutcome(id int64, outcome string) error {
	_, err := r.db.Exec(`UPDATE signal_logs SET outcome = ? WHERE id = ?`, outcome, id)
	return err
}

// SetFeedback records user feedback action (entered/not_entered).
func (r *SignalRepo) SetFeedback(id int64, action string) error {
	_, err := r.db.Exec(`UPDATE signal_logs SET feedback_action = ? WHERE id = ?`, action, id)
	return err
}

type Stats struct {
	Total      int
	Buy        int
	Sell       int
	Hold       int
	AvgConf    float64
}

func (r *SignalRepo) StatsByUser(userID int64) (*Stats, error) {
	s := &Stats{}
	err := r.db.QueryRow(`
SELECT COUNT(*),
       COALESCE(SUM(signal = 'BUY'), 0),
       COALESCE(SUM(signal = 'SELL'), 0),
       COALESCE(SUM(signal = 'HOLD'), 0),
       COALESCE(AVG(confidence), 0)
FROM signal_logs WHERE user_id = ?`, userID).
		Scan(&s.Total, &s.Buy, &s.Sell, &s.Hold, &s.AvgConf)
	if err != nil {
		return nil, err
	}
	return s, nil
}

type FeedbackStats struct {
	Total      int
	Entered    int
	NotEntered int
	Pending    int
}

func (r *SignalRepo) FeedbackStats() (*FeedbackStats, error) {
	s := &FeedbackStats{}
	err := r.db.QueryRow(`
SELECT COUNT(*),
       COALESCE(SUM(feedback_action = 'entered'), 0),
       COALESCE(SUM(feedback_action = 'not_entered'), 0),
       COALESCE(SUM(feedback_action IS NULL OR feedback_action = ''), 0)
FROM signal_logs`).
		Scan(&s.Total, &s.Entered, &s.NotEntered, &s.Pending)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// OutcomeStats returns win/loss/pending counts for signals with outcomes.
func (r *SignalRepo) OutcomeStats() (total int, wins int, losses int, pending int, err error) {
	err = r.db.QueryRow(`
SELECT COUNT(*),
       COALESCE(SUM(outcome = 'win'), 0),
       COALESCE(SUM(outcome = 'loss'), 0),
       COALESCE(SUM(outcome IS NULL), 0)
FROM signal_logs WHERE target_price IS NOT NULL`).
		Scan(&total, &wins, &losses, &pending)
	return
}

// RecentByUser returns the last n signals for a user, optionally filtered by symbol.
func (r *SignalRepo) RecentByUser(userID int64, symbol string, limit int) ([]Signal, error) {
	rows, err := r.db.Query(`
SELECT id, user_id, symbol, signal, confidence, price_at_time, target_price, stop_loss,
       COALESCE(outcome, ''), COALESCE(feedback_action, ''), created_at
FROM signal_logs WHERE user_id = ? AND (? = '' OR symbol = ?)
ORDER BY created_at DESC LIMIT ?`, userID, symbol, symbol, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Signal
	for rows.Next() {
		var s Signal
		var createdAt int64
		var tp, sl sql.NullFloat64
		if err := rows.Scan(&s.ID, &s.UserID, &s.Symbol, &s.Signal, &s.Confidence,
			&s.PriceAtTime, &tp, &sl,
			&s.Outcome, &s.FeedbackAction, &createdAt); err != nil {
			return nil, err
		}
		if tp.Valid {
			v := tp.Float64
			s.TargetPrice = &v
		}
		if sl.Valid {
			v := sl.Float64
			s.StopLoss = &v
		}
		s.CreatedAt = time.Unix(createdAt, 0)
		out = append(out, s)
	}
	return out, rows.Err()
}
