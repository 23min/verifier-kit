// Command verify is what ./verify runs, once it has built spine and verify: every gate's
// checks, a line per step. See docs/reference/tool-contract.md.
package main

import (
	"os"

	"github.com/23min/verifier-kit/tools/internal/verify"
)

func main() {
	os.Exit(verify.Main(os.Args[1:], os.Stdout, os.Stderr))
}
