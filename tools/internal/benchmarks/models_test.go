package benchmarks

// The models (#106): Mealy machines from the Automata Wiki, read from their .dot files as
// published, and what a spine sees of one under each translation.

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// A mealy machine: in each state, each input gives an output and a next state. A state's name is
// what observe() reports for it; a hidden copy of a state has its name.
type mealy struct {
	Start  int      `json:"start"`
	Names  []string `json:"names"`
	Inputs []string `json:"inputs"`
	Next   [][]arc  `json:"next"` // by state, then input
}

type arc struct {
	To  int    `json:"to"`
	Out string `json:"out"`
}

// models are the four #106 chose, with the start where the file doesn't mark it.
var models = []struct{ name, file, start string }{
	{"coffee", "coffeemachine.dot", ""},
	{"bankcard", "ASN_learnresult_MAESTRO_fix.dot", "s0"}, // the only state that reaches every other
	{"tls", "tls-1.0-mbedtls-2.0.0.dot", ""},
	{"mqtt", "mosquitto__single_client.dot", ""},
}

var (
	edgeLine  = regexp.MustCompile(`^\s*(\w+)\s*->\s*(\w+)\s*\[\s*label\s*=\s*"([^"]*)"\s*\]\s*;?\s*$`)
	startLine = regexp.MustCompile(`^\s*__start0\s*->\s*(\w+)\s*;?\s*$`)
)

// readDot reads a model: every line with an arrow must be the start or an edge labelled with its
// input and output, split at the first /, and every state must give exactly one edge for every
// input. start names the start when the file doesn't mark one.
func readDot(path, start string) (mealy, error) {
	f, err := os.Open(path)
	if err != nil {
		return mealy{}, err
	}
	defer f.Close()
	type edge struct{ from, input, to, out string }
	var edges []edge
	var names []string
	marked := ""
	scan := bufio.NewScanner(f)
	for n := 1; scan.Scan(); n++ {
		line := scan.Text()
		if m := startLine.FindStringSubmatch(line); m != nil {
			if marked != "" {
				return mealy{}, fmt.Errorf("%s:%d: a second start", path, n)
			}
			marked = m[1]
		} else if m := edgeLine.FindStringSubmatch(line); m != nil {
			input, out, ok := strings.Cut(m[3], "/")
			if !ok {
				return mealy{}, fmt.Errorf("%s:%d: a label without a /", path, n)
			}
			edges = append(edges, edge{m[1], strings.TrimSpace(input), m[2], strings.TrimSpace(out)})
			for _, s := range m[1:3] {
				if !slices.Contains(names, s) {
					names = append(names, s)
				}
			}
		} else if strings.Contains(line, "->") {
			return mealy{}, fmt.Errorf("%s:%d: an arrow that is neither an edge nor the start", path, n)
		}
	}
	if err := scan.Err(); err != nil {
		return mealy{}, err
	}
	switch {
	case marked != "" && start != "" && marked != start:
		return mealy{}, fmt.Errorf("%s marks %s as the start, not %s", path, marked, start)
	case marked != "":
		start = marked
	case start == "":
		return mealy{}, fmt.Errorf("%s marks no start", path)
	}
	m := mealy{Start: slices.Index(names, start), Names: names}
	if m.Start < 0 {
		return mealy{}, fmt.Errorf("%s has no state %s", path, start)
	}
	for _, e := range edges {
		if !slices.Contains(m.Inputs, e.input) {
			m.Inputs = append(m.Inputs, e.input)
		}
	}
	slices.Sort(m.Inputs)
	m.Next = make([][]arc, len(names))
	for s := range m.Next {
		m.Next[s] = make([]arc, len(m.Inputs))
		for i := range m.Next[s] {
			m.Next[s][i].To = -1
		}
	}
	for _, e := range edges {
		a := &m.Next[slices.Index(names, e.from)][slices.Index(m.Inputs, e.input)]
		if a.To >= 0 {
			return mealy{}, fmt.Errorf("%s: %s gives two edges for %s", path, e.from, e.input)
		}
		*a = arc{slices.Index(names, e.to), e.out}
	}
	for s, arcs := range m.Next {
		for i, a := range arcs {
			if a.To < 0 {
				return mealy{}, fmt.Errorf("%s: %s gives no edge for %s", path, names[s], m.Inputs[i])
			}
		}
	}
	return m, nil
}

// outputs are the model's outputs, sorted.
func (m mealy) outputs() []string {
	var outs []string
	for _, arcs := range m.Next {
		for _, a := range arcs {
			if !slices.Contains(outs, a.Out) {
				outs = append(outs, a.Out)
			}
		}
	}
	slices.Sort(outs)
	return outs
}

// A translation is how a model becomes a spine: with its outputs dropped, since a spine without
// outputs has none; under the Moore fold, which names each state with the output it was entered
// by; or with each step's output on its rule (#116).
type translation int

const (
	dropped translation = iota
	fold
	onRules
)

var translations = []translation{dropped, fold, onRules}

func (t translation) String() string {
	return [...]string{"outputs dropped", "Moore fold", "outputs on rules"}[t]
}

// seen is the name observe() reports: the state's, and under the Moore fold the output the state
// was entered by, which the start has none of.
func seen(name, out string, moore bool) string {
	if moore && out != "" {
		return name + " / " + out
	}
	return name
}

// A view is what a spine sees of a machine: every configuration it reaches, by the name observe()
// reports in it, where each input takes it, and with outputs on rules what each step sends. With
// outputs dropped or on rules a configuration is a state; under the Moore fold it is a state and
// the output it was entered by.
type view struct {
	names []string   // by configuration; 0 is the start
	next  [][]int    // by configuration, then input
	outs  [][]string // by configuration, then input: the step's output with outputs on rules, else ""
}

func observe(m mealy, t translation) view {
	moore := t == fold
	type config struct {
		state int
		out   string
	}
	index := map[config]int{}
	var configs []config
	add := func(c config) int {
		if !moore {
			c.out = ""
		}
		if n, ok := index[c]; ok {
			return n
		}
		index[c] = len(configs)
		configs = append(configs, c)
		return len(configs) - 1
	}
	add(config{m.Start, ""})
	var v view
	for n := 0; n < len(configs); n++ {
		c := configs[n]
		v.names = append(v.names, seen(m.Names[c.state], c.out, moore))
		row, outs := make([]int, len(m.Inputs)), make([]string, len(m.Inputs))
		for i, a := range m.Next[c.state] {
			row[i] = add(config{a.To, a.Out})
			if t == onRules {
				outs[i] = a.Out
			}
		}
		v.next, v.outs = append(v.next, row), append(v.outs, outs)
	}
	return v
}

// differs says whether some input sequence makes the two report different names, or send
// different outputs.
func differs(a, b view) bool {
	type pair struct{ a, b int }
	seenPairs := map[pair]bool{{0, 0}: true}
	todo := []pair{{0, 0}}
	for len(todo) > 0 {
		p := todo[0]
		todo = todo[1:]
		if a.names[p.a] != b.names[p.b] {
			return true
		}
		for i := range a.next[p.a] {
			if a.outs[p.a][i] != b.outs[p.b][i] {
				return true
			}
			q := pair{a.next[p.a][i], b.next[p.b][i]}
			if !seenPairs[q] {
				seenPairs[q] = true
				todo = append(todo, q)
			}
		}
	}
	return false
}

// partition gives each configuration of v its state in the smallest machine that behaves as v:
// configurations are split by the name they report, then by where each input takes them and what
// it sends, until nothing splits.
func partition(v view) []int {
	split := func(key func(c int) string) []int {
		ids := map[string]int{}
		class := make([]int, len(v.names))
		for c := range class {
			k := key(c)
			if _, ok := ids[k]; !ok {
				ids[k] = len(ids)
			}
			class[c] = ids[k]
		}
		return class
	}
	class := split(func(c int) string { return v.names[c] })
	for {
		next := split(func(c int) string {
			key := []any{class[c]}
			for i, to := range v.next[c] {
				key = append(key, class[to], v.outs[c][i])
			}
			return fmt.Sprint(key)
		})
		if slices.Max(next) == slices.Max(class) {
			return next
		}
		class = next
	}
}

// extraStates is how many more states than the spine the smallest machine that behaves as code
// has, counting only states under a name the spine has. A state under another name diverges the
// moment observe() reports it, so only the others need reaching: the argument behind ADR 0008
// holds with them alone, and code with at most K more of them than the spine has states either
// fails a run of margin K or agrees with the spine everywhere.
func extraStates(spec, code view) int {
	named := map[string]bool{}
	for _, name := range spec.names {
		named[name] = true
	}
	states := map[int]bool{}
	for c, k := range partition(code) {
		if named[code.names[c]] {
			states[k] = true
		}
	}
	return len(states) - len(spec.names)
}

// writeSpine writes v as a spine: each configuration a state, each input an event, no variables,
// and for each configuration and input a rule that moves or stays. With outputs, it declares them,
// and each rule says what its step sends.
func writeSpine(path, machine string, v view, inputs, outputs []string) error {
	q := strconv.Quote
	var b strings.Builder
	fmt.Fprintf(&b, "machine: %s\nstates:\n", q(machine))
	for _, name := range v.names {
		fmt.Fprintf(&b, "  %s: {kind: design}\n", q(name))
	}
	fmt.Fprintf(&b, "initial: %s\nevents:\n", q(v.names[0]))
	for _, input := range inputs {
		fmt.Fprintf(&b, "  - %s\n", q(input))
	}
	if outputs != nil {
		b.WriteString("outputs:\n")
		for _, out := range outputs {
			fmt.Fprintf(&b, "  - %s\n", q(out))
		}
	}
	b.WriteString("rules:\n")
	for c, row := range v.next {
		for i, to := range row {
			outcome := "to: " + q(v.names[to])
			if to == c {
				outcome = "stay: true"
			}
			if outputs != nil {
				outcome += ", outputs: [" + q(v.outs[c][i]) + "]"
			}
			fmt.Fprintf(&b, "  - {from: %s, event: %s, %s}\n", q(v.names[c]), q(inputs[i]), outcome)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
