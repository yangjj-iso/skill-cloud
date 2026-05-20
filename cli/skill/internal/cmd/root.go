// Package cmd defines the `skill` CLI command tree. Each subcommand is
// in its own file (init.go, login.go, push.go, ...) and registers
// itself on the root command via init().
//
// The root command exposes shared flags (--host, --api-key) that
// override the values loaded from ~/.skillcloud/config.yaml /
// $SKILLCLOUD_* environment variables, so a single invocation can
// target a different server without touching the saved profile.
package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

// rootOpts holds flag values shared between subcommands.
type rootOpts struct {
	hostOverride   string
	apiKeyOverride string
}

// Globals is populated by the root command for subcommands to read.
var Globals rootOpts

// NewRoot returns the root `skill` command. The CLI's main.go calls
// Execute() on this; tests construct the command tree directly so they
// can drive subcommands with `cmd.SetArgs(...)` and capture output.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "skill",
		Short:         "Skill Cloud command-line client",
		Long:          "skill manages and invokes Skill Cloud skills from the terminal.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&Globals.hostOverride, "host", "", "Skill Cloud server URL (overrides config + SKILLCLOUD_HOST)")
	root.PersistentFlags().StringVar(&Globals.apiKeyOverride, "api-key", "", "API key (overrides config + SKILLCLOUD_API_KEY)")

	root.AddCommand(newInitCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newPushCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newCallCmd())
	root.AddCommand(newLogsCmd())
	root.AddCommand(newStatsCmd())
	return root
}

// Execute is the entrypoint called by main.go.
func Execute() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := NewRoot()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// resolveHost returns the effective server host, applying --host > env > config > default.
func resolveHost(cfgHost string) string {
	if Globals.hostOverride != "" {
		return Globals.hostOverride
	}
	return cfgHost
}

// resolveAPIKey returns the effective API key, applying --api-key > env > config.
func resolveAPIKey(cfgKey string) string {
	if Globals.apiKeyOverride != "" {
		return Globals.apiKeyOverride
	}
	return cfgKey
}

// writeOrFail copies src to dst, panicking is not appropriate here —
// tests use it to capture command output without losing partial writes.
func writeOrFail(w io.Writer, msg string) error {
	if _, err := fmt.Fprint(w, msg); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
