package api

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Sora378/codingplantracker/internal/config"
)

func TestGetCurrentUsageDoesNotLeakErrorBody(t *testing.T) {
	client := &Client{
		baseURL: "https://example.test",
		token:   "test",
		client:  testHTTPClient(t, http.StatusInternalServerError, "secret-api-detail"),
	}
	_, err := client.GetCurrentUsage(context.Background(), config.DefaultConfig())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-api-detail") {
		t.Fatalf("error leaked response body: %v", err)
	}
}

func TestGetCurrentUsageParsesCodingPlanResponse(t *testing.T) {
	start := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2026, 4, 28, 15, 0, 0, 0, time.UTC).UnixMilli()

	client := &Client{
		baseURL: "https://example.test",
		token:   "test-token",
		client: testHTTPClient(t, http.StatusOK, `{
			"base_resp": {"status_code": 0, "status_msg": "ok"},
			"model_remains": [{
				"model_name": "MiniMax-M*",
				"start_time": `+int64String(start)+`,
				"end_time": `+int64String(end)+`,
				"current_interval_total_count": 100,
				"current_interval_usage_count": 25,
				"current_weekly_total_count": 1000,
				"current_weekly_usage_count": 400
			}]
		}`),
	}
	usage, err := client.GetCurrentUsage(context.Background(), config.DefaultConfig())
	if err != nil {
		t.Fatalf("GetCurrentUsage() error = %v", err)
	}

	if usage.WindowUsed != 75 || usage.WindowRemaining != 25 || usage.WindowLimit != 100 {
		t.Fatalf("window usage mismatch: %+v", usage)
	}
	if usage.WeeklyUsed != 600 || usage.WeeklyRemaining != 400 || usage.WeeklyLimit != 1000 {
		t.Fatalf("weekly usage mismatch: %+v", usage)
	}
}

func int64String(v int64) string {
	return strconv.FormatInt(v, 10)
}

func testHTTPClient(t *testing.T, status int, body string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if status == http.StatusOK {
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("authorization header = %q", got)
			}
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    r,
		}, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
