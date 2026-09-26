package cron

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	base := time.Date(2026, 9, 26, 10, 7, 30, 0, time.UTC) // a Saturday
	cases := []struct {
		expr string
		want string
	}{
		{"* * * * *", "2026-09-26T10:08:00Z"},
		{"*/15 * * * *", "2026-09-26T10:15:00Z"},
		{"0 3 * * *", "2026-09-27T03:00:00Z"},
		{"@hourly", "2026-09-26T11:00:00Z"},
		{"0 9 * * 1-5", "2026-09-28T09:00:00Z"},
		{"30 6 1 * *", "2026-10-01T06:30:00Z"},
		{"0 0 * * sun", "2026-09-27T00:00:00Z"},
		{"0 12 1 jan *", "2027-01-01T12:00:00Z"},
	}
	for _, c := range cases {
		s, err := Parse(c.expr)
		if err != nil {
			t.Fatalf("%s: %v", c.expr, err)
		}
		if got := s.Next(base).Format(time.RFC3339); got != c.want {
			t.Errorf("%s: got %s want %s", c.expr, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{"", "* * * *", "61 * * * *", "* 25 * * *", "*/0 * * * *", "a b c d e"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestDescribe(t *testing.T) {
	for expr, want := range map[string]string{"": "Manual only", "*/30 * * * *": "Every 30 minutes", "0 6 * * *": "Every day at 06:00 UTC", "0 9 * * 1-5": "Weekdays at 09:00 UTC"} {
		if got := Describe(expr); got != want {
			t.Errorf("%q: got %q want %q", expr, got, want)
		}
	}
}
