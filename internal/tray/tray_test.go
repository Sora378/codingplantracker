package tray

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Sora378/codingplantracker/internal/codex"
)

func TestUsedPercentClampsCodexUsedPercent(t *testing.T) {
	tests := []struct {
		name string
		used float64
		want float64
	}{
		{name: "normal", used: 37.5, want: 37.5},
		{name: "overused", used: 125, want: 100},
		{name: "negative", used: -10, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usedPercent(tt.used); got != tt.want {
				t.Fatalf("usedPercent(%v) = %v, want %v", tt.used, got, tt.want)
			}
		})
	}
}

func TestFormatCodexWindowShowsUsedPercent(t *testing.T) {
	window := &codex.Window{UsedPercent: 37.5}
	got := formatCodexWindow("Codex 5H", window)
	want := "Codex 5H used: 37.5%"
	if got != want {
		t.Fatalf("formatCodexWindow() = %q, want %q", got, want)
	}
}

func TestSplitCodexWindowsUsesDuration(t *testing.T) {
	fiveHours := 300
	week := 10080
	primary := &codex.Window{UsedPercent: 25, WindowDurationMins: &week}
	secondary := &codex.Window{UsedPercent: 40, WindowDurationMins: &fiveHours}

	got5H, gotWeek := splitCodexWindows(primary, secondary)
	if got5H != secondary {
		t.Fatalf("5H window = %#v, want secondary", got5H)
	}
	if gotWeek != primary {
		t.Fatalf("weekly window = %#v, want primary", gotWeek)
	}
}

func TestValidateLoginSubmission(t *testing.T) {
	form := url.Values{}
	form.Set("nonce", "token")
	form.Set("apiKey", "  secret-key  ")
	req := httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	got, err := validateLoginSubmission(req, "token")
	if err != nil {
		t.Fatalf("validateLoginSubmission() error = %v", err)
	}
	if got != "secret-key" {
		t.Fatalf("api key = %q, want trimmed secret-key", got)
	}
}

func TestValidateLoginSubmissionRejectsInvalidNonceAndMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/submit", nil)
	if _, err := validateLoginSubmission(req, "token"); !errors.Is(err, errInvalidLoginMethod) {
		t.Fatalf("GET error = %v, want errInvalidLoginMethod", err)
	}

	form := url.Values{}
	form.Set("nonce", "wrong")
	form.Set("apiKey", "secret-key")
	req = httptest.NewRequest(http.MethodPost, "/submit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := validateLoginSubmission(req, "token"); !errors.Is(err, errInvalidLoginNonce) {
		t.Fatalf("bad nonce error = %v, want errInvalidLoginNonce", err)
	}
}
