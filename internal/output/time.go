package output

import (
	"fmt"
	"time"
)

// RelativeTime renders the difference between now and t as a human phrase such
// as "about 3 hours ago" or "2 days ago".
//
// Durations of a day or more are bucketed into whole days (< 30 days), whole
// 30-day months (< 12 months) and whole 365-day years. Anything that would
// round to 12 months is reported as "1 year ago" so the two units never
// overlap.
func RelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := now.Sub(t)
	if d < 0 {
		// Future timestamps (clock skew) are reported as the present.
		return "just now"
	}
	switch {
	case d < time.Minute:
		return "less than a minute ago"
	case d < 2*time.Minute:
		return "about a minute ago"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 2*time.Hour:
		return "about an hour ago"
	case d < 24*time.Hour:
		return fmt.Sprintf("about %d hours ago", int(d.Hours()))
	}

	days := int(d / (24 * time.Hour))
	if days < 30 {
		return ago(days, "day")
	}
	if months := days / 30; months < 12 {
		return ago(months, "month")
	}
	return ago(max(days/365, 1), "year")
}

// ago formats "n unit(s) ago" with correct pluralisation.
func ago(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
}
