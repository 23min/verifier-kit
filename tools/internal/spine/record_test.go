package spine

// The record (#44): what spine gives on every input of the cases test, kept in testdata/record/
// as its reference. For each
// run it holds the exit code, the output, and in JSON runs the JUnit report, with the paths that
// differ between machines written as $ROOT and $TMP. spine must give exactly what is recorded, and every entry must still have its case (tools/internal/record).
//
// `go -C tools test ./internal/spine/ -update` writes the record again from spine.
// Review the diff before committing it: the record is the only reference.

import (
	"flag"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/23min/verifier-kit/tools/internal/record"
)

var update = flag.Bool("update", false, "write testdata/record/ again from the Go spine")

var (
	rec       *record.Record
	recordTmp string // TestCases's temporary folder, written $TMP
)

// loadRecord reads testdata/record/, unless the test is to write it.
func loadRecord(t *testing.T, td, tmp string) {
	recordTmp = tmp
	rec = record.Load(t, filepath.Join(td, "record"), *update, "the Go spine", "go -C tools test ./internal/spine/ -update")
}

// recordFile names the file a case's runs are kept in, from the case's name.
func recordFile(name string) string {
	for _, f := range []struct{ prefix, file string }{
		{"cases.txt", "cases"}, {"cli.txt", "cli"}, {"spines/", "files"}, {"seeds/", "files"},
		{"yaml/", "yaml"}, {"spine variant", "variants"}, {"behavior variant", "variants"},
		{"policy variant", "variants"}, {"value ", "values"}, {"filesystem", "filesystem"},
		{"stdin not UTF-8", "filesystem"}, {"example", "examples"}, {"fault ", "faults"},
		{"conditions", "conditions"},
	} {
		if strings.HasPrefix(name, f.prefix) {
			return f.file
		}
	}
	return ""
}

// normal writes the paths that differ between machines as $TMP and $ROOT.
func normal(s string) string {
	if recordTmp != "" {
		s = strings.ReplaceAll(s, recordTmp, "$TMP")
	}
	return strings.ReplaceAll(s, repoRoot, "$ROOT")
}

// key names one run of a case: its folder, its command line, its input and its environment.
func (c spineCase) key(args []string) string {
	dir := normal(c.dir)
	if rel, err := filepath.Rel(repoRoot, c.dir); err == nil && !strings.HasPrefix(rel, "..") {
		dir = rel
	}
	parts := []string{"in", record.Word(dir) + ":", "spine"}
	for _, a := range args {
		parts = append(parts, record.Word(normal(a)))
	}
	if c.stdin != "" {
		parts = append(parts, "<", record.Word(c.stdin))
	}
	env := append([]string(nil), c.env...)
	sort.Strings(env)
	for _, e := range env {
		parts = append(parts, "with", record.Word(normal(e)))
	}
	return strings.Join(parts, " ")
}

// recordRun compares one run of the Go spine with the record, or keeps it to write the record.
func (c spineCase) recordRun(t *testing.T, args []string, r result, junit *string) {
	t.Helper()
	file := recordFile(c.name)
	if file == "" {
		t.Fatalf("%s: no file in testdata/record/ for this case; add its prefix to recordFile", c.name)
	}
	got := record.Entry{Exit: r.exit, Sections: []record.Section{{Name: "stdout", Text: normal(r.stdout)}}}
	// A usage error's message, on stderr, is spine's own; an adapter's stderr is not recorded, as it
	// varies with the version of the adapter's language.
	if r.exit == 2 {
		got.Sections = append(got.Sections, record.Section{Name: "stderr", Text: normal(r.stderr)})
	}
	if junit != nil {
		got.Sections = append(got.Sections, record.Section{Name: "junit", Text: normal(*junit)})
	}
	rec.Check(t, c.name, file, c.key(args), got)
}
