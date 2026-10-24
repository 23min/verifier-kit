// Command spine checks state-machine specs mechanically: files in, findings out.
// See docs/reference/tool-contract.md.
package main

import (
	"os"

	"github.com/23min/verifier-kit/tools/internal/spine"
)

func main() {
	os.Exit(spine.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
