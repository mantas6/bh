package output

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero", time.Time{}, "unknown"},
		{"future", now.Add(time.Hour), "just now"},
		{"seconds", now.Add(-30 * time.Second), "less than a minute ago"},
		{"one minute", now.Add(-90 * time.Second), "about a minute ago"},
		{"minutes", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"one hour", now.Add(-90 * time.Minute), "about an hour ago"},
		{"hours", now.Add(-3 * time.Hour), "about 3 hours ago"},
		{"one day", now.Add(-25 * time.Hour), "1 day ago"},
		{"days", now.Add(-48 * time.Hour), "2 days ago"},
		{"29 days", now.Add(-29 * day), "29 days ago"},
		{"one month", now.Add(-30 * day), "1 month ago"},
		{"59 days", now.Add(-59 * day), "1 month ago"},
		{"two months", now.Add(-60 * day), "2 months ago"},
		{"11 months", now.Add(-359 * day), "11 months ago"},
		{"360 days is a year", now.Add(-360 * day), "1 year ago"},
		{"364 days", now.Add(-364 * day), "1 year ago"},
		{"365 days", now.Add(-365 * day), "1 year ago"},
		{"729 days", now.Add(-729 * day), "1 year ago"},
		{"two years", now.Add(-730 * day), "2 years ago"},
		{"ten years", now.Add(-3650 * day), "10 years ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := RelativeTime(tc.t, now); got != tc.want {
				t.Errorf("RelativeTime = %q, want %q", got, tc.want)
			}
		})
	}
}
