package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Sora378/codingplantracker/internal/models"
)

func TestGetTokenStatsDoesNotSumPollingSnapshots(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer database.Close()

	user := &models.User{ID: "u1", Email: "u1@example.com", PlanType: "starter", CreatedAt: time.Now()}
	if err := database.UpsertUser(user); err != nil {
		t.Fatalf("UpsertUser() error = %v", err)
	}

	usage := &models.CurrentUsage{WindowUsed: 10, WindowLimit: 100, WindowRemaining: 90, WeeklyUsed: 50, WeeklyLimit: 1000, WeeklyRemaining: 950}
	for i := 0; i < 3; i++ {
		if err := database.LogUsageSnapshot(user.ID, usage); err != nil {
			t.Fatalf("LogUsageSnapshot() error = %v", err)
		}
	}

	stats, err := database.GetTokenStats(user.ID)
	if err != nil {
		t.Fatalf("GetTokenStats() error = %v", err)
	}
	if stats.AllTimeUsed != 50 || stats.MonthlyUsed != 50 {
		t.Fatalf("stats should use max counters, not sum polling samples: %+v", stats)
	}
}

func TestLogTokenUsageSetsCreatedAtWhenMissing(t *testing.T) {
	database, err := New(filepath.Join(t.TempDir(), "usage.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer database.Close()

	record := &models.TokenRecord{
		PromptTokens: 10,
		OutputTokens: 5,
		TotalTokens:  15,
		ModelName:    "test-model",
	}
	if err := database.LogTokenUsage("u1", record); err != nil {
		t.Fatalf("LogTokenUsage() error = %v", err)
	}

	records, err := database.GetRecentTokenRecords("u1", 1)
	if err != nil {
		t.Fatalf("GetRecentTokenRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].CreatedAt.IsZero() {
		t.Fatalf("CreatedAt should be set")
	}
	if records[0].Date.IsZero() {
		t.Fatalf("Date should be set")
	}
}
