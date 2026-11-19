package quota

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"regexp"
	"time"
)

// Credit amounts are minor currency units (cents for USD), as supplied by
// /api/oauth/usage. Keep fractional cents until display; never estimate from tokens.
type ExtraUsage struct {
	Enabled          bool         `json:"enabled"`
	UsedCredits      *float64     `json:"usedCredits"`
	MonthlyLimit     *float64     `json:"monthlyLimit"`
	Utilization      *float64     `json:"utilization"`
	Currency         string       `json:"currency"`
	HistoryAvailable bool         `json:"historyAvailable"`
	Daily            []DailySpend `json:"daily"`
}

type DailySpend struct {
	Date    string  `json:"date"`
	Spent   float64 `json:"spent"`
	Partial bool    `json:"partial"`
}

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func normalizeExtraUsage(body []byte) *ExtraUsage {
	var response struct {
		Extra *struct {
			Enabled     bool     `json:"is_enabled"`
			Used        *float64 `json:"used_credits"`
			Limit       *float64 `json:"monthly_limit"`
			Utilization *float64 `json:"utilization"`
			Currency    string   `json:"currency"`
		} `json:"extra_usage"`
	}
	if json.Unmarshal(body, &response) != nil || response.Extra == nil {
		return nil
	}
	e := response.Extra
	if e.Used != nil && *e.Used < 0 {
		e.Used = nil
	}
	if e.Limit != nil && *e.Limit < 0 {
		e.Limit = nil
	}
	if !currencyCode.MatchString(e.Currency) {
		e.Currency = "USD"
	}
	return &ExtraUsage{Enabled: e.Enabled, UsedCredits: e.Used, MonthlyLimit: e.Limit, Utilization: e.Utilization, Currency: e.Currency}
}

// Account identity survives access-token refreshes. Hash it before persistence;
// never combine different accounts or store credentials in the history database.
func (s *Service) accountID() string {
	raw, err := os.ReadFile(s.accountPath)
	if err != nil {
		return ""
	}
	var config struct {
		Account struct {
			ID           string `json:"accountUuid"`
			Organization string `json:"organizationUuid"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(raw, &config) != nil || config.Account.ID == "" {
		return ""
	}
	return sha256Hex(config.Account.ID + ":" + config.Account.Organization)
}

// Run continues sampling when the browser is closed. Shutdown follows the server
// context. Get shares its TTL, backoff and in-flight guards with browser requests.
func (s *Service) Run(ctx context.Context) {
	s.Get(ctx, false)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Get(ctx, false)
		}
	}
}

func (s *Service) recordExtraUsage(ctx context.Context, account string, extra *ExtraUsage, now time.Time) ([]DailySpend, error) {
	day := now.UTC().Format("2006-01-02")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// The daily ledger is independent of the latest monthly balance. A null
	// balance skips recording a sample but still returns existing history.
	if extra.UsedCredits != nil {
		total := *extra.UsedCredits
		var previousDay string
		var previousTotal, spent float64
		var partial bool
		err = tx.QueryRowContext(ctx, `SELECT day, total, spent, partial FROM quota_extra_usage_daily WHERE account = ? AND currency = ? ORDER BY day DESC LIMIT 1`, account, extra.Currency).Scan(&previousDay, &previousTotal, &spent, &partial)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if previousDay <= day {
			if previousDay != day {
				spent = 0
				partial = false
			}
			switch {
			case previousDay == "":
				// The existing monthly balance predates tracking, so it is not today's spend.
				partial = true
			case previousDay[:7] != day[:7]:
				spent += total
				partial = day[8:] != "01" || now.UTC().Sub(parseDay(previousDay)) > 48*time.Hour
			default:
				if total > previousTotal {
					spent += total - previousTotal
				}
				// Retain the month's high-water mark on corrections to avoid double counting.
				if total < previousTotal {
					total = previousTotal
					partial = true
				}
				if previousDay != day && parseDay(day).Sub(parseDay(previousDay)) > 24*time.Hour {
					partial = true
				}
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO quota_extra_usage_daily (account, currency, day, total, spent, partial) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(account, currency, day) DO UPDATE SET total=excluded.total, spent=excluded.spent, partial=excluded.partial`, account, extra.Currency, day, total, spent, partial)
			if err != nil {
				return nil, err
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT day, spent, partial FROM quota_extra_usage_daily WHERE account = ? AND currency = ? ORDER BY day DESC LIMIT 5`, account, extra.Currency)
	if err != nil {
		return nil, err
	}
	history := make([]DailySpend, 0)
	for rows.Next() {
		var d DailySpend
		if err := rows.Scan(&d.Date, &d.Spent, &d.Partial); err != nil {
			rows.Close()
			return nil, err
		}
		history = append(history, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return history, nil
}

func parseDay(day string) time.Time {
	t, _ := time.Parse("2006-01-02", day)
	return t
}
