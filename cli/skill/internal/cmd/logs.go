package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/client"
	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/config"
)

// newLogsCmd builds `skill logs <ns>/<name>`.
func newLogsCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "logs <namespace/name>",
		Short: "Print recent invocations of a skill",
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
			entries, err := c.ListLogs(cmd.Context(), ns, name)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no invocations yet.")
				return nil
			}
			if limit > 0 && limit < len(entries) {
				entries = entries[:limit]
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "STARTED\tSTATUS\tLATENCY\tIN/OUT\tCALLER_IP\tERROR")
			for _, e := range entries {
				ioBytes := fmt.Sprintf("%d/%d", e.InputBytes, e.OutputBytes)
				latency := fmt.Sprintf("%dms", e.LatencyMS)
				errMsg := e.ErrorMessage
				if len(errMsg) > 60 {
					errMsg = errMsg[:60] + "…"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
					e.StartedAt, e.Status, latency, ioBytes, e.CallerIP, errMsg)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of rows to print")
	return cmd
}
