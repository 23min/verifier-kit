package spine

// The core test (#94): spine's core decides without reading a file, starting a program or
// printing (ADR 0007, decision 1). Every file of the package is core but the shell's, and the
// core imports only what coreImports lists. It sees imports, not calls: a core function that
// calls a shell one, or prints, is for review to catch.

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// shell is the files that read, start the adapter and print, around the core.
var shell = map[string]bool{"adapter.go": true, "cli.go": true, "files.go": true, "output.go": true}

// coreImports is what the core may import: no package for files, programs or the terminal. A call
// that prints, such as fmt.Println, needs no such import, and is for review to catch.
var coreImports = map[string]bool{
	"bytes": true, "crypto/sha256": true, "encoding/hex": true, "encoding/json": true, "errors": true,
	"fmt": true, "regexp": true, "slices": true, "sort": true, "strconv": true, "strings": true,
	"unicode": true, "unicode/utf8": true, "github.com/23min/verifier-kit/tools/internal/yamldoc": true,
}

func TestCoreImports(t *testing.T) {
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if shell[name] || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			if path, _ := strconv.Unquote(imp.Path.Value); !coreImports[path] {
				t.Errorf("%s imports %s, which the core may not: read files, start programs and print in a shell file", name, path)
			}
		}
	}
}
