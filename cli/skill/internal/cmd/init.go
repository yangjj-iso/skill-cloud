package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/template"
)

// newInitCmd builds the `skill init` command. It scaffolds a skill
// project in a target directory: skill.yaml manifest, a tiny Python
// entrypoint, a Dockerfile, and a README. Aimed at making the
// hello-skill end-to-end demo achievable in two commands (init + push).
func newInitCmd() *cobra.Command {
	var (
		namespace string
		dir       string
		runtime   string
	)
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Scaffold a new skill in the current directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !validIdent.MatchString(name) {
				return fmt.Errorf("invalid name %q: must match %s", name, validIdent.String())
			}
			if !validIdent.MatchString(namespace) {
				return fmt.Errorf("invalid namespace %q: must match %s", namespace, validIdent.String())
			}
			target := dir
			if target == "" {
				target = name
			}
			absTarget, err := filepath.Abs(target)
			if err != nil {
				return fmt.Errorf("resolve target: %w", err)
			}
			if _, err := os.Stat(absTarget); err == nil {
				return fmt.Errorf("target %q already exists; pass --dir to override", absTarget)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat %q: %w", absTarget, err)
			}

			if err := template.Render(absTarget, template.Params{
				Namespace: namespace,
				Name:      name,
				Runtime:   runtime,
			}); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "scaffolded %s/%s in %s\n", namespace, name, absTarget)
			fmt.Fprintf(cmd.OutOrStdout(), "next steps:\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  cd %s\n", target)
			fmt.Fprintf(cmd.OutOrStdout(), "  docker build -t %s/%s:0.1.0 .\n", namespace, name)
			fmt.Fprintf(cmd.OutOrStdout(), "  skill push\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&namespace, "namespace", "demo", "skill namespace")
	cmd.Flags().StringVar(&dir, "dir", "", "target directory (defaults to ./<name>)")
	cmd.Flags().StringVar(&runtime, "runtime", "docker", "runtime type: docker | http_proxy")
	return cmd
}

// validIdent matches the regex enforced by the server's manifest
// validation: [a-z0-9][a-z0-9_-]{0,62}. Keeping it in sync with the
// server saves a useless round-trip when the name is obviously bad.
var validIdent = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
