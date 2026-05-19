package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/client"
	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/config"
)

// newStatsCmd builds `skill stats <ns>/<name>`.
func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats <namespace/name>",
		Short: "Print aggregate stats for a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns, name, ok := splitQualified(args[0])
			if !ok {
				return fmt.Errorf("expected `namespace/name`, got %q", args[0])
			}
			cfg, err := config.Require()
			if err != nil {
				return err
			}
			host := resolveHost(cfg.Host)
			apiKey := resolveAPIKey(cfg.APIKey)
			c := client.New(host, apiKey, nil)
			stats, err := c.GetStats(cmd.Context(), ns, name)
			if err != nil {
				return err
			}
			pretty, err := json.MarshalIndent(stats, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(pretty))
			return nil
		},
	}
}
