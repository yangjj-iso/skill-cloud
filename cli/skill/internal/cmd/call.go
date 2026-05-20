package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/client"
	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/config"
)

// newCallCmd builds `skill call <ns>/<name>`. Reads input JSON from
// --input (inline), --input-file, or stdin in that order. Output is
// pretty-printed JSON.
func newCallCmd() *cobra.Command {
	var (
		inputInline string
		inputFile   string
	)
	cmd := &cobra.Command{
		Use:   "call <namespace/name>",
		Short: "Invoke a skill and print its output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ns, name, ok := splitQualified(args[0])
			if !ok {
				return fmt.Errorf("expected `namespace/name`, got %q", args[0])
			}
			input, err := readInput(cmd.InOrStdin(), inputInline, inputFile)
			if err != nil {
				return err
			}
			cfg, err := config.Require()
			if err != nil {
				return err
			}
			host := resolveHost(cfg.Host)
			apiKey := resolveAPIKey(cfg.APIKey)
			c := client.New(host, apiKey, nil)
			result, err := c.Invoke(cmd.Context(), ns, name, input)
			if err != nil {
				return err
			}
			pretty, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(pretty))
			if result.Status != "ok" {
				return fmt.Errorf("invocation status: %s", result.Status)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&inputInline, "input", "", "input JSON (inline)")
	cmd.Flags().StringVar(&inputFile, "input-file", "", "input JSON from a file (- for stdin)")
	return cmd
}

func readInput(stdin io.Reader, inline, fromFile string) (map[string]any, error) {
	var raw []byte
	switch {
	case inline != "":
		raw = []byte(inline)
	case fromFile == "-" || (fromFile == "" && !termIsStdin(stdin)):
		buf, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		raw = buf
	case fromFile != "":
		buf, err := os.ReadFile(fromFile)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fromFile, err)
		}
		raw = buf
	default:
		// No input supplied; send {}.
		return map[string]any{}, nil
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("input is not valid JSON: %w", err)
	}
	return out, nil
}

// termIsStdin reports whether stdin is a TTY. When stdin is not
// connected to a terminal (e.g. piped input), we read from it
// automatically; on a TTY we don't block waiting for input and instead
// send `{}` so `skill call ns/name` without arguments doesn't hang.
func termIsStdin(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// splitQualified parses `namespace/name`.
func splitQualified(s string) (string, string, bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Re-export of client.IsAuthError so callers don't need to import the
// internal package directly. Currently unused by tests but exposed for
// possible future use by wrappers.
var _ = client.IsAuthError

// errFlag is a sentinel used internally to fail-fast on truly invalid
// CLI invocations (e.g. missing positional). Kept here so subcommand
// files can return a typed error if needed in the future.
var errFlag = errors.New("invalid arguments")

var _ = errFlag
