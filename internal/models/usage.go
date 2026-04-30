package models

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	PlanType  string    `json:"plan_type"` // "starter", "plus", "max"
	CreatedAt time.Time `json:"created_at"`
}

type UsageRecord struct {
	ID           int64     `json:"id"`
	Date         time.Time `json:"date"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	APICalls     int       `json:"api_calls"`
	SessionMins  int       `json:"session_time_minutes"`
	Cost         float64   `json:"cost"`
}

type PlanInfo struct {
	Name         string `json:"name"`
	Prompts5Hour int    `json:"prompts_5hour"` // requests per 5-hour window
	WeeklyLimit  int    `json:"weekly_limit"`  // weekly request limit
}

var Plans = map[string]PlanInfo{
	"starter": {Name: "Starter", Prompts5Hour: 1500, WeeklyLimit: 15000},
	"plus":    {Name: "Plus", Prompts5Hour: 4500, WeeklyLimit: 45000},
	"max":     {Name: "Max", Prompts5Hour: 15000, WeeklyLimit: 150000},
}

// CodingPlanResponse is the actual API response from MiniMax
type CodingPlanResponse struct {
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
	ModelRemains []ModelRemain `json:"model_remains"`
}

type ModelRemain struct {
	StartTime                 int64  `json:"start_time"` // Unix timestamp in milliseconds
	EndTime                   int64  `json:"end_time"`
	RemainTime                int64  `json:"remains_time"`
	CurrentIntervalTotalCount int    `json:"current_interval_total_count"` // 5-hour window limit
	CurrentIntervalUsageCount int    `json:"current_interval_usage_count"` // used in window
	ModelName                 string `json:"model_name"`
	CurrentWeeklyTotalCount   int    `json:"current_weekly_total_count"` // weekly limit
	CurrentWeeklyUsageCount   int    `json:"current_weekly_usage_count"` // weekly used
}

type RequestUsage struct {
	Used      int `json:"used"`
	Remaining int `json:"remaining"`
	Limit     int `json:"limit"`
}

type CurrentUsage struct {
	Plan PlanInfo `json:"plan"`
	// 5-hour window usage
	WindowUsed      int    `json:"window_used"`
	WindowRemaining int    `json:"window_remaining"`
	WindowLimit     int    `json:"window_limit"`
	WindowStart     string `json:"window_start"`
	WindowEnd       string `json:"window_end"`
	WindowEndUnixMs int64  `json:"window_end_unix_ms"`
	// Weekly usage
	WeeklyUsed      int `json:"weekly_used"`
	WeeklyRemaining int `json:"weekly_remaining"`
	WeeklyLimit     int `json:"weekly_limit"`
	// Calculated
	WindowPercentUsed float64   `json:"window_percent_used"`
	WeeklyPercentUsed float64   `json:"weekly_percent_used"`
	LastUpdated       time.Time `json:"last_updated"`
}

type UsageSnapshot struct {
	ID              int64     `json:"id"`
	WindowUsed      int       `json:"window_used"`
	WindowLimit     int       `json:"window_limit"`
	WindowRemaining int       `json:"window_remaining"`
	WindowPercent   float64   `json:"window_percent"`
	WeeklyUsed      int       `json:"weekly_used"`
	WeeklyLimit     int       `json:"weekly_limit"`
	WeeklyRemaining int       `json:"weekly_remaining"`
	WeeklyPercent   float64   `json:"weekly_percent"`
	WindowEndUnixMs int64     `json:"window_end_unix_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

// TokenStats holds cumulative token/request usage statistics
type TokenStats struct {
	// 5-hour window
	WindowUsed int `json:"window_used"`
	// Weekly (current week)
	WeeklyUsed int `json:"weekly_used"`
	// Monthly (current month)
	MonthlyUsed int `json:"monthly_used"`
	// All-time total
	AllTimeUsed int `json:"all_time_used"`
	// Last updated
	LastUpdated time.Time `json:"last_updated"`
}

// TokenRecord represents a single API call's token consumption
type TokenRecord struct {
	ID           int64     `json:"id"`
	UserID       string    `json:"user_id"`
	Date         time.Time `json:"date"`
	PromptTokens int64     `json:"prompt_tokens"` // input tokens
	OutputTokens int64     `json:"output_tokens"` // output tokens
	TotalTokens  int64     `json:"total_tokens"`  // total tokens
	ModelName    string    `json:"model_name"`    // model used
	CreatedAt    time.Time `json:"created_at"`
}

// TokenUsageSummary holds aggregated token statistics
type TokenUsageSummary struct {
	PromptTokens int64     `json:"prompt_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
	WindowPrompt int64     `json:"window_prompt"`
	WindowOutput int64     `json:"window_output"`
	WindowTotal  int64     `json:"window_total"`
	WeekPrompt   int64     `json:"week_prompt"`
	WeekOutput   int64     `json:"week_output"`
	WeekTotal    int64     `json:"week_total"`
	MonthPrompt  int64     `json:"month_prompt"`
	MonthOutput  int64     `json:"month_output"`
	MonthTotal   int64     `json:"month_total"`
	AllPrompt    int64     `json:"all_prompt"`
	AllOutput    int64     `json:"all_output"`
	AllTotal     int64     `json:"all_total"`
	LastUpdated  time.Time `json:"last_updated"`
}
