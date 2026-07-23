package orchestrator

import (
	"testing"
	"time"
)

func TestParseDateQuery(t *testing.T) {
	// Base date: Thursday, 2026-07-23
	baseTime := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		input    string
		expected string
		parsed   bool
	}{
		// ISO date formats
		{"2026-06-24", "2026-06-24", true},
		{"24-06-2026", "2026-06-24", true},
		{"24/06/2026", "2026-06-24", true},
		{"2026/06/24", "2026-06-24", true},

		// Relative days
		{"avui", "2026-07-23", true},
		{"demà", "2026-07-24", true},
		{"dema", "2026-07-24", true},
		{"ahir", "2026-07-22", true},
		{"demà passat", "2026-07-25", true},

		// Weekdays (Reference date Thursday 23rd)
		{"dilluns", "2026-07-27", true}, // Next Monday
		{"dimarts", "2026-07-28", true}, // Next Tuesday
		{"el proper dimarts", "2026-07-28", true},
		{"dimecres", "2026-07-29", true},
		{"dijous", "2026-07-30", true},   // Next Thursday (+7)
		{"divendres", "2026-07-24", true}, // Next Friday (+1)
		{"dissabte", "2026-07-25", true},  // Next Saturday (+2)
		{"diumenge", "2026-07-26", true},  // Next Sunday (+3)

		// Invalid cases
		{"inventat", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		got, parsed := parseDateQuery(tc.input, baseTime)
		if parsed != tc.parsed {
			t.Errorf("parseDateQuery(%q) parsed = %v; want %v", tc.input, parsed, tc.parsed)
		}
		if parsed {
			gotStr := got.Format("2006-01-02")
			if gotStr != tc.expected {
				t.Errorf("parseDateQuery(%q) = %s; want %s", tc.input, gotStr, tc.expected)
			}
		}
	}
}
