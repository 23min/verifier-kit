package spine

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// ---------- the command line: argparse as Python 3.13 runs it for tools/spine.py ----------

// argAction is one argparse action.
type argAction struct {
	dest     string
	options  []string // empty for a positional
	nargs    string   // "1" (one), "*", "0" (the help flag) or "parser"
	choices  []string
	required bool
	isInt    bool // type=int: the value is int() of the argument
}

// argParser is one argparse parser: the top one, or one per command.
type argParser struct {
	prog     string
	help     string // what --help prints, as argparse formats it at 80 columns
	actions  []*argAction
	options  []string // option strings in the order they were added
	byOption map[string]*argAction
	defaults map[string]any
	commands map[string]*argParser
}

func newParser(prog, help string) *argParser {
	p := &argParser{prog: prog, help: help, byOption: map[string]*argAction{}, defaults: map[string]any{}}
	p.add(&argAction{dest: "help", options: []string{"-h", "--help"}, nargs: "0"})
	return p
}

func (p *argParser) add(a *argAction) {
	p.actions = append(p.actions, a)
	for _, o := range a.options {
		p.options = append(p.options, o)
		p.byOption[o] = a
	}
	if a.dest != "help" {
		if _, ok := p.defaults[a.dest]; !ok {
			p.defaults[a.dest] = nil
		}
	}
}

func (p *argParser) common() {
	p.add(&argAction{dest: "format", options: []string{"--format"}, nargs: "1", choices: []string{"text", "json"}})
	p.defaults["format"] = "text"
	p.add(&argAction{dest: "junit", options: []string{"--junit"}, nargs: "1"})
}

func (p *argParser) positional(dest, nargs string) {
	p.add(&argAction{dest: dest, nargs: nargs, required: nargs == "1"})
}

// usageError is argparse's error(): usage and message to stderr, exit 2.
type usageError struct {
	parser *argParser
	msg    string
}

// helpExit is the help action: help to stdout, exit 0.
type helpExit struct{ parser *argParser }

func (u *usageError) Error() string { return u.msg }
func (h *helpExit) Error() string   { return "help" }

func (p *argParser) usage() string {
	usage, _, _ := strings.Cut(p.help, "\n\n")
	return usage + "\n"
}

func (p *argParser) fail(format string, args ...any) error {
	return &usageError{p, fmt.Sprintf(format, args...)}
}

// optionTuple is one reading of an option string: its action (nil when unknown), the option
// string, and the text after it when it was given in the same word.
type optionTuple struct {
	action   *argAction
	option   string
	sep      *string
	explicit *string
}

func ptr(s string) *string { return &s }

// negativeNumber is argparse's _negative_number_matcher, with Python's Unicode \d and its `$`,
// which also matches before a final newline.
var negativeNumber = regexp.MustCompile(`^(?:-\p{Nd}+|-\p{Nd}*\.\p{Nd}+)\n?$`)

func (p *argParser) parseOptional(arg string) []optionTuple {
	if arg == "" || arg[0] != '-' {
		return nil
	}
	if a, ok := p.byOption[arg]; ok {
		return []optionTuple{{a, arg, nil, nil}}
	}
	if arg == "-" {
		return nil
	}
	if opt, explicit, found := strings.Cut(arg, "="); found {
		if a, ok := p.byOption[opt]; ok {
			return []optionTuple{{a, opt, ptr("="), ptr(explicit)}}
		}
	}
	if tuples := p.optionTuples(arg); len(tuples) > 0 {
		return tuples
	}
	if negativeNumber.MatchString(arg) {
		return nil
	}
	if strings.Contains(arg, " ") {
		return nil
	}
	return []optionTuple{{nil, arg, nil, nil}}
}

func (p *argParser) optionTuples(arg string) []optionTuple {
	var out []optionTuple
	prefix, explicit, found := strings.Cut(arg, "=")
	var sep, exp *string
	if found {
		sep, exp = ptr("="), ptr(explicit)
	}
	runes := []rune(arg)
	if runes[1] == '-' {
		for _, o := range p.options {
			if strings.HasPrefix(o, prefix) {
				out = append(out, optionTuple{p.byOption[o], o, sep, exp})
			}
		}
		return out
	}
	short, shortExplicit := string(runes[:2]), string(runes[2:])
	for _, o := range p.options {
		if o == short {
			out = append(out, optionTuple{p.byOption[o], o, ptr(""), ptr(shortExplicit)})
		} else if strings.HasPrefix(o, prefix) {
			out = append(out, optionTuple{p.byOption[o], o, sep, exp})
		}
	}
	return out
}

func nargsPattern(a *argAction) string {
	option := len(a.options) > 0
	switch a.nargs {
	case "1":
		if option {
			return `([A])`
		}
		return `(-*A-*)`
	case "*":
		if option {
			return `(A*)`
		}
		return `(-*[A-]*)`
	case "parser":
		return `(-*A[-AO]*)`
	}
	return `()` // the help flag takes nothing
}

func (p *argParser) matchArgument(a *argAction, pattern string) (int, error) {
	m := regexp.MustCompile(`^` + nargsPattern(a)).FindStringSubmatch(pattern)
	if m == nil {
		return 0, p.fail("argument %s: expected one argument", actionName(a))
	}
	return len(m[1]), nil
}

func matchPartial(actions []*argAction, pattern string) []int {
	for i := len(actions); i > 0; i-- {
		var re strings.Builder
		re.WriteString("^")
		for _, a := range actions[:i] {
			re.WriteString(nargsPattern(a))
		}
		m := regexp.MustCompile(re.String()).FindStringSubmatchIndex(pattern)
		if m == nil {
			continue
		}
		var counts []int
		for g := 1; g <= i; g++ {
			counts = append(counts, m[2*g+1]-m[2*g])
		}
		if m[1] < len(pattern) && pattern[m[1]] == 'O' {
			for len(counts) > 0 && counts[len(counts)-1] == 0 {
				counts = counts[:len(counts)-1]
			}
		}
		return counts
	}
	return nil
}

func actionName(a *argAction) string {
	if len(a.options) > 0 {
		return strings.Join(a.options, "/")
	}
	return a.dest
}

func removeFirst(list []string, s string) []string {
	for i, x := range list {
		if x == s {
			return append(append([]string(nil), list[:i]...), list[i+1:]...)
		}
	}
	return list
}

// parseKnownArgs is ArgumentParser.parse_known_args: it fills ns and returns what it didn't use.
func (p *argParser) parseKnownArgs(args []string, ns map[string]any) ([]string, error) {
	for k, v := range p.defaults {
		if _, ok := ns[k]; !ok {
			ns[k] = v
		}
	}
	options := map[int][]optionTuple{}
	var pattern strings.Builder
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			pattern.WriteString("-")
			pattern.WriteString(strings.Repeat("A", len(args)-i-1))
			break
		}
		if tuples := p.parseOptional(args[i]); tuples != nil {
			options[i] = tuples
			pattern.WriteString("O")
		} else {
			pattern.WriteString("A")
		}
	}
	pat := pattern.String()

	seen := map[*argAction]bool{}
	var extras []string
	takeAction := func(a *argAction, values []string) error {
		seen[a] = true
		switch a.nargs {
		case "0":
			return &helpExit{p}
		case "1":
			if a.choices != nil && !contains(a.choices, values[0]) {
				return p.fail("argument %s: invalid choice: %s (choose from %s)", actionName(a), pyRepr(values[0]), strings.Join(a.choices, ", "))
			}
			if a.isInt {
				n, text, ok := intArgument(values[0])
				if !ok {
					return p.fail("argument %s: invalid int value: %s", actionName(a), pyRepr(values[0]))
				}
				ns[a.dest], ns[a.dest+"_text"] = n, text
			} else {
				ns[a.dest] = values[0]
			}
		case "*":
			ns[a.dest] = append([]string{}, values...)
		case "parser":
			sub, ok := p.commands[values[0]]
			if !ok {
				names := []string{"check", "hash", "diff", "stale", "behaviors", "trace", "conform", "guard"}
				return p.fail("argument cmd: invalid choice: %s (choose from %s)", pyRepr(values[0]), strings.Join(names, ", "))
			}
			ns[a.dest] = values[0]
			subNS := map[string]any{}
			rest, err := sub.parseKnownArgs(values[1:], subNS)
			if err != nil {
				return err
			}
			for k, v := range subNS {
				ns[k] = v
			}
			ns["_unrecognized"] = rest
		}
		return nil
	}

	consumeOptional := func(start int) (int, error) {
		tuples := options[start]
		if len(tuples) > 1 {
			var names []string
			for _, t := range tuples {
				names = append(names, t.option)
			}
			return 0, p.fail("ambiguous option: %s could match %s", args[start], strings.Join(names, ", "))
		}
		t := tuples[0]
		a, option, sep, explicit := t.action, t.option, t.sep, t.explicit
		type taken struct {
			a    *argAction
			args []string
		}
		var todo []taken
		var stop int
		for {
			if a == nil {
				extras = append(extras, args[start])
				return start + 1, nil
			}
			if explicit != nil {
				count, _ := p.matchArgument(a, "A")
				optRunes := []rune(option)
				if count == 0 && optRunes[1] != '-' && *explicit != "" {
					if (sep != nil && *sep != "") || (*explicit)[0] == '-' {
						return 0, p.fail("argument %s: ignored explicit argument %s", actionName(a), pyRepr(*explicit))
					}
					todo = append(todo, taken{a, nil})
					expRunes := []rune(*explicit)
					next := string(optRunes[0]) + string(expRunes[0])
					if na, ok := p.byOption[next]; ok {
						a, option = na, next
						rest := string(expRunes[1:])
						switch {
						case rest == "":
							sep, explicit = nil, nil
						case rest[0] == '=':
							sep, explicit = ptr("="), ptr(rest[1:])
						default:
							sep, explicit = ptr(""), ptr(rest)
						}
						continue
					}
					extras = append(extras, string(optRunes[0])+*explicit)
					stop = start + 1
					break
				}
				if count == 1 {
					stop = start + 1
					todo = append(todo, taken{a, []string{*explicit}})
					break
				}
				return 0, p.fail("argument %s: ignored explicit argument %s", actionName(a), pyRepr(*explicit))
			}
			count, err := p.matchArgument(a, pat[start+1:])
			if err != nil {
				return 0, err
			}
			stop = start + 1 + count
			todo = append(todo, taken{a, args[start+1 : stop]})
			break
		}
		for _, x := range todo {
			if err := takeAction(x.a, x.args); err != nil {
				return 0, err
			}
		}
		return stop, nil
	}

	var positionals []*argAction
	for _, a := range p.actions {
		if len(a.options) == 0 {
			positionals = append(positionals, a)
		}
	}
	consumePositionals := func(start int) (int, error) {
		counts := matchPartial(positionals, pat[start:])
		for i, count := range counts {
			a := positionals[i]
			values := args[start : start+count]
			if a.nargs == "parser" {
				if pat[start] == '-' {
					values = removeFirst(values, "--")
				}
			} else if strings.Contains(pat[start:start+count], "-") {
				values = removeFirst(values, "--")
			}
			start += count
			if err := takeAction(a, values); err != nil {
				return 0, err
			}
		}
		positionals = positionals[len(counts):]
		return start, nil
	}

	maxOption := -1
	for i := range options {
		maxOption = max(maxOption, i)
	}
	start := 0
	for start <= maxOption {
		next := start
		for next <= maxOption {
			if _, ok := options[next]; ok {
				break
			}
			next++
		}
		if start != next {
			end, err := consumePositionals(start)
			if err != nil {
				return nil, err
			}
			if end > start {
				start = end
				continue
			}
			start = end
		}
		if _, ok := options[start]; !ok {
			extras = append(extras, args[start:next]...)
			start = next
		}
		var err error
		if start, err = consumeOptional(start); err != nil {
			return nil, err
		}
	}
	stop, err := consumePositionals(start)
	if err != nil {
		return nil, err
	}
	extras = append(extras, args[stop:]...)

	var missing []string
	for _, a := range p.actions {
		if !seen[a] && a.required {
			missing = append(missing, actionName(a))
		}
	}
	if len(missing) > 0 {
		return nil, p.fail("the following arguments are required: %s", strings.Join(missing, ", "))
	}
	if rest, ok := ns["_unrecognized"].([]string); ok {
		delete(ns, "_unrecognized")
		extras = append(extras, rest...)
	}
	return extras, nil
}

// buildParser is main()'s parser in tools/spine.py.
func buildParser() *argParser {
	top := newParser("spine", helpTop)
	top.add(&argAction{dest: "cmd", nargs: "parser", required: true})
	top.commands = map[string]*argParser{}
	for _, c := range []struct{ name, help string }{
		{"check", helpCheck}, {"hash", helpHash}, {"diff", helpDiff}, {"stale", helpStale},
		{"behaviors", helpBehaviors}, {"trace", helpTrace}, {"conform", helpConform}, {"guard", helpGuard},
	} {
		p := newParser("spine "+c.name, c.help)
		switch c.name {
		case "hash":
			p.positional("spec", "1")
			p.defaults["format"], p.defaults["junit"] = "text", nil
		case "check":
			p.common()
			p.positional("spec", "1")
		case "diff":
			p.common()
			p.positional("a", "1")
			p.positional("b", "1")
			p.add(&argAction{dest: "depth", options: []string{"--depth"}, nargs: "1", isInt: true})
			p.defaults["depth"] = 4
		case "stale":
			p.common()
			p.positional("spec", "1")
			p.positional("checks", "1")
		case "behaviors":
			p.common()
			p.positional("spec", "1")
			p.positional("directory", "1")
		case "trace":
			p.common()
			p.positional("spec", "1")
			p.positional("behavior", "1")
		case "conform":
			p.common()
			p.positional("spec", "1")
			p.add(&argAction{dest: "adapter_cmd", options: []string{"--adapter-cmd"}, nargs: "1", required: true})
			p.add(&argAction{dest: "depth", options: []string{"--depth"}, nargs: "1", isInt: true})
			p.defaults["depth"], p.defaults["depth_text"] = 4, "4"
		case "guard":
			p.common()
			p.positional("policy", "1")
			p.add(&argAction{dest: "branch", options: []string{"--branch"}, nargs: "1", required: true})
			p.positional("files", "*")
		}
		top.commands[c.name] = p
	}
	return top
}

// Main runs spine with the arguments after the program name, and gives the exit code.
func Main(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	top := buildParser()
	ns := map[string]any{}
	extras, err := top.parseKnownArgs(argv, ns)
	if err == nil && len(extras) > 0 { // argparse can't place FILES given after --branch itself
		files, _ := ns["files"].([]string)
		if ns["cmd"] != "guard" || anyOption(extras) {
			err = top.fail("unrecognized arguments: %s", strings.Join(extras, " "))
		} else {
			ns["files"] = append(files, extras...)
		}
	}
	switch e := err.(type) {
	case nil:
	case *helpExit:
		io.WriteString(stdout, e.parser.help)
		return 0
	case *usageError:
		fmt.Fprintf(stderr, "%s%s: error: %s\n", e.parser.usage(), e.parser.prog, e.msg)
		return 2
	}
	cmd := ns["cmd"].(string)
	str := func(k string) string { s, _ := ns[k].(string); return s }

	out := bufio.NewWriter(stdout)
	defer out.Flush()
	var findings Findings
	if cmd == "guard" {
		var policy *Policy
		policy, findings = loadPolicy(str("policy"))
		if policy != nil {
			files, _ := ns["files"].([]string)
			if len(files) == 0 {
				files = stdinLines(stdin)
			}
			findings = append(findings, guard(policy, str("branch"), files)...)
		}
	} else if cmd == "diff" {
		a, problems := loadSpec(str("a"))
		findings = append(findings, problems...)
		b, problems := loadSpec(str("b"))
		findings = append(findings, problems...)
		if findings.Errors() == 0 {
			findings = append(findings, diff(a, b, ns["depth"].(int))...)
		}
	} else {
		spec, problems := loadSpec(str("spec"))
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
				findings = append(findings, stale(spec, str("checks"))...)
			case "behaviors":
				findings = append(findings, behaviors(spec, str("directory"))...)
			case "trace":
				findings = append(findings, trace(spec, str("behavior"))...)
			case "conform":
				findings = append(findings, conform(spec, str("adapter_cmd"), ns["depth"].(int), str("depth_text"))...)
			}
		}
	}

	if str("format") == "json" {
		emitJSON(out, cmd, findings)
	} else {
		emitText(out, findings)
	}
	if junit := str("junit"); junit != "" {
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

func anyOption(list []string) bool {
	for _, s := range list {
		if strings.HasPrefix(s, "-") {
			return true
		}
	}
	return false
}

// stdinLines is the changed files on stdin: each line stripped, blank ones left out. Python's
// stdin ends lines at \n only; a \r is whitespace that strip() removes.
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

// The help argparse prints at 80 columns, the width it uses when COLUMNS is unset and output
// is not a terminal; the record holds each (cases_test.go).
const (
	helpTop = `usage: spine [-h] {check,hash,diff,stale,behaviors,trace,conform,guard} ...

spine: mechanical checks for state-machine specs, and conformance of code to
them.

positional arguments:
  {check,hash,diff,stale,behaviors,trace,conform,guard}

options:
  -h, --help            show this help message and exit
`
	helpCheck = `usage: spine check [-h] [--format {text,json}] [--junit FILE] spec

positional arguments:
  spec

options:
  -h, --help            show this help message and exit
  --format {text,json}
  --junit FILE          also write a JUnit XML report
`
	helpHash = `usage: spine hash [-h] spec

positional arguments:
  spec

options:
  -h, --help  show this help message and exit
`
	helpDiff = `usage: spine diff [-h] [--format {text,json}] [--junit FILE] [--depth DEPTH]
                  a b

positional arguments:
  a
  b

options:
  -h, --help            show this help message and exit
  --format {text,json}
  --junit FILE          also write a JUnit XML report
  --depth DEPTH
`
	helpConform = `usage: spine conform [-h] [--format {text,json}] [--junit FILE]
                     --adapter-cmd COMMAND [--depth DEPTH]
                     spec

positional arguments:
  spec

options:
  -h, --help            show this help message and exit
  --format {text,json}
  --junit FILE          also write a JUnit XML report
  --adapter-cmd COMMAND
                        a program speaking the adapter protocol; split like a
                        shell would, run without one
  --depth DEPTH
`
	helpStale = `usage: spine stale [-h] [--format {text,json}] [--junit FILE] spec checks

positional arguments:
  spec
  checks

options:
  -h, --help            show this help message and exit
  --format {text,json}
  --junit FILE          also write a JUnit XML report
`
	helpBehaviors = `usage: spine behaviors [-h] [--format {text,json}] [--junit FILE]
                       spec directory

positional arguments:
  spec
  directory

options:
  -h, --help            show this help message and exit
  --format {text,json}
  --junit FILE          also write a JUnit XML report
`
	helpTrace = `usage: spine trace [-h] [--format {text,json}] [--junit FILE] spec behavior

positional arguments:
  spec
  behavior

options:
  -h, --help            show this help message and exit
  --format {text,json}
  --junit FILE          also write a JUnit XML report
`
	helpGuard = `usage: spine guard [-h] [--format {text,json}] [--junit FILE] --branch BRANCH
                   policy [files ...]

positional arguments:
  policy
  files                 changed files; read from stdin when omitted

options:
  -h, --help            show this help message and exit
  --format {text,json}
  --junit FILE          also write a JUnit XML report
  --branch BRANCH
`
)
