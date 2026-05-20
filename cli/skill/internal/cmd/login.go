package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/client"
	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/config"
)

// newLoginCmd builds `skill login`. It writes a config file with the
// supplied host + API key. The `skill login` flow doesn't perform an
// OAuth dance — the platform issues API keys directly (see
// `POST /v1/auth/api_keys`) and the operator pastes one in here.
func newLoginCmd() *cobra.Command {
	var (
		host   string
		apiKey string
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save a host + API key to ~/.skillcloud/config.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			host = strings.TrimSpace(host)
			apiKey = strings.TrimSpace(apiKey)
			if host == "" {
				host = config.DefaultHost
			}
			if apiKey == "" {
				return fmt.Errorf("--api-key is required (paste the value the server returned from POST /v1/auth/api_keys)")
			}

			// Sanity-check the host is reachable before we save —
			// catches typos / wrong port now rather than on every
			// subsequent command.
			c := client.New(host, apiKey, nil)
			if err := c.Healthz(cmd.Context()); err != nil {
				return fmt.Errorf("server health check failed: %w", err)
			}

			if err := config.Save(config.Config{Host: host, APIKey: apiKey}); err != nil {
				return err
			}
			path, _ := config.Path()
			fmt.Fprintf(cmd.OutOrStdout(), "saved credentials to %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&host, "host", config.DefaultHost, "Skill Cloud server URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "API key to save")
	return cmd
}
