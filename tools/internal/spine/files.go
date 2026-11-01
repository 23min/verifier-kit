package spine

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/23min/verifier-kit/tools/internal/program"
)

// Files and folders: listing them and reading them, for the core to judge.

// readFile reads the file at path, or says why it can't.
func readFile(path string) file {
	data, err := os.ReadFile(path)
	if err != nil {
		return file{path: path, unreadable: program.Strerror(err)}
	}
	return file{path: path, data: data}
}

// entries lists a folder's entries in name order, or none when it can't be read.
func entries(dir string) []os.DirEntry {
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return list
}

// yamlFiles reads every entry of dir named *.yaml or *.yml, in name order: hidden ones and broken
// links included.
func yamlFiles(dir string) []file {
	var out []file
	for _, e := range entries(dir) {
		if strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml") {
			out = append(out, readFile(filepath.Join(dir, e.Name())))
		}
	}
	return out
}

// behaviorFiles reads the behavior files of dir: those in its valid/ folder, and those in invalid/.
func behaviorFiles(dir string) (valid, invalid []file) {
	return yamlFiles(filepath.Join(dir, "valid")), yamlFiles(filepath.Join(dir, "invalid"))
}

// below is everything below dir, a folder before what it holds and each folder in name order,
// without following links to folders, and without what an unreadable folder holds.
func below(dir string) []string {
	var out []string
	var walk func(string)
	walk = func(d string) {
		for _, e := range entries(d) {
			p := filepath.Join(d, e.Name())
			out = append(out, p)
			if e.IsDir() { // a DirEntry reports a link as a link, not as what it points to
				walk(p)
			}
		}
	}
	walk(filepath.Clean(dir))
	return out
}

// filesBelow reads every regular file below dir, in below's order, following links.
func filesBelow(dir string) []file {
	var out []file
	for _, p := range below(dir) {
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			out = append(out, readFile(p))
		}
	}
	return out
}
