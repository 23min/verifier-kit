package verify

// The record of ./verify (#68): what ./verify gives on fixture projects that between them reach
// every path through it, kept in testdata/record/ as the reference. It was written from the Bash
// ./verify; the Go verify that replaced it (#70) gives the same, except where #69 and #70 say.
// Each fixture is a project laid out like one that uses the toolkit: verify and tools/ copied in
// beside its own folders. For each run the record holds the exit code, standard output, standard
// error and every JUnit report the run wrote, with the test's temporary folder written as $TMP.
//
// ./verify runs as its callers run it, so the test needs what ./verify needs, Bash, Go and git,
// and Python 3 for the fixtures' commands. The fixtures need no Elixir. Every run is in the C
// locale, so that the programs it starts write their messages alike on every machine.
//
// `go -C tools test ./internal/verify/ -update` writes the record again from ./verify.
// Review the diff before committing it: the record is the only reference.

import (
	"bytes"
	"errors"
	"flag"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/23min/verifier-kit/tools/internal/record"
)

var update = flag.Bool("update", false, "write testdata/record/ again from ./verify")

// ---------- the fixtures ----------

// files maps a path in a project to its content.
type files map[string]string

// with is f with g's files added under dir.
func (f files) with(dir string, g files) files {
	out := files{}
	for p, c := range f {
		out[p] = c
	}
	for p, c := range g {
		out[filepath.Join(dir, p)] = c
	}
	return out
}

const lampSpine = `machine: lamp
vars: {}
states: {dark: {}, lit: {}, broken: {}}
initial: dark
events: [press, drop]
rules:
  - {from: dark, event: press, to: lit}
  - {from: lit, event: press, to: dark}
  - {from: dark, event: drop, to: broken}
  - {from: lit, event: drop, to: broken}
  - {from: broken, event: press, reject: true}
  - {from: broken, event: drop, reject: true}
`

// lampCode is the lamp; a second press leaves it lit when stuck.
func lampCode(stuck bool) string {
	next := `"lit" if self.state == "dark" else "dark"`
	if stuck {
		next = `"lit"`
	}
	return `class Lamp:
    def __init__(self):
        self.state = "dark"

    def press(self):
        if self.state == "broken":
            raise ValueError("broken")
        self.state = ` + next + `

    def drop(self):
        if self.state == "broken":
            raise ValueError("broken")
        self.state = "broken"
`
}

const lampAdapter = `from lamp import Lamp


def new():
    return Lamp()


def apply(lamp, event, env):
    try:
        getattr(lamp, event)()
    except ValueError:
        return "rejected"
    return "ok"


def observe(lamp):
    return lamp.state
`

const lampCheck = `from lamp import Lamp


def test_a_press_lights_it():
    lamp = Lamp()
    lamp.press()
    assert lamp.state == "lit"
`

// pythonLamp is a root in Python, each piece a command line that calls a helper in tools/.
func pythonLamp(stuck bool) files {
	f := pythonNames(stuck)
	f["conformance/adapter_lamp.cmd"] = "env PYTHONPATH=src adapter_shim.py conformance/adapter_lamp.py\n"
	f["checks/run.cmd"] = "env PYTHONPATH=src run_checks.py checks\n"
	f["mutation/mutants.yaml"] = lampMutants
	delete(f, "mutation/mutate_lamp.py") // a Python name, which pythonNames shows alone
	return f
}

// lampMutants are two mutants conform catches.
const lampMutants = `# Each one edit to src/ that the oracle must catch.
- name: a second press leaves it lit
  file: src/lamp.py
  find: '"lit" if self.state == "dark" else "dark"'
  replace: '"lit"'

- name: a dropped lamp still works
  file: src/lamp.py
  find: 'self.state = "broken"'
  replace: pass
`

// pythonNames is a root in Python with no command lines: the pieces only by their Python names.
func pythonNames(stuck bool) files {
	return files{
		"spec/lamp/spine.yaml":                    lampSpine,
		"spec/lamp/behaviors/valid/press.yaml":    "steps: [\"press\"]\nexpect: lit\n",
		"spec/lamp/behaviors/invalid/broken.yaml": "steps: [\"drop\", \"press\"]\nexpect: lit\n",
		"src/lamp.py":                             lampCode(stuck),
		"conformance/adapter_lamp.py":             lampAdapter,
		"checks/lamp/test_lamp.py":                lampCheck,
		"mutation/mutate_lamp.py":                 "print(\"-- 0 errors, 1 mutants\")\n",
	}
}

// commandLamp is a root whose lines name their programs by path, two folders below tools/.
func commandLamp() files {
	return files{
		"spec/lamp/spine.yaml":         lampSpine,
		"src/lamp.py":                  lampCode(false),
		"conformance/lamp.py":          "import os, sys\nsys.path.insert(0, os.path.join(os.path.dirname(__file__), \"..\", \"src\"))\n" + lampAdapter,
		"conformance/adapter_lamp.cmd": "python3 ../../tools/adapter_shim.py conformance/lamp.py\n",
		"checks/run.cmd":               "sh -c \"echo 1 check ran\"\n",
		"mutation/mutate_lamp.cmd":     "sh -c \"echo -- 0 errors, 1 mutants\"\r\n",
	}
}

// examplesProject holds a root for each way a root can pass or fail, under examples/.
func examplesProject() files {
	failing := pythonLamp(true)
	failing["checks/lamp/test_lamp.py"] = lampCheck + "\n\ndef test_a_second_press_darkens_it():\n    lamp = Lamp()\n    lamp.press()\n    lamp.press()\n    assert lamp.state == \"dark\", lamp.state\n"
	failing["checks/lamp/test_unloadable.py"] = "import no_such_module\n"

	failingCommands := commandLamp()
	failingCommands["src/lamp.py"] = lampCode(true)
	failingCommands["checks/run.cmd"] = "sh -c \"echo 1 of 2 checks failed; exit 1\"\n"
	failingCommands["mutation/mutate_lamp.cmd"] = "sh -c \"echo 1 mutant survived; exit 1\"\n"

	badCommands := files{
		"checks/run.cmd":          "no-such-program --checks\n",
		"mutation/mutate_one.cmd": "no-such-program --mutants\n",
	}
	for m, line := range map[string]string{
		"unreadable": "\xff\n",
		"twolines":   "sh\nsh\n",
		"unsplit":    "python3 \"conformance/lamp.py\n",
		"empty":      "   \n",
	} {
		badCommands["spec/"+m+"/spine.yaml"] = lampSpine
		badCommands["conformance/adapter_"+m+".cmd"] = line
	}

	// a mutant that survives, three that are stale, and a mutation command beside them
	mutants := pythonLamp(false)
	mutants["mutation/mutants.yaml"] = lampMutants + `
- name: a comment is added
  file: src/lamp.py
  find: 'def __init__(self):'
  replace: 'def __init__(self):  # mutated'

- name: a text that is gone
  file: src/lamp.py
  find: no such text
  replace: x

- name: a text found twice
  file: src/lamp.py
  find: raise ValueError("broken")
  replace: pass

- name: a file that is gone
  file: src/gone.py
  find: x
  replace: y
`
	mutants["mutation/mutate_tool.cmd"] = "sh -c \"echo -- 0 errors, 3 mutants\"\n"

	badMutants := pythonLamp(false)
	badMutants["mutation/mutants.yaml"] = `- name: no file
  find: x
  replace: y
- name: outside src
  file: spec/lamp/spine.yaml
  find: x
  replace: y
- name: outside src
  file: src/../spine.yaml
  find: x
  replace: y
- name: empty find
  file: src/lamp.py
  find: ''
  replace: y
- name: [not, text]
  file: src/lamp.py
  find: x
  replace: y
  extra: 1
- just a line
- name: fine but not run
  file: src/lamp.py
  find: def press(self)
  replace: def press(self, x=1)
`
	emptyMutants := pythonLamp(false)
	emptyMutants["mutation/mutants.yaml"] = "# none yet\n"

	return files{}.
		with("examples/python", pythonLamp(false)).
		with("examples/mutants", mutants).
		with("examples/badmutants", badMutants).
		with("examples/emptymutants", emptyMutants).
		with("examples/commands", commandLamp()).
		with("examples/failing", failing).
		with("examples/failingcommands", failingCommands).
		with("examples/pythonnames", pythonNames(false)).
		with("examples/badcommands", badCommands).
		with("examples/missing", files{"spec/lamp/spine.yaml": lampSpine}).
		with("examples/empty", files{"README.md": "Nothing here.\n"}).
		with("examples/unsound", files{
			"spec/lamp/spine.yaml": strings.Replace(lampSpine, "  - {from: broken, event: drop, reject: true}\n", "", 1),
			"checks/run.cmd":       "sh -c \"exit 0\"\n",
		})
}

const ownPolicy = `branches:
  "build/*": builder
  "*": person
roles:
  builder:
    allow: ["src/*"]
    deny: ["spec/*"]
  person:
    allow: ["*"]
`

// ---------- the runs ----------

type run struct {
	from    string // the folder it is started from, inside the project; empty for the project
	args    []string
	env     []string // NAME=value, added to the environment
	without string   // a program left off PATH, such as go
}

// project is a fixture project and the runs of ./verify in it, made one after another.
type project struct {
	name  string
	files files
	git   bool // a git repository: a commit tagged base, then a change to src/ and spec/
	runs  []run
}

func projects(tmp string) []project {
	r := func(args ...string) run { return run{args: args} }
	withEnv := func(env string, args ...string) run { return run{args: args, env: []string{env}} }
	return []project{
		{name: "examples", files: examplesProject(), runs: []run{
			r(), r("--full"),
			r("examples/python"), r("examples/python", "--full"),
			r("examples/commands", "--full"),
			r("examples/failing", "--full"),
			r("examples/failingcommands", "--full"),
			r("examples/pythonnames", "--full"),
			r("examples/badcommands", "--full"),
			r("examples/missing", "--full"),
			r("examples/empty", "--full"),
			r("examples/unsound", "--full"),
			r("examples/mutants", "--full"), r("examples/badmutants", "--full"),
			r("examples/emptymutants", "--full"),
			r("../outside"), r(filepath.Join(tmp, "outside"), "--full"),
			{from: "examples", args: []string{"python"}},
			withEnv("VERIFY_REPORTS=reports", "examples/python"),
			withEnv("VERIFY_VERBOSE=1", "examples/python", "--full"),
			r("--help"), r("--bogus"), r("examples/python", "examples/commands"), r("no-such-folder"),
		}},
		{name: "own", files: pythonLamp(false).with("", files{"policy.yaml": ownPolicy}), git: true, runs: []run{
			r(), r("."),
			withEnv("VERIFY_BASE=base"),
			{env: []string{"VERIFY_BASE=base", "VERIFY_BRANCH=main"}},
			withEnv("VERIFY_BASE=no-such-commit"),
			{env: []string{"VERIFY_BASE=base", "VERIFY_BRANCH=main", "VERIFY_REPORTS=reports"}},
			withEnv("VERIFY_REPORTS=" + filepath.Join(tmp, "reports")),
			withEnv("VERIFY_VERBOSE=1"),
			{without: "python3"},
		}},
		{name: "empty", files: files{}, runs: []run{r(), {without: "go"}}},
	}
}

// copyToolkit copies verify and tools/ into dir, as a project does: tools/ without its tests,
// its test inputs, its built programs and Python's caches.
func copyToolkit(t *testing.T, repo, dir string) {
	t.Helper()
	copyFile(t, filepath.Join(repo, "verify"), filepath.Join(dir, "verify"))
	tools := filepath.Join(repo, "tools")
	err := filepath.WalkDir(tools, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch {
		case d.IsDir() && (d.Name() == "testdata" || d.Name() == "bin" || d.Name() == "__pycache__"):
			return filepath.SkipDir
		case d.IsDir() || strings.HasSuffix(p, "_test.go"):
			return nil
		}
		rel, _ := filepath.Rel(repo, p)
		copyFile(t, p, filepath.Join(dir, rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(from)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, to, string(data), info.Mode().Perm())
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.com",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=build/lamp"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// makeProject writes the project; a git one gets a commit tagged base, then a commit changing
// src/ and spec/, on the branch build/lamp.
func makeProject(t *testing.T, repo, dir string, p project) {
	t.Helper()
	copyToolkit(t, repo, dir)
	for path, content := range p.files {
		writeFile(t, filepath.Join(dir, path), content, 0o644)
	}
	if !p.git {
		return
	}
	git(t, dir, "init", "-q")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "tag", "base")
	for _, path := range []string{"src/lamp.py", "spec/lamp/spine.yaml"} {
		writeFile(t, filepath.Join(dir, path), "# changed\n"+p.files[path], 0o644)
	}
	git(t, dir, "commit", "-q", "-a", "-m", "change")
}

// pathWithout is a folder holding a link to every program on PATH but the one named.
func pathWithout(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			link := filepath.Join(dir, e.Name())
			if e.Name() == name || e.IsDir() {
				continue
			}
			if _, err := os.Lstat(link); err == nil {
				continue // an earlier folder on PATH has it
			}
			if err := os.Symlink(filepath.Join(d, e.Name()), link); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

// environment is the test's own, without what ./verify reads, so that only a run's env sets
// it, and in the C locale, so that the programs it runs write their messages alike everywhere.
func environment() []string {
	env := []string{"LC_ALL=C"}
	for _, e := range os.Environ() {
		name, _, _ := strings.Cut(e, "=")
		if !strings.HasPrefix(name, "VERIFY_") && name != "LC_ALL" && name != "LANG" && name != "LANGUAGE" {
			env = append(env, e)
		}
	}
	return env
}

func execute(t *testing.T, dir string, r run, tmp string) record.Entry {
	t.Helper()
	cmd := exec.Command(invocation(r), r.args...)
	cmd.Dir = filepath.Join(dir, r.from)
	cmd.Env = append(environment(), r.env...)
	if r.without != "" {
		cmd.Env = append(cmd.Env, "PATH="+filepath.Join(tmp, "without-"+r.without))
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	e := record.Entry{}
	var exit *exec.ExitError
	switch err := cmd.Run(); {
	case errors.As(err, &exit) && exit.Exited():
		e.Exit = exit.ExitCode()
	case err != nil:
		t.Fatalf("./verify %s: %v", strings.Join(r.args, " "), err)
	}
	e.Sections = []record.Section{{Name: "stdout", Text: out.String()}, {Name: "stderr", Text: errOut.String()}}
	return e
}

// reports moves the JUnit reports a run wrote into its entry, one section each, by name.
func reports(t *testing.T, dir string, e *record.Entry) {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(dir, "*"))
	sort.Strings(paths)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		e.Sections = append(e.Sections, record.Section{Name: "report/" + filepath.Base(p), Text: string(data)})
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

// invocation is how the run names ./verify, from the folder it is started from.
func invocation(r run) string {
	if r.from == "" {
		return "./verify"
	}
	return strings.Repeat("../", strings.Count(r.from, "/")+1) + "verify"
}

// key names one run: its project, the folder it is started from, its arguments and its
// environment.
func key(project string, r run, normal func(string) string) string {
	parts := []string{project + ":"}
	if r.from != "" {
		parts = append(parts, "in", r.from+":")
	}
	parts = append(parts, invocation(r))
	for _, a := range r.args {
		parts = append(parts, record.Word(normal(a)))
	}
	env := append([]string(nil), r.env...)
	sort.Strings(env)
	for _, e := range env {
		parts = append(parts, "with", record.Word(normal(e)))
	}
	if r.without != "" {
		parts = append(parts, "with PATH without", r.without)
	}
	return strings.Join(parts, " ")
}

// ---------- the test ----------

func TestRecord(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if real, err := filepath.EvalSymlinks(tmp); err == nil {
		tmp = real // ./verify prints the folders it resolves
	}
	normal := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, tmp, "$TMP"), repo, "$ROOT")
	}
	rec := record.Load(t, filepath.Join("testdata", "record"), *update, "./verify", "go -C tools test ./internal/verify/ -update")

	for path, content := range pythonLamp(false) {
		writeFile(t, filepath.Join(tmp, "outside", path), content, 0o644)
	}
	for _, name := range []string{"go", "python3"} {
		pathWithout(t, filepath.Join(tmp, "without-"+name), name)
	}

	t.Run("projects", func(t *testing.T) {
		for _, p := range projects(tmp) {
			t.Run(p.name, func(t *testing.T) {
				t.Parallel()
				dir := filepath.Join(tmp, p.name)
				makeProject(t, repo, dir, p)
				for _, r := range p.runs {
					e := execute(t, dir, r, tmp)
					for _, env := range r.env {
						if v, ok := strings.CutPrefix(env, "VERIFY_REPORTS="); ok {
							if !filepath.IsAbs(v) {
								v = filepath.Join(dir, r.from, v)
							}
							reports(t, v, &e)
						}
					}
					for i := range e.Sections {
						e.Sections[i].Text = normal(e.Sections[i].Text)
					}
					k := key(p.name, r, normal)
					rec.Check(t, k, p.name, k, e)
				}
			})
		}
	})
	rec.Finish(t)
}
