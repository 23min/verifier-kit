package spine

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
)

// command is one of spine's commands: its arguments by name, where a last name ending in "..."
// takes any number, and the options it takes besides -h and --help.
type command struct {
	name    string
	args    []string
	options []string
	about   string
}

var commands = []command{
	{"check", []string{"SPEC"}, []string{"--format", "--junit"},
		"Validates SPEC, then checks that every state, event and variable combination has exactly one\n" +
			"rule, every state is reachable, terminal states have no exits, and worked examples end where\n" +
			"they say."},
	{"hash", []string{"SPEC"}, nil,
		"Prints the whole spine's hash, spine@hash, then one hash per state/event cell, state/event@hash."},
	{"diff", []string{"A", "B"}, []string{"--format", "--junit", "--depth"},
		"Compares two spines case by case, then by behavior over every event sequence up to the depth,\n" +
			"reporting the shortest sequence for each difference."},
	{"stale", []string{"SPEC", "CHECKS_DIR"}, []string{"--format", "--junit"},
		"Finds covers: citations in the files under CHECKS_DIR, and reports stale, orphaned and\n" +
			"uncovered cells, and text meant as a citation that isn't one."},
	{"behaviors", []string{"SPEC", "DIR"}, []string{"--format", "--junit"},
		"Replays every file in DIR/valid/, which must be allowed, and DIR/invalid/, which must be refused."},
	{"trace", []string{"SPEC", "FILE"}, []string{"--format", "--junit"},
		"Replays one behavior file."},
	{"conform", []string{"SPEC"}, []string{"--adapter-cmd", "--format", "--junit", "--depth"},
		"Runs every event sequence up to the depth against the spine and against the code, through the\n" +
			"adapter, and reports where they differ."},
	{"guard", []string{"POLICY", "FILES..."}, []string{"--branch", "--format", "--junit"},
		"Maps the branch to a role and reports changed files that role may not touch. Without FILES, it\n" +
			"reads them from stdin, one per line."},
}

// optionHelp is each option's value and what it does; the options marked required must be given.
var optionHelp = map[string]struct {
	value, help string
	required    bool
}{
	"--adapter-cmd": {"COMMAND", "the adapter's command line, split as a shell would and run without one", true},
	"--branch":      {"NAME", "the branch whose role is checked", true},
	"--format":      {"text|json", "how the findings are printed; text unless given", false},
	"--junit":       {"FILE", "also write the findings as a JUnit XML report", false},
	"--depth":       {"N", "the longest event sequence tried, 1 or more; 4 unless given", false},
}

func (c *command) usage() string {
	words := []string{"spine", c.name}
	for _, a := range c.args {
		if name, ok := strings.CutSuffix(a, "..."); ok {
			a = "[" + name + " ...]"
		}
		words = append(words, a)
	}
	for _, o := range c.options {
		if h := optionHelp[o]; h.required {
			words = append(words, o+" "+h.value)
		} else {
			words = append(words, "["+o+" "+h.value+"]")
		}
	}
	return strings.Join(words, " ")
}

func (c *command) help() string {
	var b strings.Builder
	fmt.Fprintf(&b, "usage: %s\n\n%s\n\noptions:\n", c.usage(), c.about)
	for _, o := range c.options {
		h := optionHelp[o]
		fmt.Fprintf(&b, "  %-24s %s\n", o+" "+h.value, h.help)
	}
	fmt.Fprintf(&b, "  %-24s %s\n", "-h, --help", "show this help")
	return b.String()
}

func topHelp() string {
	var b strings.Builder
	b.WriteString("usage: spine COMMAND ARGUMENTS [OPTIONS]\n\n" +
		"Mechanical checks for state-machine specs, and conformance of code to them.\n\ncommands:\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "  %s\n", c.usage())
	}
	b.WriteString("\nspine COMMAND --help describes one command.\n")
	return b.String()
}

// helpExit is help to print on stdout, exit 0; usageError is a message for stderr, exit 2.
type (
	helpExit   string
	usageError string
)

func (h helpExit) Error() string   { return string(h) }
func (u usageError) Error() string { return string(u) }

// parsed is a command line read: the command, its arguments, the options given and the depth.
type parsed struct {
	command string
	args    []string
	options map[string]string
	depth   int
}

// parseArgs reads the arguments after the program name: a command, then its arguments and options
// in any order. An option is --name value or --name=value, each given at most once, and a value
// that starts with - is given as --name=value; after --, everything is an argument.
func parseArgs(argv []string) (*parsed, error) {
	if len(argv) == 0 {
		return nil, usageError("spine: name a command; spine --help lists them")
	}
	if argv[0] == "-h" || argv[0] == "--help" {
		return nil, helpExit(topHelp())
	}
	i := slices.IndexFunc(commands, func(c command) bool { return c.name == argv[0] })
	if i < 0 {
		return nil, usageError(fmt.Sprintf("spine: unknown command %q; spine --help lists them", argv[0]))
	}
	c := &commands[i]
	fail := func(format string, a ...any) error {
		return usageError(fmt.Sprintf("spine %s: %s; spine %s --help describes it", c.name, fmt.Sprintf(format, a...), c.name))
	}
	rest := argv[1:]
	p := &parsed{command: c.name, options: map[string]string{}, depth: 4}
	onlyArgs := false
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		switch {
		case onlyArgs || a == "-" || !strings.HasPrefix(a, "-"):
			p.args = append(p.args, a)
			continue
		case a == "--":
			onlyArgs = true
			continue
		case a == "-h" || a == "--help":
			return nil, helpExit(c.help())
		}
		name, value, hasValue := strings.Cut(a, "=")
		switch {
		case name == "-h" || name == "--help":
			return nil, fail("%s takes no value", name)
		case !slices.Contains(c.options, name):
			return nil, fail("unknown option %s", name)
		}
		if _, twice := p.options[name]; twice {
			return nil, fail("%s is given twice", name)
		}
		if !hasValue {
			if i+1 == len(rest) || rest[i+1] != "-" && strings.HasPrefix(rest[i+1], "-") {
				return nil, fail("%s needs a value", name)
			}
			i++
			value = rest[i]
		}
		p.options[name] = value
	}
	fixed := c.args
	if last := fixed[len(fixed)-1]; strings.HasSuffix(last, "...") {
		fixed = fixed[:len(fixed)-1]
	} else if len(p.args) > len(fixed) {
		return nil, fail("unexpected argument %q", p.args[len(fixed)])
	}
	if len(p.args) < len(fixed) {
		return nil, fail("%s is missing", fixed[len(p.args)])
	}
	for _, o := range c.options {
		if _, given := p.options[o]; optionHelp[o].required && !given {
			return nil, fail("%s is required", o)
		}
	}
	if f, given := p.options["--format"]; given && f != "text" && f != "json" {
		return nil, fail("--format must be text or json, not %q", f)
	}
	if d, given := p.options["--depth"]; given {
		n, err := strconv.Atoi(d)
		if errors.Is(err, strconv.ErrRange) {
			n, err = math.MaxInt, nil // past an int, no depth can tell the difference
		}
		if err != nil || n < 1 || strings.Trim(d, "0123456789") != "" {
			return nil, fail("--depth must be a whole number of 1 or more, not %q", d)
		}
		p.depth = n
	}
	return p, nil
}

// Main runs spine with the arguments after the program name, and gives the exit code.
func Main(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	ns, err := parseArgs(argv)
	switch e := err.(type) {
	case nil:
	case helpExit:
		io.WriteString(stdout, string(e))
		return 0
	case usageError:
		fmt.Fprintln(stderr, e)
		return 2
	}
	cmd := ns.command
	str := func(k string) string { return ns.options[k] }

	out := bufio.NewWriter(stdout)
	defer out.Flush()
	var findings Findings
	if cmd == "guard" {
		var policy *Policy
		policy, findings = loadPolicy(ns.args[0])
		if policy != nil {
			files := ns.args[1:]
			if len(files) == 0 {
				files = stdinLines(stdin)
			}
			findings = append(findings, guard(policy, str("--branch"), files)...)
		}
	} else if cmd == "diff" {
		a, problems := loadSpec(ns.args[0])
		findings = append(findings, problems...)
		b, problems := loadSpec(ns.args[1])
		findings = append(findings, problems...)
		if findings.Errors() == 0 {
			findings = append(findings, diff(a, b, ns.depth)...)
		}
	} else {
		spec, problems := loadSpec(ns.args[0])
		findings = append(findings, problems...)
		if findings.Errors() == 0 {
			if cmd == "hash" {
				fmt.Fprintf(out, "spine@%s\n", spec.spineHash())
				for _, st := range spec.States {
					for _, ev := range spec.Events {
						if len(spec.rulesFor(st, ev)) > 0 {
							fmt.Fprintf(out, "%s/%s@%s\n", st, ev, spec.cellHash(st, ev))
						}
					}
				}
				return 0
			}
			switch cmd {
			case "check":
				findings = append(findings, check(spec)...)
			case "stale":
				findings = append(findings, stale(spec, ns.args[1])...)
			case "behaviors":
				findings = append(findings, behaviors(spec, ns.args[1])...)
			case "trace":
				findings = append(findings, trace(spec, ns.args[1])...)
			case "conform":
				findings = append(findings, conform(spec, str("--adapter-cmd"), ns.depth)...)
			}
		}
	}

	if str("--format") == "json" {
		emitJSON(out, cmd, findings)
	} else {
		emitText(out, findings)
	}
	if junit, ok := ns.options["--junit"]; ok {
		if err := writeJUnit(junit, "spine "+cmd, findings); err != nil {
			out.Flush()
			fmt.Fprintf(stderr, "spine: cannot write the JUnit report: %v\n", err)
			return 1
		}
	}
	if findings.Errors() > 0 {
		return 1
	}
	return 0
}

// stdinLines is the changed files on stdin: each line with the whitespace at its ends removed,
// \r among it, and blank lines left out.
func stdinLines(r io.Reader) []string {
	data, _ := io.ReadAll(r)
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if s := pyStrip(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}
