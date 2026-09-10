// Package output provides small presentation helpers shared by bh commands:
// aligned tables, pretty JSON, ANSI color schemes, and relative-time
// formatting.
package output

import (
	"fmt"
	"strings"
	"time"
)

// ColorScheme applies ANSI colors to strings. When disabled, every method is
// a no-op returning its input unchanged.
type ColorScheme struct {
	enabled bool
}

// NewColorScheme returns a ColorScheme. When enabled is false the methods
// return their input unchanged.
func NewColorScheme(enabled bool) *ColorScheme {
	return &ColorScheme{enabled: enabled}
}

func (c *ColorScheme) color(code, s string) string {
	if !c.enabled || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Green colors s green.
func (c *ColorScheme) Green(s string) string { return c.color("32", s) }

// Red colors s red.
func (c *ColorScheme) Red(s string) string { return c.color("31", s) }

// Yellow colors s yellow.
func (c *ColorScheme) Yellow(s string) string { return c.color("33", s) }

// Gray colors s gray (bright black).
func (c *ColorScheme) Gray(s string) string { return c.color("90", s) }

// Bold renders s in bold.
func (c *ColorScheme) Bold(s string) string { return c.color("1", s) }

// Cyan colors s cyan.
func (c *ColorScheme) Cyan(s string) string { return c.color("36", s) }

// Magenta colors s magenta.
func (c *ColorScheme) Magenta(s string) string { return c.color("35", s) }

// SuccessIcon returns a green check mark ("✓"), plain when color is disabled.
func (c *ColorScheme) SuccessIcon() string { return c.Green("✓") }

// FailureIcon returns a red cross ("✗"), plain when color is disabled.
func (c *ColorScheme) FailureIcon() string { return c.Red("✗") }

// WarningIcon returns a yellow bang ("!"), plain when color is disabled.
func (c *ColorScheme) WarningIcon() string { return c.Yellow("!") }

// StateColor colors a PR state string: OPEN->green, MERGED->magenta,
// DECLINED->red, SUPERSEDED->gray. Unknown states are returned unchanged.
func (c *ColorScheme) StateColor(state string) string {
	switch strings.ToUpper(state) {
	case "OPEN":
		return c.Green(state)
	case "MERGED":
		return c.Magenta(state)
	case "DECLINED":
		return c.Red(state)
	case "SUPERSEDED":
		return c.Gray(state)
	default:
		return state
	}
}

// RelativeTime renders the difference between now and t as a human phrase such
// as "about 3 hours ago" or "2 days ago".
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
	case d < 48*time.Hour:
		return "1 day ago"
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return "about a month ago"
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(d.Hours()/(24*30)))
	case d < 2*365*24*time.Hour:
		return "about a year ago"
	default:
		return fmt.Sprintf("%d years ago", int(d.Hours()/(24*365)))
	}
}
