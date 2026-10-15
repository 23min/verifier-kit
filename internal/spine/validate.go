package spine

import (
	"errors"
	"fmt"
	"strings"
)

// ---------- loading and validation, as in tools/spine.py ----------

var (
	topKeys      = names("machine", "section", "vars", "states", "initial", "events", "rules", "examples")
	requiredKeys = []string{"events", "initial", "machine", "rules", "states"} // sorted, as reported
	stateKeys    = names("kind", "terminal")
	stateKinds   = names("fact", "policy", "design")
	ruleKeys     = names("from", "event", "when", "to", "reject", "ignore", "stay")
	outcomes     = []string{"to", "reject", "ignore", "stay"}
	exampleKeys  = []string{"expect", "name", "steps"} // sorted, as reported
	behaviorKeys = names("steps", "expect")
	policyKeys   = []string{"branches", "roles"} // sorted, as reported
	roleKeys     = []string{"allow", "deny"}     // sorted, as checked
)

const (
	boolKeyHint = "YAML reads keys such as `on`, `off`, `yes` and `no` as true or false; spell the key as `event` or quote it"
	quoteHint   = "YAML reads `on`, `off`, `yes` and `no` as true or false, so quote it"
)

func names(list ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range list {
		m[s] = true
	}
	return m
}

// checkKeys adds yaml-boolean-key for a key YAML read as true or false, and unknown-key for any
// other key that isn't text or (when allowed isn't nil) isn't in allowed.
func checkKeys(m *Map, allowed map[string]bool, where, what string, out *Findings, hint string) {
	for _, k := range m.Keys {
		switch {
		case isBool(k):
			out.Add("yaml-boolean-key", where, fmt.Sprintf("%s key read as %s; %s", what, pyBoolRepr(k.Bool), hint))
		case !isText(k):
			out.Add("unknown-key", where, fmt.Sprintf("a %s key is not text", what))
		case allowed != nil && !allowed[k.Str]:
			out.Add("unknown-key", where, fmt.Sprintf("unknown %s key \"%s\"", what, k.Str))
		}
	}
}

// checkText reports whether v is text; otherwise it adds yaml-boolean-value or spec-shape,
// without echoing the value.
func checkText(v *Value, where, what string, out *Findings) bool {
	switch {
	case isText(v):
		return true
	case isBool(v):
		out.Add("yaml-boolean-value", where, what+" was read as true or false; quote it")
	default:
		out.Add("spec-shape", where, what+" must be text")
	}
	return false
}

// isMap is isinstance(v, dict).
func isMap(v *Value) bool { return v != nil && v.Kind == KMap }

// isList is isinstance(v, list).
func isList(v *Value) bool { return v != nil && v.Kind == KList }

// allText is all(isinstance(x, str) for x in v.Items).
func allText(v *Value) bool {
	for _, x := range v.Items {
		if !isText(x) {
			return false
		}
	}
	return true
}

// orEmpty is Python's `v or {}`: a false value becomes an empty mapping.
func orEmpty(v *Value) *Value {
	if truthy(v) {
		return v
	}
	return &Value{Kind: KMap, Map: newMap()}
}

// Var is one declared variable and its values, in the order the spine gives them.
type Var struct {
	Name   string
	Values []string
}

// checkSteps adds spec-shape unless steps is a list of text, and bad-step for each step that
// can't be parsed, names an event not in events, or doesn't give each of vars one of its
// values (vars nil skips that).
func checkSteps(steps *Value, where string, out *Findings, events []string, vars []Var) {
	if !isList(steps) || !allText(steps) {
		out.Add("spec-shape", where, `"steps" must be a list of steps such as "restore role=editor"`)
		return
	}
	for i, s := range steps.Items {
		event, env, err := parseStep(s.Str)
		if err == nil && !contains(events, event) {
			err = fmt.Errorf("\"%s\" is not a declared event", event)
		}
		if err == nil && vars != nil {
			err = checkEnv(env, vars)
		}
		if err != nil {
			out.Add("bad-step", where, fmt.Sprintf("step %d \"%s\": %s", i+1, s.Str, err))
		}
	}
}

// Env is the variable values of one case, in order: sorted by name for the cases spine
// enumerates, as written for a parsed step.
type Env struct {
	Names  []string
	Values map[string]string
}

func (e Env) String() string {
	parts := make([]string, len(e.Names))
	for i, n := range e.Names {
		parts[i] = n + "=" + e.Values[n]
	}
	return strings.Join(parts, ", ")
}

// parseStep turns "restore role=editor" into ("restore", role=editor), or an error when malformed.
func parseStep(s string) (string, Env, error) {
	words := pySplit(s)
	env := Env{Values: map[string]string{}}
	if len(words) == 0 {
		return "", env, errors.New("a step needs an event")
	}
	for _, pair := range words[1:] {
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return "", env, fmt.Errorf("\"%s\" is not variable=value", pair)
		}
		if _, seen := env.Values[name]; seen {
			return "", env, fmt.Errorf("\"%s\" is given twice", name)
		}
		env.Names = append(env.Names, name)
		env.Values[name] = value
	}
	return words[0], env, nil
}

// checkEnv is an error unless env gives every declared variable one of its values, and nothing else.
func checkEnv(env Env, vars []Var) error {
	declared := map[string][]string{}
	for _, v := range vars {
		declared[v.Name] = v.Values
	}
	for _, name := range env.Names {
		values, ok := declared[name]
		if !ok {
			return fmt.Errorf("\"%s\" is not a declared variable", name)
		}
		if !contains(values, env.Values[name]) {
			return fmt.Errorf("\"%s\" is not a value of \"%s\"", env.Values[name], name)
		}
	}
	for _, v := range vars {
		if _, ok := env.Values[v.Name]; !ok {
			return fmt.Errorf("no value for \"%s\"", v.Name)
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// textValues is a list of text as strings; the caller has checked it.
func textValues(v *Value) []string {
	out := make([]string, len(v.Items))
	for i, x := range v.Items {
		out[i] = x.Str
	}
	return out
}

// loadSpec loads and validates a spine file. The spec is nil when there are errors.
func loadSpec(path string) (*Spec, Findings) {
	var out Findings
	v, ok := readYAML(path, &out)
	if !ok {
		return nil, out
	}
	out = validate(v, path)
	if out.Errors() > 0 {
		return nil, out
	}
	return newSpec(v), out
}

// Behavior is a validated behavior file.
type Behavior struct {
	Steps  []string
	Expect *string
}

// loadBehavior loads and validates a behavior file for a valid spine. The behavior is nil when
// there are errors.
func loadBehavior(path string, s *Spec) (*Behavior, Findings) {
	var out Findings
	v, ok := readYAML(path, &out)
	if !ok {
		return nil, out
	}
	where := path
	if !isMap(v) {
		out.Add("spec-shape", where, `a behavior file must be a mapping with "steps" and an optional "expect"`)
		return nil, out
	}
	checkKeys(v.Map, behaviorKeys, where, "behavior", &out, quoteHint)
	steps, hasSteps := v.Map.Get("steps")
	if !hasSteps {
		out.Add("missing-key", where, `a behavior needs "steps"`)
	} else {
		checkSteps(steps, where, &out, s.Events, s.Vars)
	}
	expect, hasExpect := v.Map.Get("expect")
	if hasExpect && checkText(expect, where, `"expect"`, &out) && !s.stateSet[expect.Str] {
		out.Add("bad-value", where, fmt.Sprintf(`expect "%s" is not a declared state`, expect.Str))
	}
	if out.Errors() > 0 {
		return nil, out
	}
	b := &Behavior{Steps: textValues(steps)}
	if hasExpect {
		b.Expect = &expect.Str
	}
	return b, out
}

// Policy is a validated policy file: branch patterns to roles, and each role's path patterns.
type Policy struct {
	Branches []BranchRole
	Roles    map[string]Role
}

type BranchRole struct{ Pattern, Role string }

type Role struct{ Allow, Deny []string }

// loadPolicy loads and validates a policy file. The policy is nil when there are errors.
func loadPolicy(path string) (*Policy, Findings) {
	var out Findings
	v, ok := readYAML(path, &out)
	if !ok {
		return nil, out
	}
	if !isMap(v) {
		out.Add("spec-shape", path, `the policy must be a mapping with "branches" and "roles"`)
		return nil, out
	}
	checkKeys(v.Map, names(policyKeys...), path, "policy", &out, quoteHint)
	for _, k := range policyKeys {
		if !v.Map.Has(k) {
			out.Add("missing-key", path, fmt.Sprintf(`the policy needs "%s"`, k))
		}
	}
	if out.Errors() > 0 {
		return nil, out
	}
	branches, _ := v.Map.Get("branches")
	roles, _ := v.Map.Get("roles")
	if !isMap(branches) || !allPairs(branches.Map, func(k, x *Value) bool { return isText(k) && isText(x) }) {
		out.Add("spec-shape", path, `"branches" must map branch patterns to role names`)
	}
	if !isMap(roles) || !allPairs(roles.Map, func(k, _ *Value) bool { return isText(k) }) {
		out.Add("spec-shape", path, `"roles" must map role names to their allow and deny lists`)
	}
	if out.Errors() > 0 {
		return nil, out
	}
	p := &Policy{Roles: map[string]Role{}}
	for i, k := range roles.Map.Keys {
		name, rules := k.Str, orEmpty(roles.Map.Values[i])
		if !isMap(rules) {
			out.Add("spec-shape", path, fmt.Sprintf(`role "%s" must be a mapping with "allow" and "deny" lists`, name))
			continue
		}
		checkKeys(rules.Map, names(roleKeys...), path, fmt.Sprintf(`role "%s"`, name), &out, quoteHint)
		var role Role
		for _, key := range roleKeys {
			list, ok := rules.Map.Get(key)
			if !ok {
				continue
			}
			if !isList(list) || !allText(list) {
				out.Add("spec-shape", path, fmt.Sprintf(`"%s" of role "%s" must be a list of path patterns`, key, name))
				continue
			}
			if key == "allow" {
				role.Allow = textValues(list)
			} else {
				role.Deny = textValues(list)
			}
		}
		p.Roles[name] = role
	}
	for i, k := range branches.Map.Keys {
		pattern, role := k.Str, branches.Map.Values[i].Str
		if !roles.Map.Has(role) {
			out.Add("bad-value", path, fmt.Sprintf(`branch pattern "%s" names role "%s", which "roles" does not define`, pattern, role))
		}
		p.Branches = append(p.Branches, BranchRole{pattern, role})
	}
	if out.Errors() > 0 {
		return nil, out
	}
	return p, out
}

func allPairs(m *Map, ok func(k, v *Value) bool) bool {
	for i, k := range m.Keys {
		if !ok(k, m.Values[i]) {
			return false
		}
	}
	return true
}

// validate checks a spine's shape and every name in it, in the order tools/spine.py does.
func validate(spec *Value, path string) Findings {
	var out Findings
	if !isMap(spec) {
		out.Add("spec-shape", path, "the top level must be a mapping")
		return out
	}
	top := spec.Map
	checkKeys(top, topKeys, path, "top-level", &out, boolKeyHint)
	for _, k := range requiredKeys {
		if !top.Has(k) {
			out.Add("missing-key", path, fmt.Sprintf(`required key "%s" is missing`, k))
		}
	}
	if out.Errors() > 0 {
		return out
	}
	for _, k := range []string{"machine", "section"} {
		if v, ok := top.Get(k); ok {
			checkText(v, k, `"`+k+`"`, &out)
		}
	}

	states, _ := top.Get("states")
	events, _ := top.Get("events")
	varsValue, _ := top.Get("vars")
	variables := orEmpty(varsValue)
	if !isMap(states) || states.Map.Len() == 0 {
		out.Add("spec-shape", "states", "states must be a non-empty mapping")
		return out
	}
	checkKeys(states.Map, nil, "states", "state", &out, quoteHint)
	for i, k := range states.Map.Keys {
		if !isText(k) {
			continue
		}
		where := "states/" + k.Str
		meta := orEmpty(states.Map.Values[i])
		if !isMap(meta) {
			out.Add("spec-shape", where, "state details must be a mapping")
			continue
		}
		checkKeys(meta.Map, stateKeys, where, "state", &out, quoteHint)
		if kind, ok := meta.Map.Get("kind"); ok && (!isText(kind) || !stateKinds[kind.Str]) {
			out.Add("bad-value", where, "kind must be one of ['design', 'fact', 'policy']")
		}
	}
	if !isList(events) || !allText(events) {
		out.Add("spec-shape", "events", "events must be a list of names")
		return out
	}
	if !isMap(variables) {
		out.Add("spec-shape", "vars", "vars must map each variable to its list of values")
		return out
	}
	errorsBefore := out.Errors()
	checkKeys(variables.Map, nil, "vars", "variable", &out, quoteHint)
	varNames := map[string]bool{}
	for i, k := range variables.Map.Keys {
		if !isText(k) {
			continue
		}
		varNames[k.Str] = true
		where, values := "vars/"+k.Str, variables.Map.Values[i]
		switch {
		case !isList(values) || len(values.Items) == 0:
			out.Add("spec-shape", where, "each variable needs a non-empty list of values")
		case anyBool(values):
			out.Add("yaml-boolean-value", where, "a value was read as true/false; quote it")
		case !allText(values):
			out.Add("spec-shape", where, "variable values must be text")
		}
	}
	var known []Var // steps can't be checked against bad vars
	if out.Errors() == errorsBefore {
		known = specVars(variables)
	}
	initial, _ := top.Get("initial")
	if checkText(initial, "initial", "initial", &out) && !states.Map.Has(initial.Str) {
		out.Add("unknown-state", "initial", fmt.Sprintf(`initial state "%s" is not declared`, initial.Str))
	}

	rules, _ := top.Get("rules")
	if !isList(rules) {
		out.Add("spec-shape", "rules", "rules must be a list")
		return out
	}
	for i, rule := range rules.Items {
		where := fmt.Sprintf("rules[%d]", i+1)
		if !isMap(rule) {
			out.Add("spec-shape", where, "each rule must be a mapping")
			continue
		}
		r := rule.Map
		checkKeys(r, ruleKeys, where, "rule", &out, boolKeyHint)
		for _, k := range []string{"from", "event"} {
			if !r.Has(k) {
				out.Add("missing-key", where, fmt.Sprintf(`rule needs "%s"`, k))
			}
		}
		for _, k := range []string{"from", "event", "to"} {
			if v, ok := r.Get(k); ok {
				checkText(v, where, `"`+k+`"`, &out)
			}
		}
		var given []string
		for _, k := range outcomes {
			if r.Has(k) {
				given = append(given, k)
			}
		}
		if len(given) != 1 {
			out.Add("bad-outcome", where, "a rule needs exactly one of to, reject, ignore, stay")
		} else if v, _ := r.Get(given[0]); given[0] != "to" && !(isBool(v) && v.Bool) {
			out.Add("bad-outcome", where, given[0]+" must be true")
		}
		if when, ok := r.Get("when"); ok {
			if err := checkWhen(when, varNames); err != nil {
				out.Add("bad-condition", where, err.Error())
			}
		}
	}

	examples, hasExamples := top.Get("examples")
	if hasExamples && examples.Kind != KNull && !isList(examples) {
		out.Add("spec-shape", "examples", "examples must be a list")
		return out
	}
	if !hasExamples || examples.Kind == KNull {
		return out
	}
	for i, ex := range examples.Items {
		where := fmt.Sprintf("examples[%d]", i+1)
		if !isMap(ex) {
			out.Add("spec-shape", where, "each example must be a mapping")
			continue
		}
		checkKeys(ex.Map, names(exampleKeys...), where, "example", &out, quoteHint)
		for _, k := range exampleKeys {
			if !ex.Map.Has(k) {
				out.Add("missing-key", where, fmt.Sprintf(`example needs "%s"`, k))
			}
		}
		if name, ok := ex.Map.Get("name"); ok {
			checkText(name, where, `"name"`, &out)
		}
		if expect, ok := ex.Map.Get("expect"); ok && checkText(expect, where, `"expect"`, &out) &&
			!states.Map.Has(expect.Str) && expect.Str != "rejected" && expect.Str != "undefined" {
			out.Add("bad-value", where, fmt.Sprintf(`expect "%s" is not a declared state, rejected or undefined`, expect.Str))
		}
		if steps, ok := ex.Map.Get("steps"); ok {
			checkSteps(steps, where, &out, textValues(events), known)
		}
	}
	return out
}

func anyBool(v *Value) bool {
	for _, x := range v.Items {
		if isBool(x) {
			return true
		}
	}
	return false
}

// checkWhen is compile_condition's check of a rule's when.
func checkWhen(when *Value, varNames map[string]bool) error {
	if !isText(when) {
		return errors.New("condition must be text")
	}
	_, err := compileCondition(when.Str, varNames)
	return err
}

// specVars is the declared variables of a validated vars mapping, in their order.
func specVars(variables *Value) []Var {
	vars := []Var{}
	for i, k := range variables.Map.Keys {
		vars = append(vars, Var{k.Str, textValues(variables.Map.Values[i])})
	}
	return vars
}
