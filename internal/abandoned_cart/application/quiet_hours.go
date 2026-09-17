package application

import (
	"fmt"
	"strings"
	"time"
)

// ParseQuietHours accepts an optional UTC wall-clock interval in HH:MM-HH:MM
// form. Intervals crossing midnight are supported.
func ParseQuietHours(raw string) (func(time.Time) (time.Time, bool), error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return func(time.Time) (time.Time, bool) { return time.Time{}, false }, nil
	}
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("expected HH:MM-HH:MM")
	}
	parse := func(value string) (int, error) {
		parsed, err := time.Parse("15:04", strings.TrimSpace(value))
		if err != nil {
			return 0, err
		}
		return parsed.Hour()*60 + parsed.Minute(), nil
	}
	start, err := parse(parts[0])
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	end, err := parse(parts[1])
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	if start == end {
		return nil, fmt.Errorf("quiet-hours range must not cover a full day")
	}
	return func(now time.Time) (time.Time, bool) {
		now = now.UTC()
		minute := now.Hour()*60 + now.Minute()
		quiet := (start < end && minute >= start && minute < end) || (start > end && (minute >= start || minute < end))
		if !quiet {
			return time.Time{}, false
		}
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		until := day.Add(time.Duration(end) * time.Minute)
		if start > end && minute >= start {
			until = until.AddDate(0, 0, 1)
		}
		return until, true
	}, nil
}
