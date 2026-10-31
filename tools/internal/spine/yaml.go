package spine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/23min/verifier-kit/tools/internal/program"
	yaml "go.yaml.in/yaml/v4"
)

// readYAML reads a file of the spine format: one YAML document in UTF-8. A file that can't be
// read, isn't UTF-8 or isn't YAML adds a spec-unreadable finding and gives false.
func readYAML(path string, out *Findings) (*Value, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		out.Add("spec-unreadable", path, "cannot be read: "+program.Strerror(err))
		return nil, false
	}
	if off := firstInvalidUTF8(data); off >= 0 {
		out.Add("spec-unreadable", path, fmt.Sprintf("not UTF-8: byte 0x%02x at offset %d", data[off], off))
		return nil, false
	}
	v, err := loadYAML(data)
	if err != nil {
		out.Add("spec-unreadable", path, strings.SplitN(err.Error(), "\n", 2)[0])
		return nil, false
	}
	return v, true
}

// firstInvalidUTF8 is the offset of the first byte that doesn't start a UTF-8 character, or -1.
func firstInvalidUTF8(data []byte) int {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

// loadYAML reads one YAML document: the library's YAML, with the plain words YAML 1.1 reads as
// true, false, null, numbers or dates taken as such (spine-format.md, "Text only").
func loadYAML(data []byte) (*Value, error) {
	docs, err := loadNodes(data)
	var le *yaml.LoadError
	if errors.As(err, &le) && le.Message == "found invalid Unicode character escape code" &&
		escapesSurrogate(data, le.Mark.Index) {
		return nil, fmt.Errorf("not UTF-8: the string at line %d, column %d escapes a surrogate (U+D800 to U+DFFF), "+
			"which isn't a character; write the character itself", le.ContextMark.Line, le.ContextMark.Column)
	}
	if err != nil {
		return nil, err
	}
	switch len(docs) {
	case 0:
		return &Value{Kind: KNull}, nil
	case 1:
		return value(docs[0], map[*yaml.Node]*Value{})
	}
	return nil, errors.New("expected a single document in the stream")
}

// loadNodes is every document's node tree in src.
func loadNodes(src []byte) ([]*yaml.Node, error) {
	loader, err := yaml.NewLoader(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	var docs []*yaml.Node
	for {
		n := new(yaml.Node)
		err := loader.Load(n)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, n)
	}
}

// escapesSurrogate reports whether the escape the library stopped at (index counts characters
// after a leading BOM, and points at its first hex digit) gives a surrogate, U+D800 to U+DFFF,
// rather than a code point past U+10FFFF.
func escapesSurrogate(src []byte, index int) bool {
	r := []rune(string(bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))))
	if index < 1 || index > len(r) {
		return false
	}
	digits := map[rune]int{'u': 4, 'U': 8}[r[index-1]]
	if digits == 0 || index+digits > len(r) {
		return false
	}
	v, err := strconv.ParseUint(string(r[index:index+digits]), 16, 32)
	return err == nil && v >= 0xd800 && v <= 0xdfff
}

// YAML 1.1's plain words for true and false, null, numbers, dates, and the merge and value keys.
var (
	reBool  = regexp.MustCompile(`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$`)
	reNull  = regexp.MustCompile(`^(?:~|null|Null|NULL|)$`)
	reOther = regexp.MustCompile(`^(?:` +
		`[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?|\.[0-9][0-9_]*(?:[eE][-+][0-9]+)?|` + // floats
		`[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN)|` +
		`[-+]?0b[0-1_]+|[-+]?0[0-7_]+|[-+]?(?:0|[1-9][0-9_]*)|[-+]?0x[0-9a-fA-F_]+|` + // ints
		`[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+|` +
		`[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]|` + // dates and times
		`[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?(?:[Tt]|[ \t]+)[0-9][0-9]?:[0-9][0-9]:[0-9][0-9]` +
		`(?:\.[0-9]*)?(?:[ \t]*(?:Z|[-+][0-9][0-9]?(?::[0-9][0-9])?))?|` +
		`<<|=)$`) // the merge and value keys
)

// plainTags are the explicit tags that leave a node as it would be without one.
var plainTags = map[yaml.Kind]string{yaml.ScalarNode: "!!str", yaml.SequenceNode: "!!seq", yaml.MappingNode: "!!map"}

var trueWords = map[string]bool{"yes": true, "true": true, "on": true}

// value is a node's value; built holds the nodes done, so that an alias shares its anchor's value.
func value(n *yaml.Node, built map[*yaml.Node]*Value) (*Value, error) {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return &Value{Kind: KNull}, nil
		}
		return value(n.Content[0], built)
	}
	if v, ok := built[n]; ok {
		return v, nil
	}
	v := &Value{Kind: KOther}
	built[n] = v
	if (n.Style&yaml.TaggedStyle != 0 || n.Tag == "!") && n.Tag != plainTags[n.Kind] {
		return v, nil // a value with an explicit tag the format doesn't read; the library marks ! apart
	}
	switch n.Kind {
	case yaml.ScalarNode:
		s := n.Value
		switch {
		case n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 ||
			n.Style&yaml.TaggedStyle != 0:
			v.Kind, v.Str = KStr, s
		case reBool.MatchString(s):
			v.Kind, v.Bool = KBool, trueWords[strings.ToLower(s)]
		case reNull.MatchString(s):
			v.Kind = KNull
		case !reOther.MatchString(s):
			v.Kind, v.Str = KStr, s
		}
	case yaml.SequenceNode:
		v.Kind = KList
		for _, item := range n.Content {
			x, err := value(item, built)
			if err != nil {
				return nil, err
			}
			v.Items = append(v.Items, x)
		}
	case yaml.MappingNode:
		v.Kind, v.Map = KMap, newMap()
		lines := map[string]int{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, err := value(n.Content[i], built)
			if err != nil {
				return nil, err
			}
			if first, twice := lines[k.Str]; k.Kind == KStr && twice {
				return nil, keyTwice(k.Str, first, n.Content[i].Line)
			}
			if k.Kind == KStr {
				lines[k.Str] = n.Content[i].Line
			}
			x, err := value(n.Content[i+1], built)
			if err != nil {
				return nil, err
			}
			v.Map.add(k, x)
		}
	}
	return v, nil
}

// keyTwice is the error for a text key given twice in one mapping (#55).
func keyTwice(key string, first, again int) error {
	if first == again {
		return fmt.Errorf("key %s is given twice in one mapping, on line %d", strconv.Quote(key), first)
	}
	return fmt.Errorf("key %s is given twice in one mapping, at lines %d and %d", strconv.Quote(key), first, again)
}
