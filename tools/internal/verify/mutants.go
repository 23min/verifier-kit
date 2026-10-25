package verify

// Mutation as data (#58): reading mutation/mutants.yaml, making a mutant, and judging the runs
// of the oracle on the code as it is and on each mutant. All of it is pure; verify.go makes the
// copies and runs the steps.

import (
	"fmt"
	"path"
	"strings"

	yaml "go.yaml.in/yaml/v4"
)

// mutant is one edit to the code: in file, under src/, the text find, which must occur there
// exactly once, becomes replace.
type mutant struct {
	name, file, find, replace string
}

// mutation is what the mutants.yaml step does: each mutant, and the steps that read src/, which
// run on the code as it is and then on each mutant, each time in a copy of the root.
type mutation struct {
	prefix  string // the root's
	shown   string // mutants.yaml as lines name it
	mutants []mutant
	oracle  []step // planned from the root; each command runs from the copy instead
}

// copied are the root's folders a copy holds: what the steps that read src/ read (OQ4).
var copied = []string{"spec", "conformance", "checks", "src"}

// readMutants reads mutants.yaml. problems are its bad-mutants lines; with any, no mutant runs.
func readMutants(f pieceFile) (mutants []mutant, problems []string) {
	bad := func(format string, args ...any) {
		problems = append(problems, finding("bad-mutants", f.shown, fmt.Sprintf(format, args...)))
	}
	if f.err != nil {
		bad("cannot be read: %s", f.err)
		return nil, problems
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(f.data, &doc); err != nil {
		bad("is not YAML: %s", strings.SplitN(err.Error(), "\n", 2)[0])
		return nil, problems
	}
	list := &doc
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		list = doc.Content[0]
	}
	switch {
	case list.Kind == 0 || list.Kind == yaml.DocumentNode || list.Tag == "!!null":
		bad("holds no mutants")
		return nil, problems
	case list.Kind != yaml.SequenceNode:
		bad("is not a list of mutants")
		return nil, problems
	case len(list.Content) == 0:
		bad("holds no mutants")
		return nil, problems
	}
	named := map[string]bool{}
	for i, item := range list.Content {
		before := len(problems)
		which := fmt.Sprintf("mutant %d", i+1)
		if item.Kind != yaml.MappingNode {
			bad("%s is not a mapping of name, file, find and replace", which)
			continue
		}
		given, text := map[string]bool{}, map[string]string{}
		var wrong []string // the fields' problems, named once the mutant's name is known
		for j := 0; j+1 < len(item.Content); j += 2 {
			key, value := item.Content[j].Value, item.Content[j+1]
			switch {
			case key != "name" && key != "file" && key != "find" && key != "replace":
				wrong = append(wrong, "unknown field "+key)
			case given[key]:
				wrong = append(wrong, key+" is given twice")
			case value.Kind != yaml.ScalarNode || value.Tag != "!!str":
				given[key] = true
				wrong = append(wrong, key+" is not text")
			default:
				given[key], text[key] = true, value.Value
			}
		}
		if name := text["name"]; name != "" {
			which += " (" + name + ")"
		}
		for _, w := range wrong {
			bad("%s: %s", which, w)
		}
		for _, key := range []string{"name", "file", "find", "replace"} {
			if !given[key] {
				bad("%s: no %s", which, key)
			}
		}
		m := mutant{name: text["name"], file: path.Clean(text["file"]), find: text["find"], replace: text["replace"]}
		if name, ok := text["name"]; ok && name == "" {
			bad("%s: name is empty", which)
		}
		if file, ok := text["file"]; ok && !strings.HasPrefix(m.file, "src/") {
			bad("%s: file %s is not under src/", which, file)
		}
		if find, ok := text["find"]; ok && find == "" {
			bad("%s: find is empty", which)
		}
		if m.name != "" && named[m.name] {
			bad("%s: another mutant has this name", which)
		}
		named[m.name] = true
		if len(problems) == before {
			mutants = append(mutants, m)
		}
	}
	return mutants, problems
}

// apply makes the mutant from its file's text, or says why it is stale.
func apply(source string, m mutant) (string, string) {
	if n := strings.Count(source, m.find); n != 1 {
		return "", fmt.Sprintf("its find text occurs %d times in %s, not once", n, m.file)
	}
	return strings.Replace(source, m.find, m.replace, 1), ""
}

// ran is one step of the oracle, run in a copy.
type ran struct {
	name   string
	failed bool
	out    string // its output, with the copy's folder written as the root's
}

// caught names the steps that failed, and the first problem found: the first error line, in the
// contract's finding format, that a failing step printed, its runs of spaces made one. A step
// that prints none, in a format of its own, adds no problem.
func caught(runs []ran) (names []string, first string) {
	for _, r := range runs {
		if !r.failed {
			continue
		}
		names = append(names, r.name)
		for _, l := range strings.Split(r.out, "\n") {
			if first == "" && strings.HasPrefix(strings.TrimSpace(l), "error ") {
				first = strings.Join(strings.Fields(l), " ") // one line in another: no runs of spaces
			}
		}
	}
	return names, first
}

// withFirst adds the first problem to a line, when there is one.
func withFirst(line, first string) string {
	if first == "" {
		return line
	}
	return line + "; first: " + first
}

// oracleFails is the oracle-fails line when a step fails on the code as it is, or empty.
func oracleFails(shown string, runs []ran) string {
	names, first := caught(runs)
	if names == nil {
		return ""
	}
	return withFirst(finding("oracle-fails", shown, "on the code as it is, "+strings.Join(names, ", ")+" failed"), first)
}

// verdictOf is a mutant's line, and whether it counts as an error: killed when a step fails on
// it, any exit but 0, since the same steps passed on the code as it is (OQ9).
func verdictOf(name string, runs []ran) (string, bool) {
	names, first := caught(runs)
	if names == nil {
		return fmt.Sprintf("%-7s %-18s %s: %s", "error", "mutant-survived", name, "no part of the oracle noticed"), true
	}
	return withFirst(fmt.Sprintf("%-7s %-18s %s: caught by %s", "info", "mutant-killed", name, strings.Join(names, ", ")), first), false
}

// staleLine is the line for a mutant that can't be made.
func staleLine(name, why string) string {
	return finding("mutant-stale", name, why)
}

// tally is the last line under the step.
func tally(errors, mutants int) string {
	return fmt.Sprintf("-- %d errors, %d mutants", errors, mutants)
}
