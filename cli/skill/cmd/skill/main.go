// Command skill is the Skill Cloud command-line client. The thin main
// package only handles process-level concerns (exit code wiring); the
// command tree lives in cli/skill/internal/cmd.
package main

import (
	"os"

	"github.com/yangjj-iso/skill-cloud/cli/skill/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
