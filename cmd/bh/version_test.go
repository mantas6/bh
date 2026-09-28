package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	vcs := func(rev, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: rev},
			{Key: "vcs.modified", Value: modified},
		}
	}
	const rev = "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name    string
		ldflags string
		info    *debug.BuildInfo
		want    string
	}{
		{"ldflags wins", "1.2.3", &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, "1.2.3"},
		{"ldflags v prefix", "v1.2.3", nil, "1.2.3"},
		{"no build info", "dev", nil, "dev"},
		{"empty ldflags", "", nil, "dev"},
		{"go install tag", "dev", &debug.BuildInfo{Main: debug.Module{Version: "v0.4.0"}}, "0.4.0"},
		{
			"pseudo-version",
			"dev",
			&debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260928120000-0123456789ab+dirty"}},
			"0.0.0-20260928120000-0123456789ab+dirty",
		},
		{"devel without vcs", "dev", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, "dev"},
		{
			"devel with revision",
			"dev",
			&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs(rev, "false")},
			"dev-0123456789ab",
		},
		{
			"devel dirty",
			"dev",
			&debug.BuildInfo{Main: debug.Module{Version: "(devel)"}, Settings: vcs(rev, "true")},
			"dev-0123456789ab-dirty",
		},
		{
			"short revision",
			"dev",
			&debug.BuildInfo{Settings: vcs("abc123", "false")},
			"dev-abc123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveVersion(tt.ldflags, tt.info); got != tt.want {
				t.Errorf("resolveVersion(%q) = %q, want %q", tt.ldflags, got, tt.want)
			}
		})
	}
}
