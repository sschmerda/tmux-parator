package app

import "testing"

func TestShellQuote(t *testing.T) {
	tests := map[string]string{
		"":            "''",
		"simple":      "'simple'",
		"has space":   "'has space'",
		"has'quote":   "'has'\\''quote'",
		"/tmp/config": "'/tmp/config'",
	}
	for input, want := range tests {
		if got := shellQuote(input); got != want {
			t.Fatalf("shellQuote(%q) = %q, want %q", input, got, want)
		}
	}
}
