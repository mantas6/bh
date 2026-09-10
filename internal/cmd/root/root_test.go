package root

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmdutil"
)

func TestRootVersion(t *testing.T) {
	ios, _, out, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios, Version: "1.2.3"}

	cmd := NewCmdRoot(f)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "1.2.3") {
		t.Fatalf("version output = %q, want to contain %q", got, "1.2.3")
	}
}
