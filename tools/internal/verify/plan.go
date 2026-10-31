package verify

// What verify decides, without reading a file or starting a program: its arguments, the roots,
// each root's steps and what each step prints. verify.go reads the files and starts the
// programs these functions are given and planned (ADR 0007, decision 1).

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/23min/verifier-kit/tools/internal/finding"
	"github.com/23min/verifier-kit/tools/internal/shlex"
)

// options is what the arguments ask for.
type options struct {
	help bool
	full bool
	root string // the ROOT given, or empty
}

// parseArgs reads the arguments in order; the first --help wins over the ones after it. An
// error is a misuse.
func parseArgs(args []string) (options, error) {
	var o options
	for _, a := range args {
		switch {
		case a == "--full":
			o.full = true
		case a == "-h" || a == "--help":
			return options{help: true}, nil
		case strings.HasPrefix(a, "-"):
			return o, fmt.Errorf("unknown argument %s", a)
		case o.root != "":
			return o, fmt.Errorf("only one ROOT, got %s and %s", o.root, a)
		default:
			o.root = a
		}
	}
	return o, nil
}

// prefix is what goes in front of spec/, checks/ ... for the root at abs: nothing for the
// toolkit's folder here, a relative path for a folder inside it, the absolute path otherwise.
func prefix(abs, here string) string {
	switch rel, inside := strings.CutPrefix(abs, here+"/"); {
	case abs == here:
		return ""
	case inside:
		return rel + "/"
	default:
		return abs + "/"
	}
}

// roots are the prefixes of the roots to check when no ROOT is given: the toolkit's folder when
// it has spec/ or no examples, otherwise every examples/<name>/ in the order given.
func roots(hasSpec bool, examples []string) []string {
	if hasSpec || len(examples) == 0 {
		return []string{""}
	}
	out := make([]string, len(examples))
	for i, name := range examples {
		out[i] = "examples/" + name + "/"
	}
	return out
}

// pieceFile is a piece's file as read, a .cmd file or mutants.yaml: its content, or why it
// couldn't be read.
type pieceFile struct {
	shown string // its path as lines name it, the root's prefix in front
	data  []byte
	err   error
}

// machineFacts is what a root holds for one spec/<machine>/spine.yaml.
type machineFacts struct {
	name      string
	behaviors bool       // spec/<machine>/behaviors/ is a folder
	checks    bool       // checks/<machine>/ is a folder
	adapter   *pieceFile // conformance/adapter_<machine>.cmd, or nil when there is none
}

// rootFacts is what a root holds, as far as its steps depend on it.
type rootFacts struct {
	prefix    string
	machines  []machineFacts
	checks    *pieceFile  // checks/run.cmd, or nil when there is none
	mutants   *pieceFile  // mutation/mutants.yaml, or nil when there is none
	mutations []pieceFile // every mutation/mutate_*.cmd, by name
}

// config is what every root's steps share.
type config struct {
	spine   string // the toolkit's spine, an absolute path
	reports string // the folder for JUnit reports, an absolute path, or empty for none
	full    bool
}

// junit is the arguments that make a spine step write its JUnit report, when reports are wanted.
func (c config) junit(name string) []string {
	if c.reports == "" {
		return nil
	}
	return []string{"--junit", c.reports + "/" + name + ".xml"}
}

// command is a program a step runs.
type command struct {
	argv   []string
	dir    string // the folder it runs from: a root's prefix, empty for the toolkit's folder
	inRoot bool   // with tools/ first on PATH and VERIFY_SPINE set, as a root's commands run
	stdin  *string
	shown  string // the .cmd file the command came from, or empty for spine
}

// step is one line of the run, and the lines under it. Exactly one of skip, lines, cmd and
// mutate is set: a step that doesn't run, a step that fails without running, a program to run,
// or the mutants in mutants.yaml to make and judge.
type step struct {
	name   string
	skip   string
	lines  []string
	cmd    *command
	mutate *mutation
}

// group is steps run in turn. When the first fails and unsound is set, the rest don't run and
// unsound is printed as a skip line instead.
type group struct {
	steps   []step
	unsound string
}

// errorLine is an error line as the tools print it, without the indent.
func errorLine(code, where, message string) string {
	return finding.Line("error", code, where, message)
}

// guardStep runs guard over the changed files, given as git diff --name-only printed them.
func guardStep(branch, changed string, c config) step {
	stdin := changed + "\n"
	argv := append([]string{c.spine, "guard", "policy.yaml", "--branch", branch}, c.junit("guard")...)
	return step{name: "guard (" + branch + ")", cmd: &command{argv: argv, stdin: &stdin}}
}

// planRoot is every step for one root.
func planRoot(r rootFacts, c config) []group {
	p := r.prefix
	jp := strings.ReplaceAll(p, "/", "-")
	spineStep := func(name, sub string, args ...string) step {
		argv := append(append([]string{c.spine, sub}, args...), c.junit(jp+name)...)
		return step{cmd: &command{argv: argv}}
	}
	var out []group
	for _, m := range r.machines {
		spec := p + "spec/" + m.name + "/spine.yaml"
		g := group{unsound: m.name + ": other steps need a sound spine"}
		sound := spineStep(m.name+"-check", "check", spec)
		sound.name = m.name + ": spine is sound"
		g.steps = append(g.steps, sound)

		behaviors := step{name: m.name + ": behaviors match spine", skip: "no " + p + "spec/" + m.name + "/behaviors/"}
		if m.behaviors {
			behaviors = spineStep(m.name+"-behaviors", "behaviors", spec, p+"spec/"+m.name+"/behaviors")
			behaviors.name = m.name + ": behaviors match spine"
		}
		stale := step{name: m.name + ": checks cite current spine", skip: "no " + p + "checks/" + m.name + "/"}
		if m.checks {
			stale = spineStep(m.name+"-stale", "stale", spec, p+"checks/"+m.name)
			stale.name = m.name + ": checks cite current spine"
		}

		conform := conformStep(m, p, c.spine, c.junit(jp+m.name+"-conform"))
		g.steps = append(g.steps, behaviors, stale, conform)
		out = append(out, g)
	}
	if len(r.machines) == 0 {
		out = append(out, group{steps: []step{{name: "spines found", lines: []string{errorLine("no-machines", p+"spec", "no "+p+"spec/*/spine.yaml")}}}})
	}

	out = append(out, group{steps: []step{checksStep(r.checks, p)}})

	switch {
	case !c.full:
		out = append(out, group{steps: []step{{name: "mutants killed", skip: "fast mode; --full runs them"}}})
	case r.mutants == nil && len(r.mutations) == 0:
		out = append(out, group{steps: []step{{name: "mutants killed", lines: []string{errorLine("no-mutation", p+"mutation", "no "+p+"mutation/mutants.yaml or mutate_*.cmd")}}}})
	default:
		if r.mutants != nil {
			out = append(out, group{steps: []step{mutantsStep(r, c.spine)}})
		}
		for _, f := range r.mutations {
			name := strings.TrimSuffix(path.Base(f.shown), ".cmd")
			out = append(out, group{steps: []step{rootCommand("mutants killed ("+name+")", f, p)}})
		}
	}
	return out
}

// conformStep runs conform on a machine with its adapter's line, from the root at prefix p.
func conformStep(m machineFacts, p, spine string, junit []string) step {
	s := step{name: m.name + ": code conforms to spine"}
	switch {
	case m.adapter == nil:
		s.lines = []string{errorLine("no-adapter", m.name, p+"conformance/adapter_"+m.name+".cmd not found")}
	default:
		if line, _, bad := commandLine(*m.adapter); bad != "" {
			s.lines = []string{bad}
		} else {
			argv := append([]string{spine, "conform", "spec/" + m.name + "/spine.yaml", "--adapter-cmd", line}, junit...)
			s.cmd = &command{argv: argv, dir: p, inRoot: true}
		}
	}
	return s
}

// checksStep runs the content checks, from the root at prefix p.
func checksStep(f *pieceFile, p string) step {
	if f == nil {
		return step{name: "content checks", lines: []string{errorLine("no-checks", p+"checks", "no "+p+"checks/run.cmd")}}
	}
	return rootCommand("content checks", *f, p)
}

// mutantsStep makes and judges the mutants in mutants.yaml. Each copy runs the steps that read
// src/, the machines' conform and the content checks, without JUnit reports.
func mutantsStep(r rootFacts, spine string) step {
	s := step{name: "mutants killed (mutants.yaml)"}
	mutants, problems := readMutants(*r.mutants)
	if problems != nil {
		s.lines = problems
		return s
	}
	var oracle []step
	for _, m := range r.machines {
		oracle = append(oracle, conformStep(m, r.prefix, spine, nil))
	}
	oracle = append(oracle, checksStep(r.checks, r.prefix))
	s.mutate = &mutation{prefix: r.prefix, shown: r.mutants.shown, mutants: mutants, oracle: oracle}
	return s
}

// rootCommand is a step that runs a .cmd file's command from the root.
func rootCommand(name string, f pieceFile, p string) step {
	_, words, bad := commandLine(f)
	if bad != "" {
		return step{name: name, lines: []string{bad}}
	}
	return step{name: name, cmd: &command{argv: words, dir: p, inRoot: true, shown: f.shown}}
}

// commandLine reads a .cmd file as spine conform reads --adapter-cmd: one line, with one line
// ending after it at most, split by shell quoting rules. bad is the bad-command error line when
// the file holds no single command line.
func commandLine(f pieceFile) (line string, words []string, bad string) {
	badCommand := func(why string) (string, []string, string) {
		return "", nil, errorLine("bad-command", f.shown, why)
	}
	if f.err != nil {
		return badCommand("cannot be read: " + f.err.Error())
	}
	if !utf8.Valid(f.data) {
		return badCommand("cannot be read: not UTF-8")
	}
	text := string(f.data)
	line, found := strings.CutSuffix(text, "\r\n")
	if !found {
		line = strings.TrimSuffix(text, "\n")
	}
	if strings.ContainsAny(line, "\r\n") {
		return badCommand("holds more than one line")
	}
	words, err := shlex.Split(line)
	if err != nil {
		return badCommand("cannot be split: " + err.Error())
	}
	if len(words) == 0 {
		return badCommand("the command is empty")
	}
	return line, words, ""
}

// outcome is how a step's program ended.
type outcome struct {
	startErr string // why it couldn't be started, or empty
	ok       bool   // it exited 0
	out      string // its standard output and standard error, as written
}

// verdict is what a step prints: its line, then the lines under it, indented. A failing step
// shows its output without info lines; a passing one shows its output only when verbose.
func verdict(s step, o outcome, verbose bool) []string {
	switch {
	case s.skip != "":
		return []string{"skip  " + s.name + " (" + s.skip + ")"}
	case s.lines != nil:
		return failed(s.name, s.lines)
	case o.startErr != "":
		return failed(s.name, []string{startFailure(s.cmd, o.startErr)})
	}
	head := "ok    " + s.name
	if !o.ok {
		head = "FAIL  " + s.name
	} else if !verbose {
		return []string{head}
	}
	out := []string{head}
	for _, l := range strings.Split(strings.TrimRight(o.out, "\n"), "\n") {
		if !o.ok && strings.HasPrefix(l, "info ") {
			continue
		}
		out = append(out, "      "+l)
	}
	return out
}

// startFailure is the line for a command that couldn't be started: bad-command for a root's
// command, the system's reason for spine.
func startFailure(c *command, why string) string {
	if c.shown == "" {
		return why
	}
	return errorLine("bad-command", c.shown, "cannot run "+c.argv[0]+": "+why)
}

func failed(name string, lines []string) []string {
	out := []string{"FAIL  " + name}
	for _, l := range lines {
		out = append(out, "      "+l)
	}
	return out
}

// passed is whether a step counts as passing in the summary; a skip does.
func passed(s step, o outcome) bool {
	return s.skip != "" || (s.lines == nil && o.startErr == "" && o.ok)
}

// summary is the last line, and the exit code, from the steps that failed.
func summary(failures []string) (string, int) {
	if len(failures) == 0 {
		return "-- all steps passed", 0
	}
	return fmt.Sprintf("-- %d step(s) failed: %s", len(failures), strings.Join(failures, ", ")), 1
}

// label is what goes in front of a root's failed steps in the summary.
func label(prefix string) string {
	if prefix == "" {
		return ""
	}
	return strings.TrimSuffix(prefix, "/") + ": "
}
