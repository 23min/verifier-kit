package verify

// The decisions in plan.go, each tested without reading a file or starting a program (ADR 0007,
// decision 1). The record test runs them all again through ./verify.

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		want options
		err  string
	}{
		{nil, options{}, ""},
		{[]string{"--full", "examples/notes"}, options{full: true, root: "examples/notes"}, ""},
		{[]string{"--full", "--help", "--bogus"}, options{help: true}, ""},
		{[]string{"-h"}, options{help: true}, ""},
		{[]string{"--bogus", "--help"}, options{}, "unknown argument --bogus"},
		{[]string{"-"}, options{}, "unknown argument -"},
		{[]string{"a", "b"}, options{root: "a"}, "only one ROOT, got a and b"},
	} {
		got, err := parseArgs(c.args)
		if msg := errText(err); msg != c.err || (err == nil && got != c.want) {
			t.Errorf("parseArgs(%q) = %+v, %q; want %+v, %q", c.args, got, msg, c.want, c.err)
		}
	}
}

func TestPrefixAndRoots(t *testing.T) {
	for abs, want := range map[string]string{"/kit": "", "/kit/examples/notes": "examples/notes/", "/kitten": "/kitten/", "/elsewhere": "/elsewhere/"} {
		if got := prefix(abs, "/kit"); got != want {
			t.Errorf("prefix(%q) = %q, want %q", abs, got, want)
		}
	}
	for _, c := range []struct {
		hasSpec  bool
		examples []string
		want     []string
	}{
		{true, []string{"a"}, []string{""}},
		{false, nil, []string{""}},
		{false, []string{"a", "b"}, []string{"examples/a/", "examples/b/"}},
	} {
		if got := roots(c.hasSpec, c.examples); !reflect.DeepEqual(got, c.want) {
			t.Errorf("roots(%v, %q) = %q, want %q", c.hasSpec, c.examples, got, c.want)
		}
	}
}

func TestCommandLine(t *testing.T) {
	bad := func(why string) string { return finding("bad-command", "r/x.cmd", why) }
	for _, c := range []struct {
		data  string
		err   error
		line  string
		words []string
		bad   string
	}{
		{data: "env A=1 prog 'two words'\n", line: "env A=1 prog 'two words'", words: []string{"env", "A=1", "prog", "two words"}},
		{data: "prog\r\n", line: "prog", words: []string{"prog"}},
		{data: "prog", line: "prog", words: []string{"prog"}},
		{data: "prog\n\n", bad: bad("holds more than one line")},
		{data: "prog\r", bad: bad("holds more than one line")},
		{data: "   \n", bad: bad("holds no command")},
		{data: "", bad: bad("holds no command")},
		{data: "prog \"open\n", bad: bad("cannot be split: No closing quotation")},
		{data: "prog \\", bad: bad("cannot be split: No escaped character")},
		{data: "\xff\n", bad: bad("cannot be read: not UTF-8")},
		{err: errors.New("Permission denied"), bad: bad("cannot be read: Permission denied")},
	} {
		line, words, b := commandLine(cmdFile{shown: "r/x.cmd", data: []byte(c.data), err: c.err})
		if line != c.line || !reflect.DeepEqual(words, c.words) || b != c.bad {
			t.Errorf("commandLine(%q) = %q, %q, %q; want %q, %q, %q", c.data, line, words, b, c.line, c.words, c.bad)
		}
	}
}

func TestPlanRoot(t *testing.T) {
	c := config{spine: "/kit/tools/bin/spine", reports: "/reports", full: true}
	r := rootFacts{
		prefix: "examples/x/",
		machines: []machineFacts{
			{name: "lamp", behaviors: true, adapter: &cmdFile{shown: "examples/x/conformance/adapter_lamp.cmd", data: []byte("adapt 'a b'\n")}},
			{name: "door", checks: true},
		},
		mutations: []cmdFile{{shown: "examples/x/mutation/mutate_one.cmd", data: []byte("mutate\n")}},
	}
	want := []string{
		"unless sound: lamp: other steps need a sound spine",
		"lamp: spine is sound: /kit/tools/bin/spine check examples/x/spec/lamp/spine.yaml --junit /reports/examples-x-lamp-check.xml",
		"lamp: behaviors match spine: /kit/tools/bin/spine behaviors examples/x/spec/lamp/spine.yaml examples/x/spec/lamp/behaviors --junit /reports/examples-x-lamp-behaviors.xml",
		"lamp: checks cite current spine: skip no examples/x/checks/lamp/",
		"lamp: code conforms to spine: in examples/x/: /kit/tools/bin/spine conform spec/lamp/spine.yaml --adapter-cmd adapt 'a b' --junit /reports/examples-x-lamp-conform.xml",
		"unless sound: door: other steps need a sound spine",
		"door: spine is sound: /kit/tools/bin/spine check examples/x/spec/door/spine.yaml --junit /reports/examples-x-door-check.xml",
		"door: behaviors match spine: skip no examples/x/spec/door/behaviors/",
		"door: checks cite current spine: /kit/tools/bin/spine stale examples/x/spec/door/spine.yaml examples/x/checks/door --junit /reports/examples-x-door-stale.xml",
		"door: code conforms to spine: fails error   no-adapter         door: examples/x/conformance/adapter_door.cmd not found",
		"content checks: fails error   no-checks          examples/x/checks: no examples/x/checks/run.cmd",
		"mutants killed (mutate_one): in examples/x/: mutate",
	}
	if got := describe(planRoot(r, c)); !reflect.DeepEqual(got, want) {
		t.Errorf("planRoot gave\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	none := rootFacts{checks: &cmdFile{shown: "checks/run.cmd", data: []byte("run\n")}}
	want = []string{
		"spines found: fails error   no-machines        spec: no spec/*/spine.yaml",
		"content checks: in : run",
		"mutants killed: fails error   no-mutation        mutation: no mutation/mutate_*.cmd",
	}
	if got := describe(planRoot(none, config{spine: "/s", full: true})); !reflect.DeepEqual(got, want) {
		t.Errorf("planRoot without machines gave\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := describe(planRoot(none, config{spine: "/s"}))[2]; got != "mutants killed: skip fast mode; --full runs them" {
		t.Errorf("planRoot in fast mode gave %q", got)
	}

	g := guardStep("main", "src/a.py", config{spine: "/s"})
	if got := strings.Join(g.cmd.argv, " "); g.name != "guard (main)" || got != "/s guard policy.yaml --branch main" || *g.cmd.stdin != "src/a.py\n" {
		t.Errorf("guardStep gave %q: %s with %q", g.name, got, *g.cmd.stdin)
	}
}

// describe is a line per step of the groups, and one before a group that stops when its first
// step fails.
func describe(groups []group) []string {
	var out []string
	for _, g := range groups {
		if g.unsound != "" {
			out = append(out, "unless sound: "+g.unsound)
		}
		for _, s := range g.steps {
			switch {
			case s.skip != "":
				out = append(out, s.name+": skip "+s.skip)
			case s.lines != nil:
				out = append(out, s.name+": fails "+strings.Join(s.lines, " | "))
			case s.cmd.inRoot:
				out = append(out, s.name+": in "+s.cmd.dir+": "+strings.Join(s.cmd.argv, " "))
			default:
				out = append(out, s.name+": "+strings.Join(s.cmd.argv, " "))
			}
		}
	}
	return out
}

func TestVerdict(t *testing.T) {
	run := step{name: "content checks", cmd: &command{argv: []string{"prog"}, shown: "checks/run.cmd"}}
	spine := step{name: "lamp: spine is sound", cmd: &command{argv: []string{"/s"}}}
	output := "info    one\nerror   two\n-- 1 errors\n\n"
	for _, c := range []struct {
		s       step
		o       outcome
		verbose bool
		want    []string
	}{
		{run, outcome{ok: true, out: output}, false, []string{"ok    content checks"}},
		{run, outcome{ok: true, out: output}, true, []string{"ok    content checks", "      info    one", "      error   two", "      -- 1 errors"}},
		{run, outcome{out: output}, false, []string{"FAIL  content checks", "      error   two", "      -- 1 errors"}},
		{run, outcome{}, false, []string{"FAIL  content checks", "      "}},
		{run, outcome{startErr: "No such file or directory"}, false, []string{"FAIL  content checks", "      error   bad-command        checks/run.cmd: cannot run prog: No such file or directory"}},
		{spine, outcome{startErr: "Permission denied"}, false, []string{"FAIL  lamp: spine is sound", "      Permission denied"}},
		{step{name: "guard", skip: "VERIFY_BASE not set"}, outcome{}, true, []string{"skip  guard (VERIFY_BASE not set)"}},
		{step{name: "spines found", lines: []string{"error a", "error b"}}, outcome{}, false, []string{"FAIL  spines found", "      error a", "      error b"}},
	} {
		if got := verdict(c.s, c.o, c.verbose); !reflect.DeepEqual(got, c.want) {
			t.Errorf("verdict(%s, %+v, %v) = %q, want %q", c.s.name, c.o, c.verbose, got, c.want)
		}
	}
	for _, c := range []struct {
		s    step
		o    outcome
		want bool
	}{
		{run, outcome{ok: true}, true},
		{run, outcome{}, false},
		{run, outcome{ok: true, startErr: "x"}, false},
		{step{skip: "why"}, outcome{}, true},
		{step{lines: []string{"error"}}, outcome{ok: true}, false},
	} {
		if got := passed(c.s, c.o); got != c.want {
			t.Errorf("passed(%+v, %+v) = %v, want %v", c.s, c.o, got, c.want)
		}
	}
}

func TestSummary(t *testing.T) {
	if line, code := summary(nil); line != "-- all steps passed" || code != 0 {
		t.Errorf("summary(nil) = %q, %d", line, code)
	}
	if line, code := summary([]string{"a: x", "y"}); line != "-- 2 step(s) failed: a: x, y" || code != 1 {
		t.Errorf("summary = %q, %d", line, code)
	}
	if label("examples/x/") != "examples/x: " || label("") != "" {
		t.Errorf("label gave %q and %q", label("examples/x/"), label(""))
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
