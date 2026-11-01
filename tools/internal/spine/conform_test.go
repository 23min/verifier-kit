package spine

// The search without a program (#94): conform's core is driven by a fake adapter in Go, the way
// the shell drives it with a real one.

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// restoreBug is notes' code with a bug that hides in a counter the adapter can't see: it follows
// the spine, but the third restore from the trash leaves the note in the trash.
type restoreBug struct {
	s        *Spec
	state    string
	restores int
}

// answer replies to one request as the adapter program would.
func (f *restoreBug) answer(request string) reply {
	switch request {
	case newRequest:
		f.state, f.restores = f.s.Initial, 0
		return reply{line: []byte(`{"ok": true}`)}
	case observeRequest:
		return reply{line: []byte(`{"state": ` + jsonString(f.state) + `}`)}
	}
	var r struct {
		Event string
		Env   map[string]string
	}
	if err := json.Unmarshal([]byte(request), &r); err != nil {
		panic(err)
	}
	next, verdict := f.s.step(f.state, r.Event, Env{Values: r.Env})
	if verdict == "rejected" {
		return reply{line: []byte(`{"result": "rejected"}`)}
	}
	if f.state == "trashed" && r.Event == "restore" {
		if f.restores++; f.restores == 3 {
			next = f.state
		}
	}
	f.state = next
	return reply{line: []byte(`{"result": "ok"}`)}
}

// #89's AC7: a bug on the third restore from the trash takes five steps past the trash's shortest
// path, so margin 4 catches it and margin 3 does not.
func TestThirdRestore(t *testing.T) {
	s, problems := loadSpec(readFile(filepath.Join("..", "..", "..", "examples", "notes", "spec", "notes", "spine.yaml")))
	if s == nil {
		t.Fatal(problems)
	}
	for _, tc := range []struct {
		margin int
		caught bool
	}{{3, false}, {4, true}} {
		c := newSearch(s, "restoreBug", tc.margin)
		f := &restoreBug{s: s}
		for request := c.first(); request != ""; request = c.next(f.answer(request)) {
		}
		var failures []Finding
		for _, x := range c.out {
			if x.Severity == "error" {
				failures = append(failures, x)
			}
		}
		caught := len(failures) == 1 && failures[0].Code == "diverges" && strings.HasPrefix(failures[0].Where, "trashed/restore ")
		if caught != tc.caught || !caught && len(failures) > 0 {
			t.Errorf("margin %d: caught is %v, want %v; findings: %v", tc.margin, caught, tc.caught, c.out)
		}
	}
}
