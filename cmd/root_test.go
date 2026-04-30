package cmd

import (
	"bufio"
	"strings"
	"testing"
)

func TestReadLimitedLineRejectsOversizedInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("abcdef\n"))
	if _, err := readLimitedLine(reader, 3); err == nil {
		t.Fatalf("readLimitedLine() error = nil, want oversized input error")
	}
}

func TestReadLimitedLineTrimsNewlineOnly(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("secret\nleftover"))
	got, err := readLimitedLine(reader, 64)
	if err != nil {
		t.Fatalf("readLimitedLine() error = %v", err)
	}
	if got != "secret" {
		t.Fatalf("line = %q, want secret", got)
	}
}
