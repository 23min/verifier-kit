package spine

// Kind is what a YAML value is to the spine format: text, true or false, nothing, a list or a
// mapping, or something else the format never reads, such as a number, a date or a value with an
// explicit tag.
type Kind int

const (
	KNull  Kind = iota // null, ~ or nothing
	KBool              // true or false, in YAML 1.1's words (yes, no, on, off too)
	KStr               // text
	KList              // a list
	KMap               // a mapping
	KOther             // a number, a date, a merge key or a value with an explicit tag
)

// Value is one YAML value.
type Value struct {
	Kind  Kind
	Bool  bool   // KBool
	Str   string // KStr
	Items []*Value
	Map   *Map
}

// Map is a mapping: keys in the order the file gives them. A text key is given at most once.
type Map struct {
	Keys   []*Value
	Values []*Value
	index  map[string]int // text keys
}

func newMap() *Map { return &Map{index: map[string]int{}} }

// add appends a pair; the reader has refused a text key given twice.
func (m *Map) add(k, v *Value) {
	if k.Kind == KStr {
		m.index[k.Str] = len(m.Keys)
	}
	m.Keys = append(m.Keys, k)
	m.Values = append(m.Values, v)
}

// Get looks up a text key.
func (m *Map) Get(name string) (*Value, bool) {
	i, ok := m.index[name]
	if !ok {
		return nil, false
	}
	return m.Values[i], true
}

// Has reports whether the map has a text key.
func (m *Map) Has(name string) bool {
	_, ok := m.index[name]
	return ok
}

// Len is the number of keys.
func (m *Map) Len() int { return len(m.Keys) }

// isText reports whether v is text.
func isText(v *Value) bool { return v != nil && v.Kind == KStr }

// isBool reports whether v is true or false.
func isBool(v *Value) bool { return v != nil && v.Kind == KBool }

// isTrue reports whether v is true.
func isTrue(v *Value) bool { return isBool(v) && v.Bool }
