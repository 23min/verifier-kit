package spine

import (
	"strings"
	"testing"

	"github.com/23min/verifier-kit/tools/internal/yamldoc"
)

// The whole spine's hash (#59) covers what the machine does, and nothing else.

const hashBase = `machine: door
section: "1.2"
vars:
  role: [owner, guest]
  mood: [calm, cross]
states:
  closed: {kind: fact}
  open: {}
  gone: {kind: design, terminal: true}
initial: closed
events: [open, remove]
rules:
  - {from: closed, event: open, when: "role == 'owner'", to: open}
  - {from: closed, event: open, when: "role != 'owner'", reject: true}
  - {from: closed, event: remove, to: gone}
  - {from: open, event: open, stay: true}
  - {from: open, event: remove, ignore: true}
examples:
  - {name: owner opens, steps: ["open role=owner mood=calm"], expect: open}
`

func hashOf(t *testing.T, text string) string {
	t.Helper()
	v, err := yamldoc.Load([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if f := validate(v, "spine"); f.Errors() > 0 {
		t.Fatalf("not a valid spine: %v", f)
	}
	return newSpec(v).spineHash()
}

func TestSpineHash(t *testing.T) {
	base := hashOf(t, hashBase)
	for _, c := range []struct {
		change, old, new string
		same             bool
	}{
		{"the machine's name", "machine: door", "machine: gate", true},
		{"the section", `section: "1.2"`, `section: "3"`, true},
		{"a state's kind", "closed: {kind: fact}", "closed: {kind: policy}", true},
		{"the worked examples", "examples:\n  - {name: owner opens, steps: [\"open role=owner mood=calm\"], expect: open}\n", "", true},
		{"a comment", "rules:\n", "rules:\n  # who may open\n", true},
		{"the order of states", "  closed: {kind: fact}\n  open: {}\n", "  open: {}\n  closed: {kind: fact}\n", true},
		{"the order of events", "events: [open, remove]", "events: [remove, open]", true},
		{"the order of variables", "  role: [owner, guest]\n  mood: [calm, cross]\n", "  mood: [calm, cross]\n  role: [owner, guest]\n", true},
		{"the order of a variable's values", "role: [owner, guest]", "role: [guest, owner]", true},
		{"the order of rules", "  - {from: open, event: open, stay: true}\n  - {from: open, event: remove, ignore: true}\n",
			"  - {from: open, event: remove, ignore: true}\n  - {from: open, event: open, stay: true}\n", true},

		{"initial", "initial: closed", "initial: open", false},
		{"which states are terminal", "open: {}", "open: {terminal: true}", false},
		{"a state", "  gone: {kind: design, terminal: true}", "  lost: {}\n  gone: {kind: design, terminal: true}", false},
		{"an event", "events: [open, remove]", "events: [open, remove, knock]", false},
		{"a variable's values", "mood: [calm, cross]", "mood: [calm, cross, glad]", false},
		{"a rule's outcome", "{from: open, event: remove, ignore: true}", "{from: open, event: remove, reject: true}", false},
		{"a rule's condition, even reworded, as a cell's hash", `when: "role == 'owner'", to: open`, `when: "'owner' == role", to: open`, false},
	} {
		if strings.Count(hashBase, c.old) != 1 {
			t.Fatalf("%s: %q is not in the base spine once", c.change, c.old)
		}
		got := hashOf(t, strings.Replace(hashBase, c.old, c.new, 1))
		if c.same && got != base {
			t.Errorf("changing %s changed the hash", c.change)
		}
		if !c.same && got == base {
			t.Errorf("changing %s left the hash alone", c.change)
		}
	}
}
