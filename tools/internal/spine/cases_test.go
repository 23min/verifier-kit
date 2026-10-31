package spine

// The cases test (#27, #44): spine runs on every input below, and each run must give
// exactly what the record holds (record_test.go). Between them, the inputs must produce every
// finding code the tool
// contract lists. JSON output must parse, and a JUnit report must parse and refer to no character
// XML can't hold.
//
// A Python adapter is served through tools/adapter_shim.py, run by the python3 on PATH.

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/23min/verifier-kit/tools/internal/record"
	"github.com/23min/verifier-kit/tools/internal/shlex"
	"github.com/23min/verifier-kit/tools/internal/yamldoc"
)

var (
	spineBin string // the binary TestMain builds
	repoRoot string
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "spine-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	spineBin = filepath.Join(dir, "spine")
	build := exec.Command("go", "build", "-o", spineBin, "github.com/23min/verifier-kit/tools/cmd/spine")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building spine:", err)
		return 1
	}
	repoRoot, _ = filepath.Abs("../../..")
	return m.Run()
}

// ---------- running spine ----------

type result struct {
	exit   int
	stdout string
	stderr string
}

func execute(dir string, args []string, stdin string, env []string) (result, error) {
	cmd := exec.Command(spineBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	r := result{stdout: out.String(), stderr: errOut.String()}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.Exited():
		r.exit = exit.ExitCode()
	case err != nil:
		return r, err
	}
	return r, nil
}

// how a case is run: as given, in JSON with a JUnit report, or both.
type mode int

const (
	asGiven mode = 1 << iota
	asJSON
	full = asGiven | asJSON
)

type spineCase struct {
	name  string
	dir   string
	args  []string
	stdin string
	env   []string // NAME=value, added to the environment
	mode  mode
}

var formatCommands = map[string]bool{"check": true, "diff": true, "stale": true, "behaviors": true, "trace": true, "conform": true, "guard": true}

// codesSeen collects the finding codes spine gave, across all cases.
var codesSeen = struct {
	sync.Mutex
	m map[string]bool
}{m: map[string]bool{}}

type report struct {
	Tool     string    `json:"tool"`
	Command  string    `json:"command"`
	Findings []Finding `json:"findings"`
}

func (c spineCase) run(t *testing.T) {
	if c.mode&asJSON != 0 && len(c.args) > 0 && formatCommands[c.args[0]] {
		c.runJSON(t)
	}
	if c.mode&asGiven != 0 {
		c.runAsGiven(t)
	}
}

func (c spineCase) runAsGiven(t *testing.T) {
	r, err := execute(c.dir, c.args, c.stdin, c.env)
	if err != nil {
		t.Errorf("%s: %v", c.name, err)
		return
	}
	c.recordRun(t, c.args, r, nil)
}

// runJSON runs the case in JSON with a JUnit report, and gives the findings.
func (c spineCase) runJSON(t *testing.T) []Finding {
	path := filepath.Join(t.TempDir(), "report.xml")
	args := func(junit string) []string {
		return append([]string{c.args[0], "--format", "json", "--junit", junit}, c.args[1:]...)
	}
	r, err := execute(c.dir, args(path), c.stdin, c.env)
	if err != nil {
		t.Errorf("%s: %v", c.name, err)
		return nil
	}
	recorded := r
	recorded.stdout = strings.ReplaceAll(r.stdout, path, "REPORT")
	var junit *string
	data, readErr := os.ReadFile(path)
	if readErr == nil {
		s := strings.ReplaceAll(string(data), path, "REPORT")
		junit = &s
	}
	c.recordRun(t, args("REPORT"), recorded, junit)
	if r.exit == 2 { // a usage error: no JSON and no report
		return nil
	}
	shown := "spine " + quoteArgs(args("REPORT"))
	var rep report
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Errorf("%s: %s\noutput is not JSON: %v\n%s%s", c.name, shown, err, r.stdout, r.stderr)
		return nil
	}
	codesSeen.Lock()
	for _, f := range rep.Findings {
		codesSeen.m[f.Code] = true
	}
	codesSeen.Unlock()
	if readErr != nil {
		t.Errorf("%s: %s\nno JUnit report: %v", c.name, shown, readErr)
	} else if msg := checkJUnit(data); msg != "" {
		t.Errorf("%s: %s\n%s:\n%s", c.name, shown, msg, data)
	}
	return rep.Findings
}

type junitSuite struct {
	Name     string      `xml:"name,attr"`
	Tests    string      `xml:"tests,attr"`
	Failures string      `xml:"failures,attr"`
	Cases    []junitCase `xml:"testcase"`
}

type junitCase struct {
	Classname string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Failure   *junitFailure `xml:"failure"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// charRef is a character reference; encoding/xml accepts one to a character XML can't hold,
// which strict parsers refuse.
var charRef = regexp.MustCompile(`&#(x[0-9a-fA-F]+|[0-9]+);`)

// badCharRef is the first reference in data to a character XML can't hold, or "".
func badCharRef(data []byte) string {
	for _, m := range charRef.FindAllSubmatch(data, -1) {
		digits, base := string(m[1]), 10
		if digits[0] == 'x' {
			digits, base = digits[1:], 16
		}
		v, err := strconv.ParseUint(digits, base, 32)
		r := rune(v)
		if err != nil || !(r == 0x9 || r == 0xa || r == 0xd || r >= 0x20 && r <= 0xd7ff ||
			r >= 0xe000 && r <= 0xfffd || r >= 0x10000 && r <= 0x10ffff) {
			return string(m[0])
		}
	}
	return ""
}

// checkJUnit says what is wrong with a JUnit report, or is "".
func checkJUnit(data []byte) string {
	if ref := badCharRef(data); ref != "" {
		return "the report refers to a character XML can't hold: " + ref
	}
	var s junitSuite
	if err := xml.Unmarshal(data, &s); err != nil {
		return "the report doesn't parse: " + err.Error()
	}
	return ""
}

func quoteArgs(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\$*?[]") {
			parts[i] = fmt.Sprintf("%q", a)
		}
	}
	return strings.Join(parts, " ")
}

// ---------- the inputs ----------

func testdata(t *testing.T) string {
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func files(t *testing.T, dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

// argLines reads a file of command lines, one JSON array per line, or an object
// {"args": [...], "stdin": "..."}; blank lines and lines starting with # are skipped.
func argLines(t *testing.T, path string) []spineCase {
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []spineCase
	for i, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var c spineCase
		if strings.HasPrefix(line, "{") {
			var obj struct {
				Args  []string          `json:"args"`
				Stdin string            `json:"stdin"`
				Env   map[string]string `json:"env"`
			}
			err = json.Unmarshal([]byte(line), &obj)
			c.args, c.stdin = obj.Args, obj.Stdin
			for k, v := range obj.Env {
				c.env = append(c.env, k+"="+v)
			}
		} else {
			err = json.Unmarshal([]byte(line), &c.args)
		}
		if err != nil {
			t.Fatalf("%s:%d: %v", path, i+1, err)
		}
		if c.args == nil {
			c.args = []string{}
		}
		c.name = fmt.Sprintf("%s:%d", filepath.Base(path), i+1)
		out = append(out, c)
	}
	return out
}

// yamlText is a YAML double-quoted string: escapes only for what YAML can't hold raw there.
func yamlText(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || r == 0xfeff || r == 0xfffe || r == 0xffff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ynode is a seed file as a tree that can be written back as flow YAML with one change.
type ynode struct {
	kind  string // "map", "list", "text", "bool", "null"
	keys  []*ynode
	items []*ynode // a map's values, or a list's items
	text  string
	b     bool
}

func toNode(v *yamldoc.Value) *ynode {
	switch v.Kind {
	case yamldoc.KMap:
		n := &ynode{kind: "map"}
		for i, k := range v.Map.Keys {
			n.keys = append(n.keys, toNode(k))
			n.items = append(n.items, toNode(v.Map.Values[i]))
		}
		return n
	case yamldoc.KList:
		n := &ynode{kind: "list"}
		for _, x := range v.Items {
			n.items = append(n.items, toNode(x))
		}
		return n
	case yamldoc.KStr:
		return &ynode{kind: "text", text: v.Str}
	case yamldoc.KBool:
		return &ynode{kind: "bool", b: v.Bool}
	case yamldoc.KNull:
		return &ynode{kind: "null"}
	}
	panic(fmt.Sprintf("a seed holds a value of kind %d", v.Kind))
}

func (n *ynode) all() []*ynode {
	out := []*ynode{n}
	for _, x := range n.items {
		out = append(out, x.all()...)
	}
	return out
}

// emit writes n as flow YAML, with replace standing in for the ynode target, or with the key
// extra (value "x") added to the mapping target.
func (n *ynode) emit(target *ynode, replace, extra string) string {
	if n == target && replace != "" {
		return replace
	}
	switch n.kind {
	case "map":
		var parts []string
		for i, k := range n.keys {
			parts = append(parts, k.emit(target, replace, extra)+": "+n.items[i].emit(target, replace, extra))
		}
		if n == target && extra != "" {
			parts = append(parts, extra+`: "x"`)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case "list":
		var parts []string
		for _, x := range n.items {
			parts = append(parts, x.emit(target, replace, extra))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case "text":
		return yamlText(n.text)
	case "bool":
		if n.b {
			return "true"
		}
		return "false"
	}
	return "null"
}

var (
	oddValues = []string{`null`, `[]`, `["a"]`, `{}`, `{"a": 1}`, `true`, `false`, `3`, `1.5`, `""`, `"x y"`, `2026-10-14`}
	oddKeys   = []string{`true`, `3`, `null`, `1.5`, `2026-10-14`, `"zz"`}
)

// variants writes every copy of the seed with one value replaced or one odd key added, and
// gives their file names.
func variants(t *testing.T, seed, dir string) []string {
	data, err := os.ReadFile(seed)
	if err != nil {
		t.Fatal(err)
	}
	v, err := yamldoc.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	root := toNode(v)
	var names []string
	write := func(text string) {
		name := fmt.Sprintf("v%04d.yaml", len(names)+1)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	for _, n := range root.all() {
		if n != root {
			for _, odd := range oddValues {
				write(root.emit(n, odd, ""))
			}
		}
		if n.kind == "map" {
			for _, k := range oddKeys {
				write(root.emit(n, "", k))
			}
		}
	}
	return names
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// conditions is conditions.json, inside the language and just outside it; every pairing of a few
// operands; and random token runs.
func conditions(t *testing.T, td string) []string {
	var out []string
	data, err := os.ReadFile(filepath.Join(td, "conditions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	left := []string{"role", "n", "'owner'", "'own'", "['owner', 'x']", "[]", "'o'"}
	right := []string{"role", "n", "'owner'", "'own'", "['owner', 'x']", "[]", "'ow'", "1"}
	for _, a := range left {
		for _, op := range []string{"==", "!=", "in", "not in"} {
			for _, b := range right {
				out = append(out, a+" "+op+" "+b, "not "+a+" "+op+" "+b+" or "+b+" "+op+" "+a)
			}
		}
	}
	vocab := []string{"role", "n", "x", "'owner'", `"own"`, "'o'", "''", "1", "[", "]", "(", ")", ",", "==", "!=",
		"in", "not", "and", "or", "<", "-", ".", "#", "\\\n", "\n", "  ", "\t", "=", ";", "!", "$", "upper", `'a\b'`}
	rnd := rand.New(rand.NewPCG(27, 7))
	for range 2000 {
		words := make([]string, 1+rnd.IntN(7))
		for i := range words {
			words[i] = vocab[rnd.IntN(len(vocab))]
		}
		out = append(out, strings.Join(words, " "))
	}
	return out
}

// conditionSpine has one event per condition, each with one rule to a terminal state, so check
// reports the cases a condition doesn't cover: its truth table.
func conditionSpine(conds []string) string {
	var events, rules []string
	for i, c := range conds {
		e := fmt.Sprintf(`"e%d"`, i+1)
		events = append(events, e)
		rules = append(rules, fmt.Sprintf(`{"from": "s", "event": %s, "when": %s, "to": "t"}`, e, yamlText(c)))
	}
	return `{"machine": "c", "vars": {"role": ["owner", "editor", "own"], "n": ["1", "2"]}, ` +
		`"states": {"s": {}, "t": {"terminal": true}}, "initial": "s", ` +
		`"events": [` + strings.Join(events, ", ") + `], "rules": [` + strings.Join(rules, ", ") + "]}\n"
}

// valueSpine puts one YAML spelling where a state name, a rule's from and to, and an event go.
func valueSpine(spelling string) string {
	return "machine: m\nstates:\n  " + spelling + ": {}\n  b: {}\ninitial: b\nevents:\n  - go\n  - " + spelling +
		"\nrules:\n  - from: b\n    event: go\n    to: " + spelling + "\n  - from: " + spelling + "\n    event: " +
		spelling + "\n    stay: true\n"
}

// filesystemCases builds the inputs git can't hold: links, unreadable files, and a compiled file
// in __pycache__, which is binary and which stale doesn't read.
func filesystemCases(t *testing.T, td, dir string) []spineCase {
	spine := filepath.Join(td, "seeds", "spine.yaml")
	link := func(target, name string) {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	unreadable := func(name string) {
		writeFile(t, filepath.Join(dir, name), "steps: []\n# covers: closed/open@26ccaba33ea0\n")
		if err := os.Chmod(filepath.Join(dir, name), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(filepath.Join(dir, name), 0o644) })
	}
	writeFile(t, filepath.Join(dir, "b/valid/a.yaml"), "steps: [\"open role=owner\"]\n")
	writeFile(t, filepath.Join(dir, "b/valid/.hidden.yaml"), "steps: []\n")
	writeFile(t, filepath.Join(dir, "b/valid/new\nline.yaml"), "steps: []\n")
	writeFile(t, filepath.Join(dir, "b/valid/b.YAML"), "steps: []\n")
	writeFile(t, filepath.Join(dir, "b/valid/c.y.ml"), "steps: []\n")
	writeFile(t, filepath.Join(dir, "b/valid/dir.yaml/x"), "")
	writeFile(t, filepath.Join(dir, "b/invalid/.yml"), "steps: [\"open role=guest\"]\n")
	link("nowhere", "b/valid/broken.yaml")
	link("a.yaml", "b/valid/link.yaml")
	unreadable("b/invalid/locked.yaml")

	writeFile(t, filepath.Join(dir, "c/a.py"), "# covers: closed/open@26ccaba33ea0\n")
	writeFile(t, filepath.Join(dir, "c/__pycache__/a.pyc"), "\x00\x0d\x0d\x0a# covers: closed/open@000000000000\n")
	writeFile(t, filepath.Join(dir, "c/b-c/x.py"), "# covers: open/open@000000000001\n")
	writeFile(t, filepath.Join(dir, "c/b/x.py"), "# covers: open/open@000000000002\n")
	writeFile(t, filepath.Join(dir, "c/latin1.py"), "# caf\xe9 covers: open/remove@6c6a6ee2ab96 \xff\xfe covers: x\xe9/y@1\n")
	writeFile(t, filepath.Join(dir, "elsewhere/z.py"), "# covers: open/open@000000000003\n")
	link("../elsewhere", "c/linked-dir")
	link("../elsewhere/z.py", "c/linked-file.py")
	link("nowhere", "c/broken.py")
	unreadable("c/locked.py")
	if err := os.Mkdir(filepath.Join(dir, "c/closed"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(dir, "c/closed"), 0o755) })
	unreadable("locked.yaml")
	if err := os.Mkdir(filepath.Join(dir, "dir.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	cases := [][]string{
		{"behaviors", spine, "b"},
		{"trace", spine, "b/valid/link.yaml"},
		{"trace", spine, "b/valid/broken.yaml"},
		{"trace", spine, "b/valid/dir.yaml"},
		{"trace", spine, "b/invalid/locked.yaml"},
		{"stale", spine, "c"},
		{"stale", spine, "c/"},
		{"stale", spine, "./c/../c"},
		{"stale", spine, "c/a.py"},
		{"check", "locked.yaml"},
		{"check", "dir.yaml"},
		{"check", "dir.yaml/"},
		{"check", filepath.Join(td, "seeds", "spine.yaml") + "/"},
		{"hash", "locked.yaml"},
	}
	var out []spineCase
	for i, args := range cases {
		out = append(out, spineCase{name: fmt.Sprintf("filesystem %d", i+1), dir: dir, args: args, mode: full})
	}
	// changed files whose names aren't UTF-8
	out = append(out, spineCase{name: "stdin not UTF-8", dir: dir, mode: full, stdin: "spec/\xff.py\nsrc/\xfe\n",
		args: []string{"guard", filepath.Join(td, "seeds", "policy.yaml"), "--branch", "build/x"}})
	return out
}

// exampleCases runs every command on both examples as they are: spines, readings, behaviors,
// checks and the policy, and diff on every ordered pair of an example's spine and readings
// (demo/faults.sh's scenario 5 among them) and on the two spines.
func exampleCases(t *testing.T) []spineCase {
	var out []spineCase
	add := func(m mode, args ...string) {
		out = append(out, spineCase{name: "example " + quoteArgs(args), dir: repoRoot, args: args, mode: m})
	}
	specs, err := filepath.Glob(filepath.Join(repoRoot, "examples", "*", "spec", "*"))
	if err != nil || len(specs) < 2 {
		t.Fatalf("found %d example machines: %v", len(specs), err)
	}
	for _, dir := range specs {
		rel, _ := filepath.Rel(repoRoot, dir)
		machine := filepath.Base(dir)
		root := filepath.Dir(filepath.Dir(rel))
		spine := filepath.Join(rel, "spine.yaml")
		readings, _ := filepath.Glob(filepath.Join(dir, "readings", "*.yaml"))
		var all []string
		for _, s := range append([]string{filepath.Join(dir, "spine.yaml")}, readings...) {
			r, _ := filepath.Rel(repoRoot, s)
			all = append(all, r)
			add(full, "check", r)
			add(asGiven, "hash", r)
		}
		for _, a := range all {
			for _, b := range all {
				add(full, "diff", a, b)
			}
		}
		add(full, "behaviors", spine, filepath.Join(rel, "behaviors"))
		add(full, "stale", spine, filepath.Join(root, "checks", machine))
		behaviors, _ := filepath.Glob(filepath.Join(dir, "behaviors", "*", "*.yaml"))
		for _, b := range behaviors {
			r, _ := filepath.Rel(repoRoot, b)
			add(full, "trace", spine, r)
		}
		add(full, "guard", "policy.yaml", "--branch", "build/x", filepath.Join(root, "src", "a.py"), spine)
		// conform as the example's line runs it, with its src/ importable, through the shim
		adapter := filepath.Join(root, "conformance", "adapter_"+machine+".py")
		env := []string{"PYTHONPATH=" + filepath.Join(root, "src")}
		out = append(out, spineCase{name: "example conform " + machine, dir: repoRoot, env: env, mode: full,
			args: []string{"conform", spine, "--adapter-cmd", "python3 tools/adapter_shim.py " + adapter}})
	}
	add(full, "diff", "examples/notes/spec/notes/spine.yaml", "examples/ref_sync/spec/ref_sync/spine.yaml")
	return out
}

// faultCases seeds the faults demo/faults.sh seeds that spine's commands see, the same way,
// into copies of the notes spine and service: scenario 1 is the service as first written, 2
// loosens restore, 3 writes `on:` for `event:`, 4 is a builder's branch that also edits the
// spine, and 9 imports a module that doesn't exist.
func faultCases(t *testing.T, dir string) []spineCase {
	notes := filepath.Join(repoRoot, "examples", "notes")
	data, err := os.ReadFile(filepath.Join(notes, "spec", "notes", "spine.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	spine := string(data)
	replace := func(s, old, new string, n int) string {
		if !strings.Contains(s, old) {
			t.Fatalf("the notes spine no longer has %q, which demo/faults.sh replaces", old)
		}
		return strings.Replace(s, old, new, n)
	}
	loosened := replace(spine, `{from: trashed, event: restore, when: "role == 'owner'", to: active}`,
		`{from: trashed, event: restore, when: "role != 'viewer'", to: active}`, -1)
	loosened = replace(loosened, `{from: trashed, event: restore, when: "role != 'owner'", reject: true}`,
		`{from: trashed, event: restore, when: "role == 'viewer'", reject: true}`, -1)
	writeFile(t, filepath.Join(dir, "2", "spine.yaml"), loosened)
	writeFile(t, filepath.Join(dir, "3", "spine.yaml"), replace(spine, "event: publish", "on: publish", 1))
	writeFile(t, filepath.Join(dir, "4", "spine.yaml"), spine+"# and here\n")
	var out []spineCase
	add := func(name string, args ...string) {
		out = append(out, spineCase{name: name, dir: dir, args: args, mode: full})
	}
	behaviors, checks := filepath.Join(notes, "spec", "notes", "behaviors"), filepath.Join(notes, "checks", "notes")
	add("fault 2 check", "check", "2/spine.yaml")
	add("fault 2 behaviors", "behaviors", "2/spine.yaml", behaviors)
	add("fault 2 stale", "stale", "2/spine.yaml", checks)
	add("fault 3 check", "check", "3/spine.yaml")
	add("fault 3 behaviors", "behaviors", "3/spine.yaml", behaviors)
	add("fault 4 check", "check", "4/spine.yaml")
	add("fault 4 guard", "guard", filepath.Join(repoRoot, "policy.yaml"), "--branch", "build/purge-confirmation",
		"examples/notes/src/notes_app.py", "examples/notes/spec/notes/spine.yaml")

	// conform: 1 and 10 run the service as first written, 2 the loosened spine, 9 a service
	// that imports a module that doesn't exist; each with its own copy of src/
	service, err := os.ReadFile(filepath.Join(notes, "src", "notes_app.py"))
	if err != nil {
		t.Fatal(err)
	}
	asWritten, err := os.ReadFile(filepath.Join(notes, "demo", "notes_app_as_written.py"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "1", "src", "notes_app.py"), string(asWritten))
	writeFile(t, filepath.Join(dir, "2", "src", "notes_app.py"), string(service))
	writeFile(t, filepath.Join(dir, "9", "src", "notes_app.py"), string(service)+"import no_such_module\n")
	spineFile, adapter := filepath.Join(notes, "spec", "notes", "spine.yaml"), filepath.Join(notes, "conformance", "adapter_notes.py")
	line := "python3 " + shlex.Quote(filepath.Join(repoRoot, "tools", "adapter_shim.py")) + " " + shlex.Quote(adapter)
	conform := func(name, src, spine string) {
		env := []string{"PYTHONPATH=" + filepath.Join(dir, src, "src")}
		out = append(out, spineCase{name: name, dir: dir, args: []string{"conform", spine, "--adapter-cmd", line}, env: env, mode: full})
	}
	conform("fault 1 conform", "1", spineFile)
	conform("fault 2 conform", "2", "2/spine.yaml")
	conform("fault 9 conform", "9", spineFile)
	return out
}

// contractCodes is every finding code the tool contract lists for spine's commands.
func contractCodes(t *testing.T) map[string]bool {
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "reference", "tool-contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, table, found := strings.Cut(string(data), "### Finding codes")
	if !found {
		t.Fatal("tool-contract.md has no finding codes table")
	}
	table, _, _ = strings.Cut(table, "\n## ")
	spineCommands := map[string]bool{"all": true, "check": true, "hash": true, "diff": true, "stale": true, "behaviors": true, "trace": true, "conform": true, "guard": true}
	code := regexp.MustCompile("`([a-z-]+)`")
	codes := map[string]bool{}
	for _, line := range strings.Split(table, "\n") {
		cols := strings.Split(line, "|")
		if len(cols) < 5 || !strings.HasPrefix(strings.TrimSpace(cols[1]), "`") {
			continue
		}
		if spineCommands[strings.Trim(strings.TrimSpace(cols[3]), "`")] {
			for _, m := range code.FindAllStringSubmatch(cols[1], -1) {
				codes[m[1]] = true
			}
		}
	}
	if len(codes) < 20 {
		t.Fatalf("read only %d codes from the finding codes table", len(codes))
	}
	return codes
}

// ---------- the test ----------

func TestCases(t *testing.T) {
	td := testdata(t)
	tmp := t.TempDir()
	loadRecord(t, td, tmp)
	var cases []spineCase
	add := func(name, dir string, m mode, args ...string) {
		cases = append(cases, spineCase{name: name, dir: dir, args: args, mode: m})
	}

	for _, sub := range []string{"spines", "seeds"} {
		for _, f := range files(t, filepath.Join(td, sub)) {
			if sub == "seeds" && f != "spine.yaml" {
				continue
			}
			p := filepath.Join(sub, f)
			add(p, td, full, "check", p)
			add(p, td, asGiven, "hash", p)
		}
	}
	for _, f := range files(t, filepath.Join(td, "yaml")) {
		add("yaml/"+f, td, asJSON, "check", filepath.Join("yaml", f))
	}
	for _, c := range argLines(t, filepath.Join(td, "cases.txt")) {
		c.dir, c.mode = td, full
		cases = append(cases, c)
	}
	for _, c := range argLines(t, filepath.Join(td, "cli.txt")) {
		c.dir, c.mode = td, asGiven
		cases = append(cases, c)
	}

	seed := filepath.Join(td, "seeds")
	for _, kind := range []string{"spine", "behavior", "policy"} {
		dir := filepath.Join(tmp, kind)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range variants(t, filepath.Join(seed, kind+".yaml"), dir) {
			switch kind {
			case "spine":
				add("spine variant "+name, dir, asJSON, "check", name)
			case "behavior":
				add("behavior variant "+name, dir, asJSON, "trace", filepath.Join(seed, "spine.yaml"), name)
			case "policy":
				add("policy variant "+name, dir, asJSON, "guard", name, "--branch", "build/x", "src/a.py", "src/secret.py", "spec/b.yaml")
			}
		}
	}

	values, err := os.ReadFile(filepath.Join(td, "values.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for i, spelling := range strings.Split(strings.TrimSuffix(string(values), "\n"), "\n") {
		dir := filepath.Join(tmp, fmt.Sprintf("value%03d", i+1))
		writeFile(t, filepath.Join(dir, "spine.yaml"), valueSpine(spelling))
		name := fmt.Sprintf("value %d %q", i+1, spelling)
		add(name, dir, asJSON, "check", "spine.yaml")
		add(name, dir, asGiven, "hash", "spine.yaml")
	}

	cases = append(cases, filesystemCases(t, td, filepath.Join(tmp, "fs"))...)
	cases = append(cases, exampleCases(t)...)
	cases = append(cases, faultCases(t, filepath.Join(tmp, "faults"))...)

	conds := conditions(t, td)
	var batches [][]string
	for len(conds) > 0 {
		n := min(100, len(conds))
		batches, conds = append(batches, conds[:n]), conds[n:]
	}

	t.Run("cases", func(t *testing.T) {
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				t.Parallel()
				c.run(t)
			})
		}
		for i, batch := range batches {
			t.Run(fmt.Sprintf("conditions %d", i+1), func(t *testing.T) {
				t.Parallel()
				conditionBatch(t, filepath.Join(tmp, fmt.Sprintf("conditions%02d", i+1)), batch)
			})
		}
	})
	rec.Finish(t)

	want := contractCodes(t)
	var missing []string
	for c := range want {
		if !codesSeen.m[c] {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 && !record.Filtered() { // only some cases ran
		t.Errorf("no input produces these finding codes from the contract: %s", strings.Join(missing, ", "))
	}
	t.Logf("%d cases and %d condition batches; %d of the contract's %d codes seen", len(cases), len(batches), len(want)-len(missing), len(want))
}

// conditionBatch compares one batch: first as written, where check reports the conditions
// outside the language; then the allowed ones alone, where check reports their truth tables.
func conditionBatch(t *testing.T, dir string, conds []string) {
	writeFile(t, filepath.Join(dir, "all.yaml"), conditionSpine(conds))
	c := spineCase{name: filepath.Base(dir), dir: dir, args: []string{"check", "all.yaml"}, mode: asJSON}
	findings := c.runJSON(t)
	if findings == nil {
		return
	}
	bad := map[string]bool{}
	for _, f := range findings {
		bad[f.Where] = true
	}
	var allowed []string
	for i, cond := range conds {
		if !bad[fmt.Sprintf("rules[%d]", i+1)] {
			allowed = append(allowed, cond)
		}
	}
	if len(allowed) == 0 {
		return
	}
	writeFile(t, filepath.Join(dir, "allowed.yaml"), conditionSpine(allowed))
	spineCase{name: filepath.Base(dir) + " allowed", dir: dir, args: []string{"check", "allowed.yaml"}, mode: asJSON}.runJSON(t)
}
