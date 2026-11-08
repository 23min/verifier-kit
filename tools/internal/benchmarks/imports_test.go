package benchmarks

// The harness judges spine from outside (#106): it imports none of the toolkit's packages, so
// that the verdicts it expects share no code with the program it runs.

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestImports(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if path, _ := strconv.Unquote(imp.Path.Value); strings.HasPrefix(path, "github.com/23min/verifier-kit/") {
				t.Errorf("%s imports %s: the harness runs spine as a program and shares no code with it", name, path)
			}
		}
	}
}
