package spine

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// A condition is the language spine-format.md describes: a variable or text on each side of
// ==, !=, in or not in, a list of texts after in or not in, joined by and, or and not, and
// grouped with parentheses. Every value is text.

type condition struct{ root expr }

type expr interface {
	holds(env map[string]string) bool
}

type anyOf []expr // or
type allOf []expr // and
type negation struct{ e expr }

// comparison is left op right, or left op list when list is not nil.
type comparison struct {
	left, right operand
	op          string // "==", "!=", "in", "not in"
	list        []string
}

// operand is a variable when isVar, else text.
type operand struct {
	isVar bool
	s     string
}

func (c *condition) holds(env map[string]string) bool { return c.root.holds(env) }

func (a anyOf) holds(env map[string]string) bool {
	for _, e := range a {
		if e.holds(env) {
			return true
		}
	}
	return false
}

func (a allOf) holds(env map[string]string) bool {
	for _, e := range a {
		if !e.holds(env) {
			return false
		}
	}
	return true
}

func (n negation) holds(env map[string]string) bool { return !n.e.holds(env) }

func (o operand) value(env map[string]string) string {
	if o.isVar {
		return env[o.s]
	}
	return o.s
}

func (c comparison) holds(env map[string]string) bool {
	left := c.left.value(env)
	var yes bool
	switch {
	case c.op == "==" || c.op == "!=":
		yes = left == c.right.value(env)
	case c.list != nil:
		for _, s := range c.list {
			yes = yes || left == s
		}
	default:
		yes = strings.Contains(c.right.value(env), left) // text in text: a substring
	}
	return yes == (c.op == "==" || c.op == "in")
}

// compileCondition reads a condition and checks that each variable it names is declared.
func compileCondition(text string, variables map[string]bool) (*condition, error) {
	p := &condParser{}
	root, err := p.read(text)
	if err != nil {
		return nil, fmt.Errorf("cannot parse condition %q: %s", text, err)
	}
	for _, name := range p.names {
		if !variables[name] {
			return nil, fmt.Errorf("condition %q uses undeclared variable %q", text, name)
		}
	}
	return &condition{root}, nil
}

// condToken is a word (a name or a keyword), a text, one of ( ) [ ] , == !=, or the end.
type condToken struct {
	kind string // "word", "text", "mark", "end"
	s    string
	at   int // the character it starts at, from 1
}

func (t condToken) String() string {
	if t.kind == "text" {
		return fmt.Sprintf("text %s at character %d", strconv.Quote(t.s), t.at)
	}
	return fmt.Sprintf("%q at character %d", t.s, t.at)
}

// condTokens splits text into tokens; spaces, tabs and line breaks separate them.
func condTokens(text string) ([]condToken, error) {
	var out []condToken
	rs := []rune(text)
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			i++
		case r == '\'' || r == '"':
			end := i + 1
			for end < len(rs) && rs[end] != r {
				if rs[end] == '\\' {
					return nil, fmt.Errorf("the text at character %d holds a backslash", i+1)
				}
				end++
			}
			if end == len(rs) {
				return nil, fmt.Errorf("the text at character %d has no closing %c", i+1, r)
			}
			out = append(out, condToken{"text", string(rs[i+1 : end]), i + 1})
			i = end + 1
		case strings.ContainsRune("()[],", r):
			out = append(out, condToken{"mark", string(r), i + 1})
			i++
		case (r == '=' || r == '!') && i+1 < len(rs) && rs[i+1] == '=':
			out = append(out, condToken{"mark", string(rs[i : i+2]), i + 1})
			i += 2
		case unicode.IsLetter(r) || r == '_':
			end := i + 1
			for end < len(rs) && (unicode.IsLetter(rs[end]) || unicode.IsMark(rs[end]) || unicode.IsDigit(rs[end]) || rs[end] == '_') {
				end++
			}
			out = append(out, condToken{"word", string(rs[i:end]), i + 1})
			i = end
		default:
			return nil, fmt.Errorf("unexpected %q at character %d", string(r), i+1)
		}
	}
	return append(out, condToken{kind: "end", at: len(rs) + 1}), nil
}

type condParser struct {
	toks  []condToken
	pos   int
	names []string // the variables named, in order
}

func (p *condParser) next() condToken { return p.toks[p.pos] }

func (p *condParser) isWord(w string) bool {
	return p.next().kind == "word" && p.next().s == w
}

func (p *condParser) isMark(m string) bool {
	return p.next().kind == "mark" && p.next().s == m
}

func (p *condParser) unexpected() error {
	if p.next().kind == "end" {
		return fmt.Errorf("it ends too soon")
	}
	return fmt.Errorf("unexpected %s", p.next())
}

func (p *condParser) read(text string) (expr, error) {
	toks, err := condTokens(text)
	if err != nil {
		return nil, err
	}
	p.toks = toks
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.next().kind != "end" {
		return nil, p.unexpected()
	}
	return e, nil
}

// or: and ("or" and)*
func (p *condParser) or() (expr, error) {
	return p.joined("or", p.and, func(es []expr) expr { return anyOf(es) })
}

// and: not ("and" not)*
func (p *condParser) and() (expr, error) {
	return p.joined("and", p.not, func(es []expr) expr { return allOf(es) })
}

func (p *condParser) joined(word string, part func() (expr, error), join func([]expr) expr) (expr, error) {
	first, err := part()
	if err != nil {
		return nil, err
	}
	es := []expr{first}
	for p.isWord(word) {
		p.pos++
		e, err := part()
		if err != nil {
			return nil, err
		}
		es = append(es, e)
	}
	if len(es) == 1 {
		return first, nil
	}
	return join(es), nil
}

// not: "not" not | "(" or ")" | comparison
func (p *condParser) not() (expr, error) {
	switch {
	case p.isWord("not"):
		p.pos++
		e, err := p.not()
		if err != nil {
			return nil, err
		}
		return negation{e}, nil
	case p.isMark("("):
		p.pos++
		e, err := p.or()
		if err != nil {
			return nil, err
		}
		if !p.isMark(")") {
			return nil, p.unexpected()
		}
		p.pos++
		return e, nil
	}
	return p.comparison()
}

// comparison: operand ("==" | "!=") operand | operand ("in" | "not" "in") (operand | list)
func (p *condParser) comparison() (expr, error) {
	left, err := p.operand()
	if err != nil {
		return nil, err
	}
	var op string
	switch {
	case p.isMark("==") || p.isMark("!=") || p.isWord("in"):
		op = p.next().s
		p.pos++
	case p.isWord("not") && p.toks[p.pos+1].kind == "word" && p.toks[p.pos+1].s == "in":
		op = "not in"
		p.pos += 2
	default:
		return nil, p.unexpected()
	}
	if (op == "in" || op == "not in") && p.isMark("[") {
		list, err := p.list()
		if err != nil {
			return nil, err
		}
		return comparison{left: left, op: op, list: list}, nil
	}
	right, err := p.operand()
	if err != nil {
		return nil, err
	}
	return comparison{left: left, op: op, right: right}, nil
}

// operand: a variable's name, or a text.
func (p *condParser) operand() (operand, error) {
	t := p.next()
	switch {
	case t.kind == "text":
		p.pos++
		return operand{s: t.s}, nil
	case t.kind == "word" && t.s != "and" && t.s != "or" && t.s != "not" && t.s != "in":
		p.pos++
		p.names = append(p.names, t.s)
		return operand{isVar: true, s: t.s}, nil
	}
	return operand{}, p.unexpected()
}

// list: "[" text ("," text)* "]"
func (p *condParser) list() ([]string, error) {
	p.pos++ // the "["
	var out []string
	for {
		if p.next().kind != "text" {
			return nil, p.unexpected()
		}
		out = append(out, p.next().s)
		p.pos++
		switch {
		case p.isMark(","):
			p.pos++
		case p.isMark("]"):
			p.pos++
			return out, nil
		default:
			return nil, p.unexpected()
		}
	}
}
