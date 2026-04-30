package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Sora378/codingplantracker/internal/config"
	"github.com/Sora378/codingplantracker/internal/models"
)

type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		baseURL: cfg.APIEndpoint(),
		token:   cfg.GetAccessToken(),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) GetCurrentUsage(ctx context.Context, cfg *config.Config) (*models.CurrentUsage, error) {
	endpoint := c.baseURL + "/v1/api/openplatform/coding_plan/remains"

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("unauthorized: please re-login or check your API key")
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("API endpoint not found. Make sure you're using a Coding Plan API key")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage API failed: status %d", resp.StatusCode)
	}

	// Parse the actual API response
	var apiResp models.CodingPlanResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode usage: %w", err)
	}

	if apiResp.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("API error: %s (code %d)", apiResp.BaseResp.StatusMsg, apiResp.BaseResp.StatusCode)
	}

	// Find M2.7 entry from the array
	var m2_7Data *models.ModelRemain
	for i := range apiResp.ModelRemains {
		if apiResp.ModelRemains[i].ModelName == "MiniMax-M*" {
			m2_7Data = &apiResp.ModelRemains[i]
			break
		}
	}

	if m2_7Data == nil && len(apiResp.ModelRemains) > 0 {
		// Fallback to first entry
		m2_7Data = &apiResp.ModelRemains[0]
	}

	if m2_7Data == nil {
		return nil, fmt.Errorf("no usage data found")
	}

	// Convert Unix timestamps (milliseconds) to readable time
	windowStart := time.UnixMilli(m2_7Data.StartTime).Format("2006-01-02 15:04")
	windowEnd := time.UnixMilli(m2_7Data.EndTime).Format("2006-01-02 15:04")

	// API field names are misleading:
	// current_interval_usage_count = remaining requests
	// remains_time = remaining time in ms
	windowUsed := m2_7Data.CurrentIntervalTotalCount - m2_7Data.CurrentIntervalUsageCount
	windowRemaining := m2_7Data.CurrentIntervalUsageCount
	windowLimit := m2_7Data.CurrentIntervalTotalCount

	windowPercent := 0.0
	if windowLimit > 0 {
		windowPercent = float64(windowUsed) / float64(windowLimit) * 100
	}

	weeklyUsed := m2_7Data.CurrentWeeklyTotalCount - m2_7Data.CurrentWeeklyUsageCount
	weeklyRemaining := m2_7Data.CurrentWeeklyUsageCount
	weeklyLimit := m2_7Data.CurrentWeeklyTotalCount

	weeklyPercent := 0.0
	if weeklyLimit > 0 {
		weeklyPercent = float64(weeklyUsed) / float64(weeklyLimit) * 100
	}

	return &models.CurrentUsage{
		Plan: models.PlanInfo{
			Name: "M2.7 (Token Plan)",
		},
		WindowUsed:        windowUsed,
		WindowRemaining:   windowRemaining,
		WindowLimit:       windowLimit,
		WindowStart:       windowStart,
		WindowEnd:         windowEnd,
		WindowEndUnixMs:   m2_7Data.EndTime,
		WeeklyUsed:        weeklyUsed,
		WeeklyRemaining:   weeklyRemaining,
		WeeklyLimit:       weeklyLimit,
		WindowPercentUsed: windowPercent,
		WeeklyPercentUsed: weeklyPercent,
		LastUpdated:       time.Now(),
	}, nil
}

func (c *Client) GetAccountInfo(ctx context.Context) (*models.User, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/account", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("account API failed: status %d", resp.StatusCode)
	}

	var account struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		PlanType string `json:"plan_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, err
	}

	return &models.User{
		ID:        account.ID,
		Email:     account.Email,
		PlanType:  account.PlanType,
		CreatedAt: time.Now(),
	}, nil
}
