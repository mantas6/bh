package main

import (
	"runtime/debug"
	"strings"
)

// version is the release version, injected at build time with
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/bh
//
// When left at "dev", buildVersion falls back to the module build info.
var version = "dev"

func buildVersion() string {
	info, _ := debug.ReadBuildInfo()
	return resolveVersion(version, info)
}

// resolveVersion returns the version to report. An ldflags-injected version
// wins. Otherwise the main module version recorded by the go tool is used
// (e.g. "v1.2.3" from `go install ...@v1.2.3`, or a pseudo-version for a
// build inside a git checkout). A "(devel)" build reports "dev", suffixed
// with the short VCS revision and "-dirty" when known.
func resolveVersion(ldflags string, info *debug.BuildInfo) string {
	if ldflags != "" && ldflags != "dev" {
		return strings.TrimPrefix(ldflags, "v")
	}
	if info == nil {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return strings.TrimPrefix(v, "v")
	}

	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if revision == "" {
		return "dev"
	}
	if len(revision) > 12 {
		revision = revision[:12]
	}
	v := "dev-" + revision
	if dirty {
		v += "-dirty"
	}
	return v
}
