package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/client"
	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/config"
)

// newPushCmd builds `skill push`. It reads ./skill.yaml (or the file
// passed via --file), validates that the manifest matches what the
// server expects, and POSTs it to `/v1/skills`. The CLI does NOT
// upload the skill source today — the docker image referenced in the
// manifest must already be available on the host. That follow-up is
// tracked under M2's MinIO source-upload work.
func newPushCmd() *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Publish a skill manifest to the server",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Require()
			if err != nil {
				return err
			}
			host := resolveHost(cfg.Host)
			apiKey := resolveAPIKey(cfg.APIKey)

			path := file
			if path == "" {
				path = "skill.yaml"
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("resolve %s: %w", path, err)
			}
			raw, err := os.ReadFile(abs)
			if err != nil {
				return fmt.Errorf("read %s: %w", abs, err)
			}
			var manifest map[string]any
			if err := yaml.Unmarshal(raw, &manifest); err != nil {
				return fmt.Errorf("parse %s: %w", abs, err)
			}

			c := client.New(host, apiKey, nil)
			s, err := c.PublishSkill(cmd.Context(), manifest)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "published %s/%s@%s\n", s.Namespace, s.Name, s.Version)
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "path to manifest (defaults to ./skill.yaml)")
	return cmd
}
