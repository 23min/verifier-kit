package benchmarks

// The mutants (#106): faults planted in a model, in the classes conformance testing uses. Each is
// a model of its own, which the adapter runs as the code.

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
)

type mutant struct {
	what string // the fault, for a report
	code mealy
}

// classes are the faults, each tried at the margin #106 gives it.
var classes = []struct {
	name   string
	margin int
	make   func(mealy) []mutant
}{
	{"transfer", 0, transferFaults},
	{"output", 0, outputFaults},
	{"hidden-1", 1, func(m mealy) []mutant { return hiddenStates(m, 1) }},
	{"hidden-2", 1, func(m mealy) []mutant { return hiddenStates(m, 2) }},
}

// hiddenCount is how many mutants each hidden-state class has per model, chosen with hiddenSeed.
const (
	hiddenCount = 100
	hiddenSeed  = 106
)

func (m mealy) clone() mealy {
	c := m
	c.Names = slices.Clone(m.Names)
	c.Next = make([][]arc, len(m.Next))
	for s := range m.Next {
		c.Next[s] = slices.Clone(m.Next[s])
	}
	return c
}

// transferFaults sends each cell, in turn, to each other state.
func transferFaults(m mealy) []mutant {
	var ms []mutant
	for s := range m.Next {
		for i, a := range m.Next[s] {
			for to := range m.Names {
				if to != a.To {
					c := m.clone()
					c.Next[s][i].To = to
					ms = append(ms, mutant{fmt.Sprintf("%s/%s goes to %s, not %s", m.Names[s], m.Inputs[i], m.Names[to], m.Names[a.To]), c})
				}
			}
		}
	}
	return ms
}

// outputFaults gives each cell, in turn, each other output the model has.
func outputFaults(m mealy) []mutant {
	var ms []mutant
	for s := range m.Next {
		for i, a := range m.Next[s] {
			for _, out := range m.outputs() {
				if out != a.Out {
					c := m.clone()
					c.Next[s][i].Out = out
					ms = append(ms, mutant{fmt.Sprintf("%s/%s answers %q, not %q", m.Names[s], m.Inputs[i], out, a.Out), c})
				}
			}
		}
	}
	return ms
}

// hiddenStates adds depth copies of states, in a chain that a redirected cell enters, and changes
// one cell of the last copy, its target or its output. A copy reports the name of the state it
// copies, so the code keeps state that observe() can't see. The mutants are distinct; a report
// names the k-th copy of s3 s3'k.
func hiddenStates(m mealy, depth int) []mutant {
	rng := rand.New(rand.NewPCG(hiddenSeed, uint64(depth)))
	outs := m.outputs()
	var ms []mutant
	seen := map[string]bool{}
	for len(ms) < hiddenCount {
		c := m.clone()
		name := func(s int) string {
			if s < len(m.Names) {
				return c.Names[s]
			}
			return fmt.Sprintf("%s'%d", c.Names[s], s-len(m.Names)+1)
		}
		from, input := rng.IntN(len(c.Names)), rng.IntN(len(c.Inputs))
		what := fmt.Sprintf("%s/%s enters", name(from), c.Inputs[input])
		for k := range depth {
			if k > 0 {
				input = rng.IntN(len(c.Inputs))
				what += fmt.Sprintf(", whose %s enters", c.Inputs[input])
			}
			copied := c.Next[from][input].To
			c.Names = append(c.Names, c.Names[copied])
			c.Next = append(c.Next, slices.Clone(c.Next[copied]))
			c.Next[from][input].To = len(c.Names) - 1
			from = len(c.Names) - 1
			what += fmt.Sprintf(" %s, a hidden copy of %s", name(from), name(copied))
		}
		i := rng.IntN(len(c.Inputs))
		a := &c.Next[from][i]
		if rng.IntN(2) == 0 {
			to := rng.IntN(len(c.Names))
			for to == a.To {
				to = rng.IntN(len(c.Names))
			}
			a.To = to
			what += fmt.Sprintf(", whose %s goes to %s", c.Inputs[i], name(to))
		} else {
			out := outs[rng.IntN(len(outs))]
			for out == a.Out {
				out = outs[rng.IntN(len(outs))]
			}
			a.Out = out
			what += fmt.Sprintf(", whose %s answers %q", c.Inputs[i], out)
		}
		key, _ := json.Marshal(c)
		if !seen[string(key)] {
			seen[string(key)] = true
			ms = append(ms, mutant{what, c})
		}
	}
	return ms
}
