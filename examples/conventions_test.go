package examples

// The conventions every example in this repo follows (#60, ADR 0007 decision 6), as far as
// they are mechanical; examples/README.md states them all. They hold for this repo's examples
// only, so the test lives here and not in tools/, which projects copy.
//
// `go -C examples test ./...` runs it.

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// readmeSections are an example README's sections, in order.
var readmeSections = []string{
	"What it shows",
	"Run it",
	"Layout and who owns what",
	"What is checked, and how far",
	"The mutants",
	"Not verified",
}

// chapterSections are a chapter's numbered sections; section 2, the process, is named for it.
var chapterSections = []string{
	"Glossary",
	"",
	"What must hold",
	"Scenarios",
	"Assumptions",
	"Not covered",
	"What the lifecycle does not show",
}

var (
	readingHeader = regexp.MustCompile(`^Reading ([AB]) of the .+ chapter, by ([^,]+), \d{4}-\d{2}-\d{2}\. Made from \(git hash-object\): ` +
		`chapter\.md [0-9a-f]{40}, readings/prompt\.md [0-9a-f]{40}, docs/reference/spine-format\.md [0-9a-f]{40}\. `)
	spineHeader = regexp.MustCompile(`^The lifecycle of .+ \(chapter\.md, section 2\)\. Built on \d{4}-\d{2}-\d{2} from readings/ ` +
		`and the owner's answers in #\d+: .+ Ratified by the reviewed merge of this file\.$`)
	// rule1 is the prompt's first rule, which names the machine and its variables.
	rule1 = regexp.MustCompile(`(?s)\n1\. .*?\n2\. `)
)

func TestConventions(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	prompts := map[string]string{} // each example's prompt, without rule 1
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			dir := e.Name()
			if got := headings(t, filepath.Join(dir, "README.md"), "## "); !reflect.DeepEqual(got, readmeSections) {
				t.Errorf("%s/README.md has the sections %q, not %q", dir, got, readmeSections)
			}
			machines, _ := filepath.Glob(filepath.Join(dir, "spec", "*", "spine.yaml"))
			if len(machines) == 0 {
				t.Errorf("%s: no spec/*/spine.yaml", dir)
			}
			for _, spine := range machines {
				spec := filepath.Dir(spine)
				checkChapter(t, filepath.Join(spec, "chapter.md"))
				checkReadings(t, filepath.Join(spec, "readings"))
				checkSpine(t, spine)
				prompts[dir] = rule1.ReplaceAllString(read(t, filepath.Join(spec, "readings", "prompt.md")), "\n1. \n2. ")
			}
			if got := names(t, filepath.Join(dir, "mutation")); !reflect.DeepEqual(got, []string{"mutants.yaml"}) {
				t.Errorf("%s/mutation holds %q, not mutants.yaml alone", dir, got)
			}
			checkAdapter(t, filepath.Join(dir, "conformance"))
		})
	}
	var first string
	for dir, prompt := range prompts {
		if first == "" {
			first = dir
		} else if prompt != prompts[first] {
			t.Errorf("%s's readings/prompt.md differs from %s's in more than rule 1", dir, first)
		}
	}
}

// checkChapter: a title, then the seven numbered sections.
func checkChapter(t *testing.T, path string) {
	t.Helper()
	text := read(t, path)
	if !strings.HasPrefix(text, "# ") {
		t.Errorf("%s doesn't start with a title", path)
	}
	got := headings(t, path, "## ")
	if len(got) != len(chapterSections) {
		t.Errorf("%s has the sections %q, not the seven %q, section 2 named for the process", path, got, chapterSections)
		return
	}
	for i, name := range chapterSections {
		number, title, _ := strings.Cut(got[i], ". ")
		if number != string(rune('1'+i)) || (name != "" && title != name) || title == "" {
			t.Errorf("%s: section %d is %q, not %d. %s", path, i+1, got[i], i+1, name)
		}
	}
}

// checkReadings: the prompt and two readings, each headed with its model, date and inputs, by
// two different models.
func checkReadings(t *testing.T, dir string) {
	t.Helper()
	if got := names(t, dir); !reflect.DeepEqual(got, []string{"prompt.md", "reading-a.yaml", "reading-b.yaml"}) {
		t.Errorf("%s holds %q, not prompt.md, reading-a.yaml and reading-b.yaml", dir, got)
		return
	}
	models := map[string]string{}
	for _, f := range []string{"reading-a.yaml", "reading-b.yaml"} {
		m := readingHeader.FindStringSubmatch(comment(t, filepath.Join(dir, f)))
		if m == nil || "reading-"+strings.ToLower(m[1])+".yaml" != f {
			t.Errorf("%s doesn't start with its reader, model, date and inputs, as the other readings do", filepath.Join(dir, f))
			continue
		}
		models[f] = m[2]
	}
	if len(models) == 2 && models["reading-a.yaml"] == models["reading-b.yaml"] {
		t.Errorf("%s: both readings are by %s; they are by two different models", dir, models["reading-a.yaml"])
	}
}

// checkSpine: the header says where the spine came from, and there are no worked examples.
func checkSpine(t *testing.T, path string) {
	t.Helper()
	if !spineHeader.MatchString(comment(t, path)) {
		t.Errorf("%s doesn't start with the lifecycle it is, and when and from what it was built, as the other spines do", path)
	}
	if regexp.MustCompile(`(?m)^examples:`).MatchString(read(t, path)) {
		t.Errorf("%s has worked examples; sample runs go in behaviors/", path)
	}
}

// checkAdapter: the adapter's program states the whole contract with src/.
func checkAdapter(t *testing.T, dir string) {
	t.Helper()
	for _, f := range names(t, dir) {
		if !strings.HasSuffix(f, ".cmd") && strings.Contains(read(t, filepath.Join(dir, f)), "What src/ must offer") {
			return
		}
	}
	t.Errorf("%s: no adapter states what src/ must offer", dir)
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// names are the files in dir, sorted; folders, such as a language's caches, are left out.
func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() {
			out = append(out, e.Name())
		}
	}
	return out
}

// headings are the file's lines that start with prefix, without it.
func headings(t *testing.T, path, prefix string) []string {
	t.Helper()
	var out []string
	for line := range strings.SplitSeq(read(t, path), "\n") {
		if h, ok := strings.CutPrefix(line, prefix); ok {
			out = append(out, h)
		}
	}
	return out
}

// comment is the file's first comment lines as one line of text.
func comment(t *testing.T, path string) string {
	t.Helper()
	var words []string
	for line := range strings.SplitSeq(read(t, path), "\n") {
		text, ok := strings.CutPrefix(line, "#")
		if !ok {
			break
		}
		words = append(words, strings.Fields(text)...)
	}
	return strings.Join(words, " ")
}
