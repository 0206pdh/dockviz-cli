package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// pflag turns text in backticks inside a flag's usage into the argument
// name shown in --help ("--context docker context ls" instead of
// "--context string"). Keep backticks out of flag usage strings.
func TestFlagUsageHasNoBackticks(t *testing.T) {
	rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if strings.Contains(f.Usage, "`") {
			t.Errorf("--%s usage contains a backtick: %q", f.Name, f.Usage)
		}
		if name, _ := pflag.UnquoteUsage(f); f.Value.Type() == "string" && name != "string" {
			t.Errorf("--%s shows argument name %q in --help, want \"string\"", f.Name, name)
		}
	})
}
