package spine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ---------- the machine, as in tools/spine.py ----------

// Outcome is what one rule does: to (with a target), reject, ignore or stay.
type Outcome struct{ Kind, Target string }

// key is str() of the Python tuple, which decide() sorts by.
func (o Outcome) key() string {
	target := "None"
	if o.Kind == "to" {
		target = pyRepr(o.Target)
	}
	return "(" + pyRepr(o.Kind) + ", " + target + ")"
}

// Rule is one validated rule.
type Rule struct {
	From, Event string
	Outcome     Outcome
	When        *condition // nil when the rule has no when
	canon       string     // json.dumps(rule, sort_keys=True), for the cell hash
}

// Example is one validated worked example.
type Example struct {
	Name, Expect string
	Steps        []string
}

// Spec is a validated spine.
type Spec struct {
	States   []string // declared order
	Initial  string
	Events   []string // as written, repeats and all
	Vars     []Var    // declared order; never nil
	Rules    []*Rule
	Examples []Example
	stateSet map[string]bool
	terminal map[string]bool
}

// newSpec builds a Spec from a spine that validate() passed.
func newSpec(v *Value) *Spec {
	top := v.Map
	get := func(k string) *Value { x, _ := top.Get(k); return x }
	s := &Spec{stateSet: map[string]bool{}, terminal: map[string]bool{}}
	states := get("states").Map
	for i, k := range states.Keys {
		s.States = append(s.States, k.Str)
		s.stateSet[k.Str] = true
		if t, ok := orEmpty(states.Values[i]).Map.Get("terminal"); ok && truthy(t) {
			s.terminal[k.Str] = true
		}
	}
	s.Initial = get("initial").Str
	s.Events = textValues(get("events"))
	s.Vars = specVars(orEmpty(get("vars")))
	varNames := map[string]bool{}
	for _, x := range s.Vars {
		varNames[x.Name] = true
	}
	for _, item := range get("rules").Items {
		m := item.Map
		r := &Rule{canon: canonJSON(m)}
		from, _ := m.Get("from")
		event, _ := m.Get("event")
		r.From, r.Event = from.Str, event.Str
		if to, ok := m.Get("to"); ok {
			r.Outcome = Outcome{"to", to.Str}
		} else {
			for _, k := range []string{"reject", "ignore", "stay"} {
				if x, ok := m.Get(k); ok && truthy(x) {
					r.Outcome = Outcome{Kind: k}
					break
				}
			}
		}
		if when, ok := m.Get("when"); ok {
			r.When, _ = compileCondition(when.Str, varNames)
		}
		s.Rules = append(s.Rules, r)
	}
	if ex, ok := top.Get("examples"); ok && ex.Kind == KList {
		for _, item := range ex.Items {
			name, _ := item.Map.Get("name")
			expect, _ := item.Map.Get("expect")
			steps, _ := item.Map.Get("steps")
			s.Examples = append(s.Examples, Example{name.Str, expect.Str, textValues(steps)})
		}
	}
	return s
}

// canonJSON is json.dumps(rule, sort_keys=True) for a validated rule, whose values are text or true.
func canonJSON(m *Map) string {
	type kv struct{ k, v string }
	var pairs []kv
	for i, k := range m.Keys {
		v := m.Values[i]
		val := "true"
		if isText(v) {
			val = pyJSONString(v.Str)
		}
		pairs = append(pairs, kv{k.Str, val})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = pyJSONString(p.k) + ": " + p.v
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// assignments is every combination of the declared variable values, in a stable order.
func (s *Spec) assignments() []Env {
	vars := append([]Var(nil), s.Vars...)
	sort.Slice(vars, func(i, j int) bool { return vars[i].Name < vars[j].Name })
	names := make([]string, len(vars))
	for i, v := range vars {
		names[i] = v.Name
	}
	envs := []Env{{Names: names, Values: map[string]string{}}}
	for _, v := range vars {
		var next []Env
		for _, e := range envs {
			for _, value := range v.Values {
				values := map[string]string{}
				for k, x := range e.Values {
					values[k] = x
				}
				values[v.Name] = value
				next = append(next, Env{Names: names, Values: values})
			}
		}
		envs = next
	}
	return envs
}

func (s *Spec) rulesFor(state, event string) []*Rule {
	var out []*Rule
	for _, r := range s.Rules {
		if r.From == state && r.Event == event {
			out = append(out, r)
		}
	}
	return out
}

// decide is every distinct outcome the spec allows for one event, in one state, with one set of values.
func (s *Spec) decide(state, event string, env Env) []Outcome {
	seen := map[Outcome]bool{}
	var hits []Outcome
	for _, r := range s.rulesFor(state, event) {
		if (r.When == nil || r.When.holds(env.Values)) && !seen[r.Outcome] {
			seen[r.Outcome] = true
			hits = append(hits, r.Outcome)
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].key() < hits[j].key() })
	return hits
}

func show(outcomes []Outcome) string {
	if len(outcomes) == 0 {
		return "nothing (no rule matches)"
	}
	parts := make([]string, len(outcomes))
	for i, o := range outcomes {
		parts[i] = o.Kind
		if o.Kind == "to" {
			parts[i] = "-> " + o.Target
		}
	}
	return strings.Join(parts, " and ")
}

// step applies one event: the new state, and "" or a verdict, "rejected" or "undefined".
func (s *Spec) step(state, event string, env Env) (string, string) {
	hits := s.decide(state, event, env)
	if len(hits) != 1 {
		return state, "undefined"
	}
	switch hits[0].Kind {
	case "reject":
		return state, "rejected"
	case "to":
		return hits[0].Target, ""
	}
	return state, ""
}

type parsedStep struct {
	event string
	env   Env
}

func (s *Spec) run(steps []parsedStep) string {
	state := s.Initial
	for _, st := range steps {
		var verdict string
		state, verdict = s.step(state, st.event, st.env)
		if verdict != "" {
			return verdict
		}
	}
	return state
}

func orThisCase(s string) string {
	if s == "" {
		return "this case"
	}
	return s
}

// ---------- check ----------

func check(s *Spec) Findings {
	var out Findings
	for _, r := range s.Rules {
		where := r.From + "/" + r.Event
		if !s.stateSet[r.From] {
			out.Add("unknown-state", where, fmt.Sprintf(`rule starts in undeclared state "%s"`, r.From))
		}
		if r.Outcome.Kind == "to" && !s.stateSet[r.Outcome.Target] {
			out.Add("unknown-state", where, fmt.Sprintf(`rule goes to undeclared state "%s"`, r.Outcome.Target))
		}
		if !contains(s.Events, r.Event) {
			out.Add("unknown-event", where, fmt.Sprintf(`rule uses undeclared event "%s"`, r.Event))
		}
	}

	envs := s.assignments()
	for _, st := range s.States {
		for _, e := range s.Events {
			where, rules := st+"/"+e, s.rulesFor(st, e)
			if s.terminal[st] {
				for _, r := range rules {
					if r.Outcome.Kind == "to" {
						out.Add("terminal-has-exit", where, fmt.Sprintf(`"%s" is terminal but "%s" leaves it`, st, e))
						break
					}
				}
				continue
			}
			if len(rules) == 0 {
				out.Add("undecided-cell", where, fmt.Sprintf(`nothing says what "%s" does in "%s"`, e, st))
				continue
			}
			for _, env := range envs {
				hits := s.decide(st, e, env)
				if len(hits) == 0 {
					out.Add("missing-case", where, "no rule covers "+orThisCase(env.String()))
				} else if len(hits) > 1 {
					out.Add("overlap", where, orThisCase(env.String())+" matches conflicting rules: "+show(hits))
				}
			}
		}
	}

	seen, queue := map[string]bool{s.Initial: true}, []string{s.Initial}
	for len(queue) > 0 {
		st := queue[0]
		queue = queue[1:]
		for _, e := range s.Events {
			for _, env := range envs {
				for _, o := range s.decide(st, e, env) {
					if o.Kind == "to" && s.stateSet[o.Target] && !seen[o.Target] {
						seen[o.Target] = true
						queue = append(queue, o.Target)
					}
				}
			}
		}
	}
	for _, st := range s.States {
		if !seen[st] {
			out.Add("unreachable", st, fmt.Sprintf(`no sequence of events reaches "%s" from "%s"`, st, s.Initial))
		}
	}

	for _, ex := range s.Examples {
		var steps []parsedStep
		for _, text := range ex.Steps {
			event, env, _ := parseStep(text)
			steps = append(steps, parsedStep{event, env})
		}
		if got := s.run(steps); got != ex.Expect {
			out.Add("example-fails", ex.Name, fmt.Sprintf("expected %s, got %s", ex.Expect, got))
		}
	}
	return out
}

// ---------- hash / stale ----------

func (s *Spec) cellHash(state, event string) string {
	var canon []string
	for _, r := range s.rulesFor(state, event) {
		canon = append(canon, r.canon)
	}
	sort.Strings(canon)
	sum := sha256.Sum256([]byte(strings.Join(canon, "\n")))
	return hex.EncodeToString(sum[:])[:12]
}

// covers is COVERS in tools/spine.py, with Python's Unicode \s and \w spelled out.
var covers = regexp.MustCompile(`covers:` + pySpaceClass + `*([\p{L}\p{N}_-]+)/([\p{L}\p{N}_-]+)@([0-9a-f]+)`)

const pySpaceClass = `[\t\n\v\f\r\x1c-\x1f \x{85}\x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}]`

func stale(s *Spec, checksDir string) Findings {
	var out Findings
	covered := map[string]bool{}
	for _, path := range rglob(checksDir) {
		if !isFile(path) || contains(strings.Split(path, "/"), "__pycache__") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			out.Add("spec-unreadable", path, "cannot be read: "+strerror(err))
			continue
		}
		text := strings.ToValidUTF8(string(data), "�")
		for _, m := range covers.FindAllStringSubmatch(text, -1) {
			st, e, h := m[1], m[2], m[3]
			cell := st + "/" + e
			switch {
			case !s.stateSet[st] || !contains(s.Events, e) || len(s.rulesFor(st, e)) == 0:
				out.Add("orphan-check", path, fmt.Sprintf("covers %s, which the spec no longer defines", cell))
			case s.cellHash(st, e) != h:
				out.Add("stale-check", path, fmt.Sprintf(
					"covers %s@%s, but that cell now hashes to %s: re-ratify, then update the check", cell, h, s.cellHash(st, e)))
			default:
				covered[cell] = true
			}
		}
	}
	cells := map[string]bool{}
	for _, r := range s.Rules {
		if r.Outcome.Kind == "to" && !covered[r.From+"/"+r.Event] {
			cells[r.From+"/"+r.Event] = true
		}
	}
	var list []string
	for c := range cells {
		list = append(list, c)
	}
	sort.Strings(list)
	for _, c := range list {
		out.AddSeverity("uncovered", c, "no check cites this transition; conformance still exercises it", "info")
	}
	return out
}

// ---------- behaviors / trace ----------

// replayBehavior reports whether the spine allows the behavior, and why.
func replayBehavior(s *Spec, b *Behavior) (bool, string) {
	state := s.Initial
	for i, text := range b.Steps {
		event, env, _ := parseStep(text)
		var verdict string
		state, verdict = s.step(state, event, env)
		if verdict != "" {
			return false, fmt.Sprintf(`step %d "%s" is %s in state %s`, i+1, text, verdict, state)
		}
	}
	if b.Expect != nil && state != *b.Expect {
		return false, fmt.Sprintf("ends in %s, expected %s", state, *b.Expect)
	}
	return true, "allowed, ends in " + state
}

func behaviors(s *Spec, directory string) Findings {
	var out Findings
	counted, checked := 0, 0
	for _, folder := range []struct {
		name      string
		mustAllow bool
	}{{"valid", true}, {"invalid", false}} {
		for _, path := range glob(pyJoin(directory, folder.name), "*.y*ml") {
			counted++
			b, problems := loadBehavior(path, s)
			out = append(out, problems...)
			if b == nil {
				continue
			}
			checked++
			allowed, why := replayBehavior(s, b)
			if folder.mustAllow && !allowed {
				out.Add("behavior-refused", path, "marked valid, but the spine refuses it: "+why)
			} else if !folder.mustAllow && allowed {
				out.Add("behavior-allowed", path, "marked invalid, but the spine allows it ("+why+")")
			}
		}
	}
	if counted == 0 {
		out.AddSeverity("no-behaviors", directory, "no behavior files found under valid/ or invalid/", "warning")
	} else {
		out.AddSeverity("behaviors", directory, fmt.Sprintf("%d behavior files checked", checked), "info")
	}
	return out
}

func trace(s *Spec, path string) Findings {
	var out Findings
	if n := check(s).Errors(); n > 0 {
		out.Add("spine-unsound", "spec", fmt.Sprintf("%d errors from spine check; run it for details", n))
		return out
	}
	b, problems := loadBehavior(path, s)
	out = append(out, problems...)
	if b == nil {
		return out
	}
	if allowed, why := replayBehavior(s, b); !allowed {
		out.Add("refused", path, why)
	}
	return out
}

// ---------- guard ----------

func guard(p *Policy, branch string, files []string) Findings {
	var out Findings
	role, found := "", false
	for _, b := range p.Branches {
		if fnmatch(branch, b.Pattern) {
			role, found = b.Role, true
			break
		}
	}
	if !found {
		out.Add("unknown-role", branch, "no policy entry matches this branch")
		return out
	}
	rules := p.Roles[role]
	for _, f := range files {
		if anyMatch(f, rules.Deny) {
			out.Add("path-denied", f, fmt.Sprintf("the %s role may not change this path", role))
		} else if !anyMatch(f, rules.Allow) {
			out.Add("path-not-allowed", f, fmt.Sprintf("no allow rule for the %s role covers this path", role))
		}
	}
	out.AddSeverity("role", branch, fmt.Sprintf("checked %d changed files as role %s", len(files), role), "info")
	return out
}

func anyMatch(name string, patterns []string) bool {
	for _, p := range patterns {
		if fnmatch(name, p) {
			return true
		}
	}
	return false
}
