package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Sora378/codingplantracker/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, err
	}

	return db, nil
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		plan_type TEXT DEFAULT 'starter',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS usage_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT,
		date DATE NOT NULL,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		api_calls INTEGER DEFAULT 0,
		session_minutes INTEGER DEFAULT 0,
		cost REAL DEFAULT 0,
		FOREIGN KEY (user_id) REFERENCES users(id),
		UNIQUE(user_id, date)
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT,
		start_time DATETIME NOT NULL,
		end_time DATETIME,
		duration_seconds INTEGER DEFAULT 0,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE INDEX IF NOT EXISTS idx_usage_date ON usage_records(date);
	CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

	CREATE TABLE IF NOT EXISTS usage_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT,
		window_used INTEGER DEFAULT 0,
		window_limit INTEGER DEFAULT 0,
		window_remaining INTEGER DEFAULT 0,
		window_percent REAL DEFAULT 0,
		weekly_used INTEGER DEFAULT 0,
		weekly_limit INTEGER DEFAULT 0,
		weekly_remaining INTEGER DEFAULT 0,
		weekly_percent REAL DEFAULT 0,
		window_end_unix_ms INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE INDEX IF NOT EXISTS idx_snapshots_user ON usage_snapshots(user_id);

	CREATE TABLE IF NOT EXISTS token_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id TEXT,
		date DATETIME NOT NULL,
		prompt_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		model_name TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE INDEX IF NOT EXISTS idx_tokens_user ON token_records(user_id);
	CREATE INDEX IF NOT EXISTS idx_tokens_date ON token_records(date);
	`
	_, err := db.conn.Exec(schema)
	return err
}

func (db *DB) UpsertUser(user *models.User) error {
	_, err := db.conn.Exec(`
		INSERT INTO users (id, email, plan_type, created_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			email = excluded.email,
			plan_type = excluded.plan_type
	`, user.ID, user.Email, user.PlanType, user.CreatedAt)
	return err
}

func (db *DB) GetUser(userID string) (*models.User, error) {
	row := db.conn.QueryRow(`
		SELECT id, email, plan_type, created_at FROM users WHERE id = ?
	`, userID)

	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.PlanType, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (db *DB) GetLatestUser() (*models.User, error) {
	row := db.conn.QueryRow(`
		SELECT id, email, plan_type, created_at FROM users ORDER BY created_at DESC LIMIT 1
	`)

	var u models.User
	err := row.Scan(&u.ID, &u.Email, &u.PlanType, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (db *DB) UpsertUsageRecord(userID string, date time.Time, rec *models.UsageRecord) error {
	_, err := db.conn.Exec(`
		INSERT INTO usage_records (user_id, date, input_tokens, output_tokens, api_calls, session_minutes, cost)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, date) DO UPDATE SET
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			api_calls = excluded.api_calls,
			session_minutes = excluded.session_minutes,
			cost = excluded.cost
	`, userID, date.Format("2006-01-02"), rec.InputTokens, rec.OutputTokens, rec.APICalls, rec.SessionMins, rec.Cost)
	return err
}

func (db *DB) GetUsageHistory(userID string, days int) ([]models.UsageRecord, error) {
	rows, err := db.conn.Query(`
		SELECT id, date, input_tokens, output_tokens, api_calls, session_minutes, cost
		FROM usage_records
		WHERE user_id = ? AND date >= date('now', ?)
		ORDER BY date DESC
	`, userID, "-"+fmt.Sprintf("%d", days)+" days")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.UsageRecord
	for rows.Next() {
		var r models.UsageRecord
		var dateStr string
		if err := rows.Scan(&r.ID, &dateStr, &r.InputTokens, &r.OutputTokens, &r.APICalls, &r.SessionMins, &r.Cost); err != nil {
			return nil, err
		}
		r.Date, _ = time.Parse("2006-01-02", dateStr)
		records = append(records, r)
	}
	return records, nil
}

func (db *DB) GetTodayUsage(userID string) (*models.UsageRecord, error) {
	row := db.conn.QueryRow(`
		SELECT id, date, input_tokens, output_tokens, api_calls, session_minutes, cost
		FROM usage_records
		WHERE user_id = ? AND date = date('now')
	`, userID)

	var r models.UsageRecord
	var dateStr string
	err := row.Scan(&r.ID, &dateStr, &r.InputTokens, &r.OutputTokens, &r.APICalls, &r.SessionMins, &r.Cost)
	if err != nil {
		return nil, err
	}
	r.Date, _ = time.Parse("2006-01-02", dateStr)
	return &r, nil
}

func (db *DB) LogUsageSnapshot(userID string, usage *models.CurrentUsage) error {
	_, err := db.conn.Exec(`
		INSERT INTO usage_snapshots (user_id, window_used, window_limit, window_remaining, window_percent,
			weekly_used, weekly_limit, weekly_remaining, weekly_percent, window_end_unix_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, usage.WindowUsed, usage.WindowLimit, usage.WindowRemaining, usage.WindowPercentUsed,
		usage.WeeklyUsed, usage.WeeklyLimit, usage.WeeklyRemaining, usage.WeeklyPercentUsed,
		usage.WindowEndUnixMs, time.Now())
	return err
}

func (db *DB) GetUsageSnapshots(userID string, limit int) ([]models.UsageSnapshot, error) {
	rows, err := db.conn.Query(`
		SELECT id, window_used, window_limit, window_remaining, window_percent,
			weekly_used, weekly_limit, weekly_remaining, weekly_percent, window_end_unix_ms, created_at
		FROM usage_snapshots
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []models.UsageSnapshot
	for rows.Next() {
		var s models.UsageSnapshot
		if err := rows.Scan(&s.ID, &s.WindowUsed, &s.WindowLimit, &s.WindowRemaining, &s.WindowPercent,
			&s.WeeklyUsed, &s.WeeklyLimit, &s.WeeklyRemaining, &s.WeeklyPercent,
			&s.WindowEndUnixMs, &s.CreatedAt); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, nil
}

// GetTokenStats calculates cumulative token usage from snapshots
func (db *DB) GetTokenStats(userID string) (*models.TokenStats, error) {
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	stats := &models.TokenStats{LastUpdated: now}

	// Get weekly used (most recent snapshot's weekly_used represents current week total)
	row := db.conn.QueryRow(`
		SELECT COALESCE(weekly_used, 0) FROM usage_snapshots
		WHERE user_id = ? AND created_at >= ?
		ORDER BY created_at DESC LIMIT 1
	`, userID, weekStart)
	row.Scan(&stats.WeeklyUsed)

	// Snapshot rows are polling samples, not events. Use max counters instead of summing
	// samples, otherwise totals explode as the app refreshes.
	row = db.conn.QueryRow(`
		SELECT COALESCE(MAX(weekly_used), 0) FROM usage_snapshots
		WHERE user_id = ? AND created_at >= ?
	`, userID, monthStart)
	row.Scan(&stats.MonthlyUsed)

	row = db.conn.QueryRow(`
		SELECT COALESCE(MAX(weekly_used), 0) FROM usage_snapshots WHERE user_id = ?
	`, userID)
	row.Scan(&stats.AllTimeUsed)

	// Get current window used from most recent snapshot
	row = db.conn.QueryRow(`
		SELECT COALESCE(window_used, 0) FROM usage_snapshots
		WHERE user_id = ? ORDER BY created_at DESC LIMIT 1
	`, userID)
	row.Scan(&stats.WindowUsed)

	return stats, nil
}

// LogTokenUsage records a single API call's token consumption
func (db *DB) LogTokenUsage(userID string, record *models.TokenRecord) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}
	if record.Date.IsZero() {
		record.Date = record.CreatedAt
	}
	_, err := db.conn.Exec(`
		INSERT INTO token_records (user_id, date, prompt_tokens, output_tokens, total_tokens, model_name, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, record.Date.Format("2006-01-02 15:04:05"), record.PromptTokens, record.OutputTokens, record.TotalTokens, record.ModelName, record.CreatedAt)
	return err
}

// GetTokenUsageSummary returns aggregated token statistics
func (db *DB) GetTokenUsageSummary(userID string) (*models.TokenUsageSummary, error) {
	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	summary := &models.TokenUsageSummary{LastUpdated: now}

	// Get weekly totals (last 7 days, roughly aligning with week)
	row := db.conn.QueryRow(`
		SELECT COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0)
		FROM token_records
		WHERE user_id = ? AND created_at >= ?
	`, userID, weekStart)
	row.Scan(&summary.WeekPrompt, &summary.WeekOutput, &summary.WeekTotal)

	// Get monthly totals
	row = db.conn.QueryRow(`
		SELECT COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0)
		FROM token_records
		WHERE user_id = ? AND created_at >= ?
	`, userID, monthStart)
	row.Scan(&summary.MonthPrompt, &summary.MonthOutput, &summary.MonthTotal)

	// Get all-time totals
	row = db.conn.QueryRow(`
		SELECT COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(total_tokens), 0)
		FROM token_records WHERE user_id = ?
	`, userID)
	row.Scan(&summary.AllPrompt, &summary.AllOutput, &summary.AllTotal)

	return summary, nil
}

// GetRecentTokenRecords returns the most recent token usage records
func (db *DB) GetRecentTokenRecords(userID string, limit int) ([]models.TokenRecord, error) {
	rows, err := db.conn.Query(`
		SELECT id, user_id, date, prompt_tokens, output_tokens, total_tokens, model_name, created_at
		FROM token_records
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []models.TokenRecord
	for rows.Next() {
		var r models.TokenRecord
		if err := rows.Scan(&r.ID, &r.UserID, &r.Date, &r.PromptTokens, &r.OutputTokens, &r.TotalTokens, &r.ModelName, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}
