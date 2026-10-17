package spine

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"unicode"
)

// Kind is the Python type PyYAML's safe_load would give a YAML value. The spine format accepts
// only text, so most checks need just the kind; the value matters for text, for true/false, and
// for telling mapping keys apart the way a Python dict does.
type Kind int

const (
	KNull  Kind = iota // None
	KBool              // bool
	KInt               // int
	KFloat             // float
	KStr               // str
	KDate              // datetime.date
	KTime              // datetime.datetime
	KBytes             // bytes (!!binary)
	KList              // list, and the lists of pairs !!omap and !!pairs give
	KMap               // dict
	KSet               // set (!!set)
)

// Value is one YAML value as Python would hold it.
type Value struct {
	Kind  Kind
	Bool  bool
	Int   *big.Int
	Float float64
	Str   string // KStr; KBytes as raw bytes; KDate and KTime as a key for equality
	Items []*Value
	Map   *Map // KMap, and KSet (keys only)
}

// Map is a Python dict: keys stay in first-insertion order, and a key equal to an earlier one
// (as Python compares them: 1, 1.0 and true are one key) replaces that key's value.
type Map struct {
	Keys   []*Value
	Values []*Value
	index  map[string]int
}

func newMap() *Map { return &Map{index: map[string]int{}} }

// Set inserts or replaces, keeping the first key, as dict assignment does.
func (m *Map) Set(k, v *Value) {
	hk := hashKey(k)
	if i, ok := m.index[hk]; ok {
		m.Values[i] = v
		return
	}
	m.index[hk] = len(m.Keys)
	m.Keys = append(m.Keys, k)
	m.Values = append(m.Values, v)
}

// Get looks up a text key.
func (m *Map) Get(name string) (*Value, bool) {
	i, ok := m.index["S:"+name]
	if !ok {
		return nil, false
	}
	return m.Values[i], true
}

// Has reports whether the map has a text key.
func (m *Map) Has(name string) bool {
	_, ok := m.index["S:"+name]
	return ok
}

// Len is the number of keys.
func (m *Map) Len() int { return len(m.Keys) }

var nanCount int

// hashKey gives keys that are equal in Python the same string. Values that are unhashable in
// Python (lists, dicts, sets) never reach it: the loader reports them first.
func hashKey(v *Value) string {
	switch v.Kind {
	case KNull:
		return "N"
	case KBool:
		if v.Bool {
			return "I:1"
		}
		return "I:0"
	case KInt:
		return "I:" + v.Int.String()
	case KFloat:
		f := v.Float
		switch {
		case math.IsNaN(f) && v.Str == "shared nan":
			return "NaN"
		case math.IsNaN(f):
			nanCount++ // nan != nan: each one is its own key
			return fmt.Sprintf("NaN:%d", nanCount)
		case math.IsInf(f, 0):
			return "F:" + strconv.FormatFloat(f, 'g', -1, 64)
		case f == math.Trunc(f):
			i, _ := new(big.Float).SetFloat64(f).Int(nil)
			return "I:" + i.String()
		}
		return "F:" + strconv.FormatFloat(f, 'g', -1, 64)
	case KStr:
		return "S:" + v.Str
	case KDate:
		return "D:" + v.Str
	case KTime:
		return "T:" + v.Str
	case KBytes:
		return "B:" + v.Str
	}
	panic(fmt.Sprintf("unhashable kind %d", v.Kind))
}

// hashable reports whether Python could use the value as a dict key.
func hashable(v *Value) bool {
	return v.Kind != KList && v.Kind != KMap && v.Kind != KSet
}

// truthy is Python's bool(value).
func truthy(v *Value) bool {
	if v == nil {
		return false
	}
	switch v.Kind {
	case KNull:
		return false
	case KBool:
		return v.Bool
	case KInt:
		return v.Int.Sign() != 0
	case KFloat:
		return v.Float != 0 // nan is true in Python too
	case KStr, KBytes:
		return v.Str != ""
	case KDate, KTime:
		return true
	case KList:
		return len(v.Items) > 0
	case KMap, KSet:
		return v.Map.Len() > 0
	}
	return true
}

func text(s string) *Value { return &Value{Kind: KStr, Str: s} }

// isText is isinstance(v, str).
func isText(v *Value) bool { return v != nil && v.Kind == KStr }

// isBool is isinstance(v, bool).
func isBool(v *Value) bool { return v != nil && v.Kind == KBool }

// pyRepr is Python's repr() of text: the quote it picks and the escapes it uses.
func pyRepr(s string) string {
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteRune(quote)
	for _, r := range s {
		switch {
		case r == quote || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < ' ' || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r < 0x7f:
			b.WriteRune(r)
		case unicode.IsPrint(r):
			b.WriteRune(r)
		case r <= 0xff:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r <= 0xffff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			fmt.Fprintf(&b, `\U%08x`, r)
		}
	}
	b.WriteRune(quote)
	return b.String()
}

// pyBoolRepr is repr() of a bool.
func pyBoolRepr(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// pyIsSpace is str.isspace() for one character.
func pyIsSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', 0x1c, 0x1d, 0x1e, 0x1f, ' ', 0x85, 0xa0, 0x1680,
		0x2028, 0x2029, 0x202f, 0x205f, 0x3000:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

// pySplit is str.split() with no arguments: runs of whitespace separate, none at either end.
func pySplit(s string) []string {
	return strings.FieldsFunc(s, pyIsSpace)
}

// pyStrip is str.strip() with no arguments.
func pyStrip(s string) string {
	return strings.TrimFunc(s, pyIsSpace)
}

// intArgument is int(s), which argparse calls for type=int: base 10, digits from any script,
// single underscores between digits, and whitespace at either end, which is ASCII's six and any
// other character str.isspace() takes. More than 4,300 digits, leading zeros among them, is an
// error in Python too. A value past an int is clamped: no depth can tell the difference.
func intArgument(s string) (int, bool) {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case pyIsSpace(r):
			b.WriteByte(' ')
		default:
			d := pyDecimal(r)
			if d < 0 {
				return 0, false
			}
			b.WriteByte(byte('0' + d))
		}
	}
	t := strings.Trim(b.String(), " \t\n\v\f\r")
	neg := strings.HasPrefix(t, "-")
	if neg || strings.HasPrefix(t, "+") {
		t = t[1:]
	}
	digits := strings.ReplaceAll(t, "_", "")
	if digits == "" || len(digits) > 4300 || strings.Trim(digits, "0123456789") != "" ||
		t[0] == '_' || t[len(t)-1] == '_' || strings.Contains(t, "__") {
		return 0, false
	}
	n, _ := new(big.Int).SetString(digits, 10)
	if neg {
		n.Neg(n)
	}
	switch {
	case n.Cmp(big.NewInt(math.MaxInt)) > 0:
		return math.MaxInt, true
	case n.Cmp(big.NewInt(math.MinInt)) < 0:
		return math.MinInt, true
	}
	return int(n.Int64()), true
}

// pyDecimal is the value of a decimal digit (Unicode category Nd), or -1. Unicode encodes each
// script's digits as a run of ten, zero first, so a digit's value is its place in its run.
func pyDecimal(r rune) int {
	if !unicode.IsDigit(r) {
		return -1
	}
	z := r
	for unicode.IsDigit(z - 1) {
		z--
	}
	return int(r-z) % 10
}
