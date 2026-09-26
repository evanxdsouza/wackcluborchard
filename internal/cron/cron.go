// Package cron parses standard five-field cron expressions (minute hour
// day-of-month month day-of-week) plus the @hourly/@daily/... shorthands,
// and computes the next fire time.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Schedule struct {
	min, hour, dom, month, dow uint64
	domStar, dowStar           bool
	src                        string
}

var shorthands = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

var monthNames = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
var dayNames = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

func Parse(expr string) (*Schedule, error) {
	src := strings.TrimSpace(expr)
	if s, ok := shorthands[src]; ok {
		expr = s
	}
	f := strings.Fields(expr)
	if len(f) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d", len(f))
	}
	s := &Schedule{src: src}
	var err error
	if s.min, err = field(f[0], 0, 59, nil); err != nil {
		return nil, fmt.Errorf("cron minute: %w", err)
	}
	if s.hour, err = field(f[1], 0, 23, nil); err != nil {
		return nil, fmt.Errorf("cron hour: %w", err)
	}
	if s.dom, err = field(f[2], 1, 31, nil); err != nil {
		return nil, fmt.Errorf("cron day-of-month: %w", err)
	}
	if s.month, err = field(f[3], 1, 12, monthNames); err != nil {
		return nil, fmt.Errorf("cron month: %w", err)
	}
	if s.dow, err = field(f[4], 0, 7, dayNames); err != nil {
		return nil, fmt.Errorf("cron day-of-week: %w", err)
	}
	if s.dow&(1<<7) != 0 {
		s.dow |= 1
	}
	s.domStar = f[2] == "*" || f[2] == "?"
	s.dowStar = f[4] == "*" || f[4] == "?"
	return s, nil
}

func field(f string, lo, hi int, names map[string]int) (uint64, error) {
	var bits uint64
	for _, part := range strings.Split(f, ",") {
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			n, err := strconv.Atoi(part[i+1:])
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("bad step in %q", part)
			}
			step = n
			part = part[:i]
		}
		start, end := lo, hi
		switch {
		case part == "*" || part == "?":
		case strings.Contains(part, "-"):
			ab := strings.SplitN(part, "-", 2)
			a, err := num(ab[0], names)
			if err != nil {
				return 0, err
			}
			b, err := num(ab[1], names)
			if err != nil {
				return 0, err
			}
			start, end = a, b
		default:
			a, err := num(part, names)
			if err != nil {
				return 0, err
			}
			start = a
			if step == 1 {
				end = a
			}
		}
		if start < lo || end > hi || start > end {
			return 0, fmt.Errorf("%q out of range %d-%d", part, lo, hi)
		}
		for v := start; v <= end; v += step {
			bits |= 1 << uint(v)
		}
	}
	return bits, nil
}

func num(s string, names map[string]int) (int, error) {
	if names != nil {
		if v, ok := names[strings.ToLower(s)]; ok {
			return v, nil
		}
	}
	return strconv.Atoi(s)
}

// Next returns the first activation strictly after t.
func (s *Schedule) Next(t time.Time) time.Time {
	t = t.Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0)
	for t.Before(limit) {
		if s.month&(1<<uint(t.Month())) == 0 {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !s.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}
		if s.hour&(1<<uint(t.Hour())) == 0 {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}
		if s.min&(1<<uint(t.Minute())) == 0 {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

// Standard cron: when both day fields are restricted, either may match.
func (s *Schedule) dayMatches(t time.Time) bool {
	d := s.dom&(1<<uint(t.Day())) != 0
	w := s.dow&(1<<uint(t.Weekday())) != 0
	switch {
	case s.domStar && s.dowStar:
		return true
	case s.domStar:
		return w
	case s.dowStar:
		return d
	}
	return d || w
}

// Describe renders a schedule in plain English for the common shapes.
func Describe(expr string) string {
	expr = strings.TrimSpace(expr)
	switch expr {
	case "":
		return "Manual only"
	case "@hourly", "0 * * * *":
		return "Every hour"
	case "@daily", "@midnight", "0 0 * * *":
		return "Every day at midnight"
	case "@weekly", "0 0 * * 0":
		return "Every Sunday at midnight"
	case "@monthly", "0 0 1 * *":
		return "On the 1st of every month"
	case "* * * * *":
		return "Every minute"
	}
	f := strings.Fields(expr)
	if len(f) == 5 {
		if strings.HasPrefix(f[0], "*/") && f[1] == "*" && f[2] == "*" && f[3] == "*" && f[4] == "*" {
			return "Every " + f[0][2:] + " minutes"
		}
		if f[0] == "0" && strings.HasPrefix(f[1], "*/") && f[2] == "*" && f[3] == "*" && f[4] == "*" {
			return "Every " + f[1][2:] + " hours"
		}
		m, err1 := strconv.Atoi(f[0])
		h, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil && f[2] == "*" && f[3] == "*" {
			at := fmt.Sprintf("%02d:%02d UTC", h, m)
			switch f[4] {
			case "*":
				return "Every day at " + at
			case "1-5":
				return "Weekdays at " + at
			}
		}
	}
	return expr
}
