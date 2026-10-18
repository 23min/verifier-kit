package spine

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Conditions are Python expressions, which tools/spine.py parses with ast.parse(mode="eval")
// and then checks node by node. This file is a parser for Python's expression grammar that
// gives the same verdict: text Python can't parse is "cannot parse", and the node classes the
// check looks at carry their Python names, which appear in messages. Only the small allowed
// language is ever evaluated.

// node is an ast node. kind is the Python class name.
type node struct {
	kind string
	op   string   // BoolOp "and"/"or"; UnaryOp "not", "-", "+", "~"
	ops  []string // Compare
	kids []*node  // BoolOp values; UnaryOp operand; Compare left then comparators; List/Tuple elements
	id   string   // Name
	c    constant // Constant
}

// constant is an ast.Constant's value: "str", "int", "float", "bool", "none", "bytes",
// "complex" or "ellipsis".
type constant struct {
	kind string
	s    string
	i    *big.Int
	f    float64
	b    bool
}

var errSyntax = errors.New("syntax error")

// ---------- tokens ----------

type tokKind int

const (
	tName tokKind = iota
	tNumber
	tString
	tOp
	tNewline
	tEnd
)

type token struct {
	kind tokKind
	text string // name, operator, or the number's text
	str  strLit // tString
	num  constant
}

type strLit struct {
	value  string
	bytes  bool
	fmt    bool
	prefix string
}

var keywords = map[string]bool{
	"False": true, "None": true, "True": true, "and": true, "as": true, "assert": true, "async": true,
	"await": true, "break": true, "class": true, "continue": true, "def": true, "del": true, "elif": true,
	"else": true, "except": true, "finally": true, "for": true, "from": true, "global": true, "if": true,
	"import": true, "in": true, "is": true, "lambda": true, "nonlocal": true, "not": true, "or": true,
	"pass": true, "raise": true, "return": true, "try": true, "while": true, "with": true, "yield": true,
}

var operators = []string{
	"**=", "//=", ">>=", "<<=", "...", "!=", "%=", "&=", "**", "*=", "+=", "-=", "->", "//", "/=", ":=",
	"<<", "<=", "==", ">=", ">>", "@=", "^=", "|=", "%", "&", "(", ")", "*", "+", ",", "-", ".", "/",
	":", ";", "<", "=", ">", "@", "[", "]", "^", "{", "|", "}", "~",
}

func idStart(r rune) bool {
	return r == '_' || (r < 0x80 && unicode.IsLetter(r)) || (r >= 0x80 && (unicode.IsLetter(r) || unicode.Is(unicode.Nl, r)))
}

func idContinue(r rune) bool {
	return idStart(r) || (r >= '0' && r <= '9') ||
		(r >= 0x80 && (unicode.IsDigit(r) || unicode.In(r, unicode.Mn, unicode.Mc, unicode.Pc, unicode.Nd)))
}

type lexer struct {
	src   []rune
	pos   int
	depth []rune // open brackets
	toks  []token
}

func tokenize(text string) ([]token, error) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if strings.ContainsRune(text, 0) {
		return nil, errSyntax
	}
	lx := &lexer{src: []rune(text)}
	if err := lx.run(); err != nil {
		return nil, err
	}
	return lx.toks, nil
}

func (lx *lexer) peek(off int) rune {
	if lx.pos+off < len(lx.src) {
		return lx.src[lx.pos+off]
	}
	return -1
}

// run tokenizes one logical line of an eval input: blank lines before it, its tokens, a
// NEWLINE, and then only blank lines.
func (lx *lexer) run() error {
	atLineStart, done := true, false
	for {
		if atLineStart && len(lx.depth) == 0 {
			// indentation runs across continuations and a form feed resets it; a blank
			// line (spaces, a comment) is skipped; content must not be indented
			col, i, joined := 0, lx.pos, false
			for {
				for i < len(lx.src) && (lx.src[i] == ' ' || lx.src[i] == '\t' || lx.src[i] == '\f') {
					if lx.src[i] == '\f' {
						col = 0
					} else {
						col++
					}
					i++
				}
				if i+1 < len(lx.src) && lx.src[i] == '\\' && lx.src[i+1] == '\n' {
					i += 2
					joined = true
					continue
				}
				break
			}
			if i >= len(lx.src) {
				if col != 0 || joined {
					return errSyntax // an indented last line, or a continuation into nothing
				}
				lx.pos = i
				break
			}
			if lx.src[i] == '\n' {
				lx.pos = i + 1
				continue
			}
			if lx.src[i] == '#' {
				for i < len(lx.src) && lx.src[i] != '\n' {
					i++
				}
				lx.pos = i
				continue
			}
			if done || col != 0 {
				return errSyntax // more content, or an indented first line
			}
			lx.pos = i
			atLineStart = false
		}
		if lx.pos >= len(lx.src) {
			break
		}
		r := lx.src[lx.pos]
		switch {
		case r == ' ' || r == '\t' || r == '\f':
			lx.pos++
		case r == '\n':
			lx.pos++
			if len(lx.depth) == 0 {
				if !done {
					lx.toks = append(lx.toks, token{kind: tNewline})
					done = true
				}
				atLineStart = true
			}
		case r == '#':
			for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
				lx.pos++
			}
		case r == '\\':
			if lx.peek(1) != '\n' {
				return errSyntax
			}
			lx.pos += 2
			if lx.pos >= len(lx.src) {
				return errSyntax // a continuation needs a next line
			}
		default:
			if done {
				return errSyntax
			}
			if err := lx.token(); err != nil {
				return err
			}
		}
	}
	if len(lx.depth) > 0 {
		return errSyntax // never closed
	}
	if !done {
		lx.toks = append(lx.toks, token{kind: tNewline})
	}
	lx.toks = append(lx.toks, token{kind: tEnd})
	return nil
}

var stringPrefixes = map[string]bool{
	"r": true, "u": true, "f": true, "b": true, "br": true, "rb": true, "fr": true, "rf": true,
}

func (lx *lexer) token() error {
	r := lx.src[lx.pos]
	switch {
	case r == '\'' || r == '"':
		return lx.str("")
	case r >= '0' && r <= '9' || (r == '.' && lx.peek(1) >= '0' && lx.peek(1) <= '9'):
		return lx.number()
	case idStart(r):
		start := lx.pos
		for lx.pos < len(lx.src) && idContinue(lx.src[lx.pos]) {
			lx.pos++
		}
		word := string(lx.src[start:lx.pos])
		if q := lx.peek(0); (q == '\'' || q == '"') && stringPrefixes[strings.ToLower(word)] {
			return lx.str(word)
		}
		lx.toks = append(lx.toks, token{kind: tName, text: word})
		return nil
	}
	for _, op := range operators {
		if lx.hasPrefix(op) {
			lx.pos += utf8.RuneCountInString(op)
			switch op {
			case "(", "[", "{":
				if len(lx.depth) >= 200 {
					return errSyntax // too many nested parentheses
				}
				lx.depth = append(lx.depth, rune(op[0]))
			case ")", "]", "}":
				open := map[string]rune{")": '(', "]": '[', "}": '{'}[op]
				if len(lx.depth) == 0 || lx.depth[len(lx.depth)-1] != open {
					return errSyntax
				}
				lx.depth = lx.depth[:len(lx.depth)-1]
			}
			lx.toks = append(lx.toks, token{kind: tOp, text: op})
			return nil
		}
	}
	return errSyntax // invalid character, '$', '?', '!' alone, a non-printable character
}

func (lx *lexer) hasPrefix(s string) bool {
	i := lx.pos
	for _, r := range s {
		if i >= len(lx.src) || lx.src[i] != r {
			return false
		}
		i++
	}
	return true
}

func isDigitIn(r rune, base int) bool {
	switch base {
	case 2:
		return r == '0' || r == '1'
	case 8:
		return r >= '0' && r <= '7'
	case 16:
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
	}
	return r >= '0' && r <= '9'
}

// digits reads digits of a base with single underscores between them.
func (lx *lexer) digits(base int) (string, bool) {
	var b strings.Builder
	if !isDigitIn(lx.peek(0), base) {
		return "", false
	}
	for {
		for isDigitIn(lx.peek(0), base) {
			b.WriteRune(lx.src[lx.pos])
			lx.pos++
		}
		if lx.peek(0) == '_' && isDigitIn(lx.peek(1), base) {
			lx.pos++
			continue
		}
		if lx.peek(0) == '_' {
			return "", false
		}
		return b.String(), true
	}
}

// number reads a Python numeric literal and checks what follows it.
func (lx *lexer) number() error {
	var c constant
	if lx.peek(0) == '0' && strings.ContainsRune("xXoObB", lx.peek(1)) {
		base := map[rune]int{'x': 16, 'X': 16, 'o': 8, 'O': 8, 'b': 2, 'B': 2}[lx.peek(1)]
		lx.pos += 2
		if lx.peek(0) == '_' {
			lx.pos++
		}
		d, ok := lx.digits(base)
		if !ok {
			return errSyntax
		}
		i, _ := new(big.Int).SetString(d, base)
		c = constant{kind: "int", i: i}
	} else {
		var intPart, frac, exp string
		ok := true
		if lx.peek(0) != '.' {
			intPart, ok = lx.digits(10)
			if !ok {
				return errSyntax
			}
		}
		isFloat := false
		if lx.peek(0) == '.' {
			isFloat = true
			lx.pos++
			if isDigitIn(lx.peek(0), 10) {
				if frac, ok = lx.digits(10); !ok {
					return errSyntax
				}
			}
		}
		if e := lx.peek(0); e == 'e' || e == 'E' {
			save := lx.pos
			lx.pos++
			sign := ""
			if s := lx.peek(0); s == '+' || s == '-' {
				sign = string(s)
				lx.pos++
			}
			d, ok := lx.digits(10)
			if !ok {
				lx.pos = save
				return errSyntax // "1e" is an invalid decimal literal
			}
			exp = "e" + sign + d
			isFloat = true
		}
		if j := lx.peek(0); j == 'j' || j == 'J' {
			lx.pos++
			c = constant{kind: "complex"}
		} else if isFloat {
			f, err := strconv.ParseFloat(intPart+"."+frac+exp, 64)
			if err != nil && !errors.Is(err, strconv.ErrRange) {
				return errSyntax
			}
			c = constant{kind: "float", f: f}
		} else {
			if len(intPart) > 1 && intPart[0] == '0' && strings.Trim(intPart, "0") != "" {
				return errSyntax // leading zeros in a decimal integer
			}
			i, _ := new(big.Int).SetString(intPart, 10)
			c = constant{kind: "int", i: i}
		}
	}
	// a number may not run into a name, except one of a few keywords
	if idStart(lx.peek(0)) || (lx.peek(0) >= '0' && lx.peek(0) <= '9') {
		rest := string(lx.src[lx.pos:min(lx.pos+6, len(lx.src))])
		allowed := false
		for _, kw := range []string{"and", "else", "for", "if", "in", "is", "not", "or"} {
			if strings.HasPrefix(rest, kw) {
				allowed = true
			}
		}
		if !allowed {
			return errSyntax
		}
	}
	lx.toks = append(lx.toks, token{kind: tNumber, num: c})
	return nil
}

// str reads a string literal after its prefix.
func (lx *lexer) str(prefix string) error {
	p := strings.ToLower(prefix)
	raw, isBytes, isFmt := strings.Contains(p, "r"), strings.Contains(p, "b"), strings.Contains(p, "f")
	q := lx.src[lx.pos]
	triple := lx.peek(1) == q && lx.peek(2) == q
	if triple {
		lx.pos += 3
	} else {
		lx.pos++
	}
	var body []rune
	depth := 0 // brace depth inside an f-string
	for {
		if lx.pos >= len(lx.src) {
			return errSyntax
		}
		r := lx.src[lx.pos]
		if isFmt && depth > 0 && (r == '\'' || r == '"') {
			// a string inside a replacement field (allowed since Python 3.12)
			save := len(lx.toks)
			if err := lx.str(""); err != nil {
				return err
			}
			lx.toks = lx.toks[:save]
			continue
		}
		if r == q && depth == 0 && (!triple || (lx.peek(1) == q && lx.peek(2) == q)) {
			if triple {
				lx.pos += 3
			} else {
				lx.pos++
			}
			break
		}
		if r == '\n' && !triple && depth == 0 {
			return errSyntax // unterminated string literal
		}
		if r == '\\' {
			if lx.pos+1 >= len(lx.src) {
				return errSyntax
			}
			body = append(body, r, lx.src[lx.pos+1])
			lx.pos += 2
			continue
		}
		if isFmt {
			if r == '{' {
				if lx.peek(1) == '{' && depth == 0 {
					body = append(body, '{', '{')
					lx.pos += 2
					continue
				}
				depth++
			} else if r == '}' {
				if depth == 0 && lx.peek(1) == '}' {
					body = append(body, '}', '}')
					lx.pos += 2
					continue
				}
				if depth == 0 {
					return errSyntax // a single '}' is not allowed
				}
				depth--
			}
		}
		body = append(body, r)
		lx.pos++
	}
	value := string(body)
	if !raw {
		var err error
		if value, err = unescape(body, isBytes); err != nil {
			return err
		}
	}
	if isBytes {
		for _, r := range body {
			if r > 0x7f {
				return errSyntax // bytes can only contain ASCII literal characters
			}
		}
	}
	lx.toks = append(lx.toks, token{kind: tString, str: strLit{value: value, bytes: isBytes, fmt: isFmt, prefix: p}})
	return nil
}

// unescape processes the backslash escapes of a non-raw literal.
func unescape(body []rune, isBytes bool) (string, error) {
	var b strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			b.WriteRune(body[i])
			continue
		}
		i++
		c := body[i]
		switch c {
		case '\n':
		case '\\', '\'', '"':
			b.WriteRune(c)
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 'f':
			b.WriteByte(12)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte(11)
		case '0', '1', '2', '3', '4', '5', '6', '7':
			n, j := 0, 0
			for j < 3 && i+j < len(body) && body[i+j] >= '0' && body[i+j] <= '7' {
				n = n*8 + int(body[i+j]-'0')
				j++
			}
			i += j - 1
			b.WriteRune(rune(n))
		case 'x':
			if i+2 >= len(body)+0 || !isHexRunes(body[i+1:min(i+3, len(body))], 2) {
				return "", errSyntax
			}
			n, _ := strconv.ParseUint(string(body[i+1:i+3]), 16, 32)
			b.WriteRune(rune(n))
			i += 2
		case 'N', 'u', 'U':
			if isBytes {
				b.WriteRune('\\')
				b.WriteRune(c)
				continue
			}
			if c == 'N' { // a named character; Go has no name table, so keep the text
				end := -1
				if i+1 < len(body) && body[i+1] == '{' {
					for j := i + 2; j < len(body); j++ {
						if body[j] == '}' {
							end = j
							break
						}
					}
				}
				if end < 0 || end == i+2 {
					return "", errSyntax
				}
				b.WriteString(string(body[i-1 : end+1]))
				i = end
				continue
			}
			width := 4
			if c == 'U' {
				width = 8
			}
			if i+width >= len(body)+0 || !isHexRunes(body[i+1:min(i+1+width, len(body))], width) {
				return "", errSyntax
			}
			n, _ := strconv.ParseUint(string(body[i+1:i+1+width]), 16, 32)
			if n > 0x10ffff {
				return "", errSyntax
			}
			b.WriteRune(rune(n))
			i += width
		default:
			b.WriteRune('\\')
			b.WriteRune(c)
		}
	}
	return b.String(), nil
}

func isHexRunes(rs []rune, n int) bool {
	if len(rs) != n {
		return false
	}
	for _, r := range rs {
		if !isDigitIn(r, 16) {
			return false
		}
	}
	return true
}

// ---------- parser ----------

type parser struct {
	toks []token
	pos  int
}

func (p *parser) tok() token { return p.toks[p.pos] }

func (p *parser) isOp(op string) bool {
	t := p.tok()
	return t.kind == tOp && t.text == op
}

func (p *parser) isKw(kw string) bool {
	t := p.tok()
	return t.kind == tName && t.text == kw
}

func (p *parser) acceptOp(op string) bool {
	if p.isOp(op) {
		p.pos++
		return true
	}
	return false
}

func (p *parser) acceptKw(kw string) bool {
	if p.isKw(kw) {
		p.pos++
		return true
	}
	return false
}

func (p *parser) expectOp(op string) error {
	if !p.acceptOp(op) {
		return errSyntax
	}
	return nil
}

func other(kind string) *node { return &node{kind: kind} }

// parseCondition is ast.parse(text, mode="eval").body.
func parseCondition(text string) (*node, error) {
	toks, err := tokenize(text)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	n, err := p.expressions()
	if err != nil {
		return nil, err
	}
	if p.tok().kind != tNewline {
		return nil, errSyntax
	}
	p.pos++
	if p.tok().kind != tEnd {
		return nil, errSyntax
	}
	return n, nil
}

// startsExpression reports whether the current token can begin an expression.
func (p *parser) startsExpression() bool {
	t := p.tok()
	switch t.kind {
	case tNumber, tString:
		return true
	case tName:
		return !keywords[t.text] || t.text == "True" || t.text == "False" || t.text == "None" ||
			t.text == "not" || t.text == "lambda" || t.text == "await"
	case tOp:
		switch t.text {
		case "(", "[", "{", "-", "+", "~", "...":
			return true
		}
	}
	return false
}

// expressions: expression (',' expression)* [','] — a Tuple when there is a comma.
func (p *parser) expressions() (*node, error) {
	first, err := p.expression()
	if err != nil {
		return nil, err
	}
	if !p.isOp(",") {
		return first, nil
	}
	elts := []*node{first}
	for p.acceptOp(",") {
		if !p.startsExpression() {
			break
		}
		e, err := p.expression()
		if err != nil {
			return nil, err
		}
		elts = append(elts, e)
	}
	return &node{kind: "Tuple", kids: elts}, nil
}

func (p *parser) expression() (*node, error) {
	if p.isKw("lambda") {
		return p.lambda()
	}
	body, err := p.disjunction()
	if err != nil {
		return nil, err
	}
	if p.acceptKw("if") {
		if _, err := p.disjunction(); err != nil {
			return nil, err
		}
		if !p.acceptKw("else") {
			return nil, errSyntax
		}
		if _, err := p.expression(); err != nil {
			return nil, err
		}
		return other("IfExp"), nil
	}
	_ = body
	return body, nil
}

// lambda: 'lambda' [params] ':' expression.
func (p *parser) lambda() (*node, error) {
	p.pos++
	seenDefault, seenStar := false, false
	for !p.isOp(":") {
		switch {
		case p.acceptOp("/"):
		case p.acceptOp("**"):
			if p.tok().kind != tName || keywords[p.tok().text] {
				return nil, errSyntax
			}
			p.pos++
		case p.acceptOp("*"):
			seenStar = true
			if p.tok().kind == tName && !keywords[p.tok().text] {
				p.pos++
			}
		case p.tok().kind == tName && !keywords[p.tok().text]:
			p.pos++
			if p.acceptOp("=") {
				if _, err := p.expression(); err != nil {
					return nil, err
				}
				seenDefault = true
			} else if seenDefault && !seenStar {
				return nil, errSyntax // non-default argument follows default argument
			}
		default:
			return nil, errSyntax
		}
		if !p.acceptOp(",") {
			break
		}
	}
	if err := p.expectOp(":"); err != nil {
		return nil, err
	}
	if _, err := p.expression(); err != nil {
		return nil, err
	}
	return other("Lambda"), nil
}

func (p *parser) disjunction() (*node, error) {
	first, err := p.conjunction()
	if err != nil {
		return nil, err
	}
	if !p.isKw("or") {
		return first, nil
	}
	values := []*node{first}
	for p.acceptKw("or") {
		v, err := p.conjunction()
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return &node{kind: "BoolOp", op: "or", kids: values}, nil
}

func (p *parser) conjunction() (*node, error) {
	first, err := p.inversion()
	if err != nil {
		return nil, err
	}
	if !p.isKw("and") {
		return first, nil
	}
	values := []*node{first}
	for p.acceptKw("and") {
		v, err := p.inversion()
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return &node{kind: "BoolOp", op: "and", kids: values}, nil
}

func (p *parser) inversion() (*node, error) {
	if p.acceptKw("not") {
		operand, err := p.inversion()
		if err != nil {
			return nil, err
		}
		return &node{kind: "UnaryOp", op: "not", kids: []*node{operand}}, nil
	}
	return p.comparison()
}

func (p *parser) compareOp() string {
	t := p.tok()
	if t.kind == tOp {
		switch t.text {
		case "==", "!=", "<", "<=", ">", ">=":
			p.pos++
			return t.text
		}
	}
	if t.kind == tName {
		switch t.text {
		case "in":
			p.pos++
			return "in"
		case "not":
			if n := p.toks[p.pos+1]; n.kind == tName && n.text == "in" {
				p.pos += 2
				return "not in"
			}
		case "is":
			p.pos++
			if p.acceptKw("not") {
				return "is not"
			}
			return "is"
		}
	}
	return ""
}

func (p *parser) comparison() (*node, error) {
	left, err := p.binary(0)
	if err != nil {
		return nil, err
	}
	var ops []string
	kids := []*node{left}
	for {
		op := p.compareOp()
		if op == "" {
			break
		}
		right, err := p.binary(0)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
		kids = append(kids, right)
	}
	if ops == nil {
		return left, nil
	}
	return &node{kind: "Compare", ops: ops, kids: kids}, nil
}

// binary handles the binary operators by precedence, lowest first.
var binaryLevels = [][]string{{"|"}, {"^"}, {"&"}, {"<<", ">>"}, {"+", "-"}, {"*", "/", "//", "%", "@"}}

func (p *parser) binary(level int) (*node, error) {
	if level == len(binaryLevels) {
		return p.factor()
	}
	left, err := p.binary(level + 1)
	if err != nil {
		return nil, err
	}
	for {
		matched := false
		for _, op := range binaryLevels[level] {
			if p.acceptOp(op) {
				matched = true
				break
			}
		}
		if !matched {
			return left, nil
		}
		if _, err := p.binary(level + 1); err != nil {
			return nil, err
		}
		left = other("BinOp")
	}
}

func (p *parser) factor() (*node, error) {
	for _, op := range []string{"+", "-", "~"} {
		if p.acceptOp(op) {
			operand, err := p.factor()
			if err != nil {
				return nil, err
			}
			return &node{kind: "UnaryOp", op: op, kids: []*node{operand}}, nil
		}
	}
	return p.power()
}

func (p *parser) power() (*node, error) {
	var base *node
	var err error
	if p.acceptKw("await") {
		if _, err = p.primary(); err != nil {
			return nil, err
		}
		base = other("Await")
	} else if base, err = p.primary(); err != nil {
		return nil, err
	}
	if p.acceptOp("**") {
		if _, err := p.factor(); err != nil {
			return nil, err
		}
		return other("BinOp"), nil
	}
	return base, nil
}

func (p *parser) primary() (*node, error) {
	n, err := p.atom()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.acceptOp("."):
			if p.tok().kind != tName || keywords[p.tok().text] {
				return nil, errSyntax
			}
			p.pos++
			n = other("Attribute")
		case p.acceptOp("("):
			if err := p.arguments(); err != nil {
				return nil, err
			}
			n = other("Call")
		case p.acceptOp("["):
			if err := p.slices(); err != nil {
				return nil, err
			}
			n = other("Subscript")
		default:
			return n, nil
		}
	}
}

// arguments: a call's arguments up to ')'.
func (p *parser) arguments() error {
	if p.acceptOp(")") {
		return nil
	}
	seenKeyword, seenDoubleStar, count := false, false, 0
	for {
		count++
		switch {
		case p.acceptOp("**"):
			if _, err := p.expression(); err != nil {
				return err
			}
			seenDoubleStar = true
		case p.acceptOp("*"):
			if seenDoubleStar {
				return errSyntax
			}
			if _, err := p.expression(); err != nil {
				return err
			}
		case p.tok().kind == tName && !keywords[p.tok().text] && p.toks[p.pos+1].kind == tOp && p.toks[p.pos+1].text == "=":
			p.pos += 2
			if _, err := p.expression(); err != nil {
				return err
			}
			seenKeyword = true
		default:
			if seenKeyword || seenDoubleStar {
				return errSyntax // positional argument follows keyword argument
			}
			if _, err := p.namedExpression(); err != nil {
				return err
			}
			if p.isKw("for") || p.isKw("async") {
				if err := p.comprehension(); err != nil {
					return err
				}
				if count > 1 || !p.isOp(")") {
					return errSyntax // a generator argument must be the only one
				}
			}
		}
		if p.acceptOp(")") {
			return nil
		}
		if err := p.expectOp(","); err != nil {
			return err
		}
		if p.acceptOp(")") {
			return nil
		}
	}
}

// slices: what goes between a subscript's brackets, up to ']'.
func (p *parser) slices() error {
	for {
		if err := p.slice(); err != nil {
			return err
		}
		if p.acceptOp("]") {
			return nil
		}
		if err := p.expectOp(","); err != nil {
			return err
		}
		if p.acceptOp("]") {
			return nil
		}
	}
}

func (p *parser) slice() error {
	if p.acceptOp("*") {
		_, err := p.binary(0)
		return err
	}
	if !p.isOp(":") {
		if _, err := p.namedExpression(); err != nil {
			return err
		}
		if !p.isOp(":") {
			return nil
		}
	}
	for i := 0; i < 2 && p.acceptOp(":"); i++ {
		if !p.isOp(":") && !p.isOp("]") && !p.isOp(",") {
			if _, err := p.expression(); err != nil {
				return err
			}
		}
	}
	return nil
}

// namedExpression: NAME ':=' expression, or an expression.
func (p *parser) namedExpression() (*node, error) {
	if t := p.tok(); t.kind == tName && !keywords[t.text] && p.toks[p.pos+1].kind == tOp && p.toks[p.pos+1].text == ":=" {
		p.pos += 2
		if _, err := p.expression(); err != nil {
			return nil, err
		}
		return other("NamedExpr"), nil
	}
	n, err := p.expression()
	if err != nil {
		return nil, err
	}
	if p.isOp(":=") {
		return nil, errSyntax // only a name can be assigned
	}
	return n, nil
}

// starNamed: '*' bitwise_or (a Starred element), or a named expression.
func (p *parser) starNamed() (*node, error) {
	if p.acceptOp("*") {
		if _, err := p.binary(0); err != nil {
			return nil, err
		}
		return other("Starred"), nil
	}
	return p.namedExpression()
}

// comprehension: one or more `for target in iterable` clauses, each with optional ifs.
func (p *parser) comprehension() error {
	for p.isKw("for") || p.isKw("async") {
		p.acceptKw("async")
		if !p.acceptKw("for") {
			return errSyntax
		}
		if err := p.targets(); err != nil {
			return err
		}
		if !p.acceptKw("in") {
			return errSyntax
		}
		if _, err := p.disjunction(); err != nil {
			return err
		}
		for p.acceptKw("if") {
			if _, err := p.disjunction(); err != nil {
				return err
			}
		}
	}
	return nil
}

// targets: what a comprehension assigns to; anything else is "cannot assign".
func (p *parser) targets() error {
	for {
		if err := p.target(); err != nil {
			return err
		}
		if !p.acceptOp(",") || p.isKw("in") {
			return nil
		}
	}
}

func (p *parser) target() error {
	if p.acceptOp("*") {
		return p.target()
	}
	if p.isOp("(") || p.isOp("[") {
		closer := map[string]string{"(": ")", "[": "]"}[p.tok().text]
		p.pos++
		for !p.acceptOp(closer) {
			if err := p.target(); err != nil {
				return err
			}
			if !p.acceptOp(",") {
				return p.expectOp(closer)
			}
		}
		return nil
	}
	start := p.pos
	n, err := p.primary()
	if err != nil {
		return err
	}
	switch n.kind {
	case "Name", "Attribute", "Subscript":
		return nil
	}
	_ = start
	return errSyntax // cannot assign to a literal, a call, ...
}

func (p *parser) atom() (*node, error) {
	t := p.tok()
	switch t.kind {
	case tNumber:
		p.pos++
		return &node{kind: "Constant", c: t.num}, nil
	case tString:
		return p.strings()
	case tName:
		switch t.text {
		case "True", "False":
			p.pos++
			return &node{kind: "Constant", c: constant{kind: "bool", b: t.text == "True"}}, nil
		case "None":
			p.pos++
			return &node{kind: "Constant", c: constant{kind: "none"}}, nil
		}
		if keywords[t.text] {
			return nil, errSyntax
		}
		p.pos++
		return &node{kind: "Name", id: t.text}, nil
	case tOp:
		switch t.text {
		case "...":
			p.pos++
			return &node{kind: "Constant", c: constant{kind: "ellipsis"}}, nil
		case "(":
			p.pos++
			return p.paren()
		case "[":
			p.pos++
			return p.bracket()
		case "{":
			p.pos++
			return p.brace()
		}
	}
	return nil, errSyntax
}

// strings: adjacent literals concatenated, as the parser does.
func (p *parser) strings() (*node, error) {
	var parts []strLit
	for p.tok().kind == tString {
		parts = append(parts, p.tok().str)
		p.pos++
	}
	anyBytes, anyText, anyFmt := false, false, false
	var b strings.Builder
	for _, s := range parts {
		if s.bytes {
			anyBytes = true
		} else {
			anyText = true
		}
		if s.fmt {
			anyFmt = true
		}
		b.WriteString(s.value)
	}
	if anyBytes && anyText {
		return nil, errSyntax // cannot mix bytes and nonbytes literals
	}
	if anyFmt {
		return other("JoinedStr"), nil
	}
	if anyBytes {
		return &node{kind: "Constant", c: constant{kind: "bytes", s: b.String()}}, nil
	}
	return &node{kind: "Constant", c: constant{kind: "str", s: b.String()}}, nil
}

// paren: after '(' — an empty tuple, a yield, a group, a tuple or a generator.
func (p *parser) paren() (*node, error) {
	if p.acceptOp(")") {
		return &node{kind: "Tuple"}, nil
	}
	if p.isKw("yield") {
		p.pos++
		kind := "Yield"
		if p.acceptKw("from") {
			kind = "YieldFrom"
			if _, err := p.expression(); err != nil {
				return nil, err
			}
		} else if !p.isOp(")") {
			if _, err := p.starExpressions(); err != nil {
				return nil, err
			}
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
		return other(kind), nil
	}
	first, err := p.starNamed()
	if err != nil {
		return nil, err
	}
	if p.isKw("for") || p.isKw("async") {
		if first.kind == "Starred" {
			return nil, errSyntax
		}
		if err := p.comprehension(); err != nil {
			return nil, err
		}
		if err := p.expectOp(")"); err != nil {
			return nil, err
		}
		return other("GeneratorExp"), nil
	}
	if p.acceptOp(")") {
		if first.kind == "Starred" {
			return nil, errSyntax // a starred expression needs a comma here
		}
		return first, nil
	}
	if err := p.expectOp(","); err != nil {
		return nil, err
	}
	elts := []*node{first}
	for !p.acceptOp(")") {
		e, err := p.starNamed()
		if err != nil {
			return nil, err
		}
		elts = append(elts, e)
		if !p.acceptOp(",") {
			if err := p.expectOp(")"); err != nil {
				return nil, err
			}
			break
		}
	}
	return &node{kind: "Tuple", kids: elts}, nil
}

// starExpressions: what a bare yield may give.
func (p *parser) starExpressions() (*node, error) {
	for {
		if _, err := p.starNamed(); err != nil {
			return nil, err
		}
		if !p.acceptOp(",") || p.isOp(")") {
			return other("Tuple"), nil
		}
	}
}

// bracket: after '[' — a list or a list comprehension.
func (p *parser) bracket() (*node, error) {
	if p.acceptOp("]") {
		return &node{kind: "List"}, nil
	}
	first, err := p.starNamed()
	if err != nil {
		return nil, err
	}
	if p.isKw("for") || p.isKw("async") {
		if first.kind == "Starred" {
			return nil, errSyntax
		}
		if err := p.comprehension(); err != nil {
			return nil, err
		}
		if err := p.expectOp("]"); err != nil {
			return nil, err
		}
		return other("ListComp"), nil
	}
	elts := []*node{first}
	for p.acceptOp(",") {
		if p.isOp("]") {
			break
		}
		e, err := p.starNamed()
		if err != nil {
			return nil, err
		}
		elts = append(elts, e)
	}
	if err := p.expectOp("]"); err != nil {
		return nil, err
	}
	return &node{kind: "List", kids: elts}, nil
}

// brace: after '{' — a dict, a set, or their comprehensions.
func (p *parser) brace() (*node, error) {
	if p.acceptOp("}") {
		return other("Dict"), nil
	}
	if p.acceptOp("**") { // a dict that starts by unpacking another
		if _, err := p.binary(0); err != nil {
			return nil, err
		}
		return p.dictRest()
	}
	first, err := p.starNamed()
	if err != nil {
		return nil, err
	}
	if p.acceptOp(":") {
		if first.kind == "Starred" || first.kind == "NamedExpr" {
			return nil, errSyntax
		}
		if _, err := p.expression(); err != nil {
			return nil, err
		}
		if p.isKw("for") || p.isKw("async") {
			if err := p.comprehension(); err != nil {
				return nil, err
			}
			if err := p.expectOp("}"); err != nil {
				return nil, err
			}
			return other("DictComp"), nil
		}
		return p.dictRest()
	}
	if p.isKw("for") || p.isKw("async") {
		if first.kind == "Starred" {
			return nil, errSyntax
		}
		if err := p.comprehension(); err != nil {
			return nil, err
		}
		if err := p.expectOp("}"); err != nil {
			return nil, err
		}
		return other("SetComp"), nil
	}
	for p.acceptOp(",") {
		if p.isOp("}") {
			break
		}
		if _, err := p.starNamed(); err != nil {
			return nil, err
		}
	}
	if err := p.expectOp("}"); err != nil {
		return nil, err
	}
	return other("Set"), nil
}

// dictRest: the rest of a dict display after its first item.
func (p *parser) dictRest() (*node, error) {
	for p.acceptOp(",") {
		if p.isOp("}") {
			break
		}
		if p.acceptOp("**") {
			if _, err := p.binary(0); err != nil {
				return nil, err
			}
			continue
		}
		if _, err := p.expression(); err != nil {
			return nil, err
		}
		if err := p.expectOp(":"); err != nil {
			return nil, err
		}
		if _, err := p.expression(); err != nil {
			return nil, err
		}
	}
	if err := p.expectOp("}"); err != nil {
		return nil, err
	}
	return other("Dict"), nil
}

// ---------- the check and evaluation, as in tools/spine.py ----------

type condition struct {
	text string
	root *node
}

// compileCondition is compile_condition: parse, then allow only the small language.
func compileCondition(text string, variables map[string]bool) (*condition, error) {
	root, err := parseCondition(text)
	if err != nil {
		return nil, fmt.Errorf("cannot parse condition %s", pyRepr(text))
	}
	if err := checkCondition(root, variables, text); err != nil {
		return nil, err
	}
	return &condition{text, root}, nil
}

func isLiteral(n *node) bool {
	if n.kind != "Constant" {
		return false
	}
	switch n.c.kind {
	case "str", "int", "float", "bool":
		return true
	}
	return false
}

func isTextNode(n *node) bool {
	return n.kind == "Name" || (n.kind == "Constant" && n.c.kind == "str")
}

func checkCondition(n *node, variables map[string]bool, text string) error {
	switch {
	case n.kind == "BoolOp":
		for _, v := range n.kids {
			if err := checkCondition(v, variables, text); err != nil {
				return err
			}
		}
	case n.kind == "UnaryOp" && n.op == "not":
		return checkCondition(n.kids[0], variables, text)
	case n.kind == "Compare":
		for _, op := range n.ops {
			switch op {
			case "==", "!=", "in", "not in":
			default:
				return fmt.Errorf("only ==, !=, in and not in are allowed in %s", pyRepr(text))
			}
		}
		for _, part := range n.kids {
			if err := checkCondition(part, variables, text); err != nil {
				return err
			}
		}
		for i, op := range n.ops {
			left, right := n.kids[i], n.kids[i+1]
			if (op == "in" || op == "not in") && !(right.kind == "List" || right.kind == "Tuple" || (isTextNode(left) && isTextNode(right))) {
				return fmt.Errorf("`in` needs a list on its right, or text or a variable on both sides, in %s", pyRepr(text))
			}
		}
	case n.kind == "Name":
		if !variables[n.id] {
			return fmt.Errorf("condition %s uses undeclared variable \"%s\"", pyRepr(text), n.id)
		}
	case isLiteral(n):
	case n.kind == "List" || n.kind == "Tuple":
		for _, e := range n.kids {
			if !isLiteral(e) {
				return fmt.Errorf("lists may hold only literals in %s", pyRepr(text))
			}
		}
	default:
		return fmt.Errorf("unsupported element %s in %s", n.kind, pyRepr(text))
	}
	return nil
}

// cv is a value while evaluating: text, a number, a bool or a list of those.
type cv struct {
	kind string // "str", "int", "float", "bool", "list"
	s    string
	i    *big.Int
	f    float64
	b    bool
	list []cv
}

func (c *condition) holds(env map[string]string) bool {
	return cvTruthy(evaluate(c.root, env))
}

func evaluate(n *node, env map[string]string) cv {
	switch n.kind {
	case "BoolOp":
		if n.op == "and" {
			for _, v := range n.kids {
				if !cvTruthy(evaluate(v, env)) {
					return cv{kind: "bool", b: false}
				}
			}
			return cv{kind: "bool", b: true}
		}
		for _, v := range n.kids {
			if cvTruthy(evaluate(v, env)) {
				return cv{kind: "bool", b: true}
			}
		}
		return cv{kind: "bool", b: false}
	case "UnaryOp":
		return cv{kind: "bool", b: !cvTruthy(evaluate(n.kids[0], env))}
	case "Compare":
		left := evaluate(n.kids[0], env)
		for i, op := range n.ops {
			right := evaluate(n.kids[i+1], env)
			if !compare(op, left, right) {
				return cv{kind: "bool", b: false}
			}
			left = right
		}
		return cv{kind: "bool", b: true}
	case "Name":
		return cv{kind: "str", s: env[n.id]}
	case "Constant":
		return constantValue(n.c)
	}
	var items []cv // List or Tuple: Python builds a list of the values
	for _, e := range n.kids {
		items = append(items, constantValue(e.c))
	}
	return cv{kind: "list", list: items}
}

func constantValue(c constant) cv {
	switch c.kind {
	case "str":
		return cv{kind: "str", s: c.s}
	case "int":
		return cv{kind: "int", i: c.i}
	case "float":
		return cv{kind: "float", f: c.f}
	}
	return cv{kind: "bool", b: c.b}
}

func cvTruthy(v cv) bool {
	switch v.kind {
	case "str":
		return v.s != ""
	case "int":
		return v.i.Sign() != 0
	case "float":
		return v.f != 0
	case "bool":
		return v.b
	}
	return len(v.list) > 0
}

// number gives an exact value for comparing int, float and bool as Python does.
func number(v cv) (*big.Rat, float64, bool) {
	switch v.kind {
	case "int":
		return new(big.Rat).SetInt(v.i), 0, true
	case "bool":
		if v.b {
			return big.NewRat(1, 1), 0, true
		}
		return big.NewRat(0, 1), 0, true
	case "float":
		if math.IsInf(v.f, 0) || math.IsNaN(v.f) {
			return nil, v.f, true
		}
		r, _ := new(big.Rat).SetString(strconv.FormatFloat(v.f, 'g', -1, 64))
		return r, 0, true
	}
	return nil, 0, false
}

func pyEqual(a, b cv) bool {
	if a.kind == "str" || b.kind == "str" {
		return a.kind == b.kind && a.s == b.s
	}
	if a.kind == "list" || b.kind == "list" {
		if a.kind != b.kind || len(a.list) != len(b.list) {
			return false
		}
		for i := range a.list {
			if !pyEqual(a.list[i], b.list[i]) {
				return false
			}
		}
		return true
	}
	ra, fa, _ := number(a)
	rb, fb, _ := number(b)
	if ra == nil || rb == nil {
		return ra == nil && rb == nil && fa == fb
	}
	return ra.Cmp(rb) == 0
}

func compare(op string, a, b cv) bool {
	switch op {
	case "==":
		return pyEqual(a, b)
	case "!=":
		return !pyEqual(a, b)
	}
	var in bool
	if b.kind == "list" {
		for _, e := range b.list {
			if pyEqual(a, e) {
				in = true
				break
			}
		}
	} else {
		in = strings.Contains(b.s, a.s) // text in text: a substring, as the check requires
	}
	if op == "in" {
		return in
	}
	return !in
}
