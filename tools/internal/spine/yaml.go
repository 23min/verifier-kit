package spine

import (
	"os"
	"strings"

	"github.com/23min/verifier-kit/tools/internal/program"
	"github.com/23min/verifier-kit/tools/internal/yamldoc"
)

// readYAML reads a file of the spine format: one YAML document in UTF-8. A file that can't be
// read, isn't UTF-8 or isn't YAML adds a spec-unreadable finding and gives false.
func readYAML(path string, out *Findings) (*yamldoc.Value, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		out.Add("spec-unreadable", path, "cannot be read: "+program.Strerror(err))
		return nil, false
	}
	v, err := yamldoc.Load(data)
	if err != nil {
		out.Add("spec-unreadable", path, strings.SplitN(err.Error(), "\n", 2)[0])
		return nil, false
	}
	return v, true
}
