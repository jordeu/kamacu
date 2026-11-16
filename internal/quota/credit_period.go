package quota

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// CreditPeriod tracks a continuous interval in which at least one subscription
// window is exhausted. Missing history is represented explicitly, never as $0
// since an unobserved limit crossing. Amounts use the same minor units as usage.
type CreditPeriod struct {
	SpentCredits  float64   `json:"spentCredits"`
	StartedAt     time.Time `json:"startedAt"`
	BaselineKnown bool      `json:"baselineKnown"`
}

type creditSnapshot struct {
	At        time.Time         `json:"at"`
	Total     float64           `json:"total"`
	Exhausted map[string]string `json:"exhausted"` // window key -> reset boundary
	Period    *CreditPeriod     `json:"period"`
}

// recordCreditPeriod shares the daily-history transaction, so a restart cannot
// replay an increase in one counter without replaying the other.
func recordCreditPeriod(ctx context.Context, tx *sql.Tx, account string, extra *ExtraUsage, windows []Window, now time.Time) (*CreditPeriod, error) {
	if len(windows) == 0 {
		return nil, nil
	} // unknown quota is not a replenishment
	var previous creditSnapshot
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT snapshot FROM quota_credit_tracking WHERE account = ? AND currency = ?`, account, extra.Currency).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &previous); err != nil {
			return nil, err
		}
	}
	if now.Before(previous.At) {
		return previous.Period, nil
	}
	next := creditSnapshot{At: now.UTC(), Total: *extra.UsedCredits, Exhausted: map[string]string{}}
	for _, w := range windows {
		if w.Utilization < 100 {
			continue
		}
		reset := ""
		if w.ResetsAt != nil {
			reset = *w.ResetsAt
			// An expired window cannot establish a current paid-usage period.
			if boundary, err := time.Parse(time.RFC3339, reset); err == nil {
				if !boundary.After(now) {
					continue
				}
				reset = boundary.UTC().Format(time.RFC3339)
			} else {
				reset = "" // invalid boundaries cannot prove continuity across a gap
			}
		}
		next.Exhausted[w.Key] = reset
	}
	sameMonth := !previous.At.IsZero() && previous.At.UTC().Format("2006-01") == now.UTC().Format("2006-01")
	delta := 0.0
	if sameMonth {
		delta = max(0, next.Total-previous.Total)
		next.Total = max(next.Total, previous.Total) // counter corrections must not be counted twice
	}
	recent := !previous.At.IsZero() && now.Sub(previous.At) <= 2*cacheTTL
	continuous := false
	for key, reset := range next.Exhausted {
		if oldReset, ok := previous.Exhausted[key]; ok && oldReset == reset && (reset != "" || recent) {
			continuous = true
		}
	}
	if extra.Enabled && len(next.Exhausted) > 0 {
		if continuous && previous.Period != nil {
			period := *previous.Period
			if !sameMonth {
				// The old month's unsampled tail cannot be reconstructed from the new total.
				delta = next.Total
				period.BaselineKnown = false
			}
			if *extra.UsedCredits < previous.Total && sameMonth {
				period.BaselineKnown = false
			}
			period.SpentCredits += delta
			next.Period = &period
		} else {
			known := recent && sameMonth && *extra.UsedCredits >= previous.Total && previous.Exhausted != nil && len(previous.Exhausted) == 0 && previous.Period == nil
			spent := 0.0
			if known {
				spent = delta
			}
			next.Total = *extra.UsedCredits // a new period starts from its own observed balance
			next.Period = &CreditPeriod{SpentCredits: spent, StartedAt: now.UTC(), BaselineKnown: known}
		}
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO quota_credit_tracking (account, currency, snapshot) VALUES (?, ?, ?) ON CONFLICT(account, currency) DO UPDATE SET snapshot=excluded.snapshot`, account, extra.Currency, string(encoded))
	return next.Period, err
}
