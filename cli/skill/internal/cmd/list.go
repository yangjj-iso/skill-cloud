package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/client"
	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/config"
)

// newListCmd builds `skill list` — fetches every skill in the caller's
// org and prints them as a table.
func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every skill registered in the caller's org",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Require()
			if err != nil {
				return err
			}
			host := resolveHost(cfg.Host)
			apiKey := resolveAPIKey(cfg.APIKey)

			c := client.New(host, apiKey, nil)
			skills, err := c.ListSkills(cmd.Context())
			if err != nil {
				return err
			}
			if len(skills) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no skills registered. Run `skill push` after `skill init`.")
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAMESPACE/NAME\tVERSION\tRUNTIME\tDESCRIPTION")
			for _, s := range skills {
				fmt.Fprintf(tw, "%s/%s\t%s\t%s\t%s\n", s.Namespace, s.Name, s.Version, s.Runtime.Type, s.Description)
			}
			return tw.Flush()
		},
	}
}
