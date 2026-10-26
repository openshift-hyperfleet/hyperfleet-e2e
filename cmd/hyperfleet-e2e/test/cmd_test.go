package test

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

func newBindingCommand() *cobra.Command {
	parent := &cobra.Command{Use: "runner"}
	for _, name := range []string{"api-url", "log-level", "log-format", "log-output"} {
		parent.PersistentFlags().String(name, "", "")
	}
	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{"label-filter", "focus", "skip", "junit-report"} {
		cmd.Flags().String(name, "", "")
	}
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().Int("flake-attempts", 1, "")
	parent.AddCommand(cmd)
	return cmd
}

func TestBindFlagsAndEnvPrecedence(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("GINKGO_LABEL_FILTER", "tier1")
	cmd := newBindingCommand()
	if err := bindFlagsAndEnv(cmd); err != nil {
		t.Fatal(err)
	}
	if got := viper.GetString(config.Tests.GinkgoLabelFilter); got != "tier1" {
		t.Fatalf("environment filter = %q, want tier1", got)
	}
	if err := cmd.Flags().Set("label-filter", "tier0"); err != nil {
		t.Fatal(err)
	}
	if got := viper.GetString(config.Tests.GinkgoLabelFilter); got != "tier0" {
		t.Fatalf("flag filter = %q, want tier0", got)
	}
	if err := cmd.Parent().PersistentFlags().Set("api-url", "https://api.example.test"); err != nil {
		t.Fatal(err)
	}
	if got := viper.GetString(config.API.URL); got != "https://api.example.test" {
		t.Fatalf("parent API flag = %q", got)
	}
}

func TestBindFlagsAndEnvRejectsMissingFlags(t *testing.T) {
	for _, parentFlag := range []bool{false, true} {
		t.Run(map[bool]string{false: "test flag", true: "parent flag"}[parentFlag], func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			cmd := newBindingCommand()
			want := "failed to bind flag"
			if parentFlag {
				cmd.Parent().ResetFlags()
				want = "failed to bind parent flag"
			} else {
				cmd.ResetFlags()
			}
			if err := bindFlagsAndEnv(cmd); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("got %v, want %q", err, want)
			}
		})
	}
}
