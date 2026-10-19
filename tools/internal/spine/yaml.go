package spine

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	yaml "go.yaml.in/yaml/v4"
)

// readYAML is read_yaml in tools/spine.py: the file's YAML as PyYAML's safe_load reads it. A file
// that can't be read, isn't UTF-8 or isn't YAML adds a spec-unreadable finding and gives false.
func readYAML(path string, out *Findings) (*Value, bool) {
	data, err := os.ReadFile(pyPath(path)) // Python reads Path(path): "x.yaml/" is x.yaml
	if err != nil {
		out.Add("spec-unreadable", path, "cannot be read: "+strerror(err))
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

// strerror is the C library's text for an errno, which Python's OSError.strerror carries;
// Go's own texts are the same in lower case.
func strerror(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		s := errno.Error()
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return err.Error()
}

// firstInvalidUTF8 is the offset Python's UnicodeDecodeError.start reports, or -1.
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

// pyYAMLPrintable is the set PyYAML's Reader accepts; anything else stops it.
func pyYAMLPrintable(r rune) bool {
	return r == 0x09 || r == 0x0a || r == 0x0d || (r >= 0x20 && r <= 0x7e) || r == 0x85 ||
		(r >= 0xa0 && r <= 0xd7ff) || (r >= 0xe000 && r <= 0xfffd) || (r >= 0x10000 && r <= 0x10ffff)
}

// loadYAML parses one YAML stream as safe_load does. The library gives the node tree; what
// each scalar means, merge keys and the errors safe_load raises are PyYAML's, done here.
func loadYAML(data []byte) (*Value, error) {
	for _, r := range string(data) {
		if !pyYAMLPrintable(r) {
			return nil, fmt.Errorf("unacceptable character #x%04x: special characters are not allowed", r)
		}
	}
	src, widened := pyYAMLDirectives(data), []int(nil)
	var docs []*yaml.Node
	for {
		var err error
		docs, err = loadNodes(src)
		var le *yaml.LoadError
		if errors.As(err, &le) && le.Message == "found unknown escape character" {
			if fixed, at, ok := unescapeSlashes(src, le.Mark.Index); ok {
				src, widened = fixed, append(widened, at...)
				continue
			}
		}
		if errors.As(err, &le) && le.Message == "found invalid Unicode character escape code" &&
			escapesSurrogate(src, le.Mark.Index) {
			_, at := widenedAt(src, widened)
			return nil, fmt.Errorf("not UTF-8: the string at line %d, column %d escapes a surrogate (U+D800 to U+DFFF), "+
				"which isn't a character; write the character itself", le.ContextMark.Line,
				fileColumn(at, le.ContextMark.Line, le.ContextMark.Column))
		}
		if err != nil {
			return nil, err
		}
		break
	}
	if len(docs) == 0 {
		return &Value{Kind: KNull}, nil
	}
	if len(docs) > 1 {
		return nil, errors.New("expected a single document in the stream")
	}
	if err := checkAnchors(docs[0], map[string]bool{}); err != nil {
		return nil, err
	}
	text, at := widenedAt(src, widened)
	if err := checkSimpleKeys(docs[0], strings.Split(string(text), "\n"), at); err != nil {
		return nil, err
	}
	c := &constructor{built: map[*yaml.Node]*Value{}, flat: map[*yaml.Node][]pair{}, flattening: map[*yaml.Node]bool{}}
	return c.value(docs[0])
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

// unescapeSlashes: PyYAML reads `\/` in a double-quoted scalar as "/", as YAML 1.2 does, and v4
// refuses it. Given where v4 stopped (a count of characters, not counting a leading BOM), it
// rewrites that `\/` and every later one in the same scalar to `\x2f`, which v4 reads as "/",
// and gives the byte offsets of the rewrites; false unless v4 stopped at a `\/`.
func unescapeSlashes(src []byte, index int) ([]byte, []int, bool) {
	if bytes.HasPrefix(src, []byte("\xef\xbb\xbf")) {
		index++
	}
	off := 0
	for ; index > 0 && off < len(src); index-- {
		_, size := utf8.DecodeRune(src[off:])
		off += size
	}
	if !bytes.HasPrefix(src[off:], []byte(`\/`)) {
		return nil, nil, false
	}
	var out []byte
	var at []int
	out = append(out, src[:off]...)
	i := off
	for ; i < len(src) && src[i] != '"'; i++ {
		if src[i] == '\\' && i+1 < len(src) {
			if src[i+1] == '/' {
				at = append(at, len(out))
				out = append(out, `\x2f`...)
			} else {
				out = append(out, src[i:i+2]...)
			}
			i++
			continue
		}
		out = append(out, src[i])
	}
	return append(out, src[i:]...), at, true
}

// widenedAt is src after a leading BOM, where v4 and PyYAML start counting columns, and where
// each `\/` at the given offsets into src now reads `\x2f`, two characters longer: by line (from
// 1), the columns (from 0, in characters) where each `\x2f` starts. Positions can then still be
// given, and keys measured, in the characters of the file.
func widenedAt(src []byte, offsets []int) ([]byte, map[int][]int) {
	text := bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	at := map[int][]int{}
	for _, off := range offsets {
		before := text[:off-(len(src)-len(text))]
		line := bytes.Count(before, []byte("\n")) + 1
		start := bytes.LastIndexByte(before, '\n') + 1
		at[line] = append(at[line], utf8.RuneCount(before[start:]))
	}
	return text, at
}

// fileColumn is a v4 column (from 1) as the file has it, before any `\/` earlier on the line
// was rewritten.
func fileColumn(at map[int][]int, line, column int) int {
	n := 0
	for _, c := range at[line] {
		if c < column-1 {
			n++
		}
	}
	return column - 2*n
}

// escapesSurrogate reports whether the escape v4 stopped at (index counts characters after a
// leading BOM, and points at its first hex digit) gives a surrogate, U+D800 to U+DFFF, rather
// than a code point past U+10FFFF.
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

var reYAMLDirective = regexp.MustCompile(`^%YAML[ \t]+0*1\.[0-9]+`)

// pyYAMLDirectives rewrites the directive lines before the first document as PyYAML reads
// them: it accepts any %YAML 1.x (warning about versions other than 1.1, which the library
// refuses) and ignores directives it doesn't know. Line numbers stay the same.
func pyYAMLDirectives(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		t := bytes.TrimLeft(line, " \t")
		switch {
		case len(t) == 0 || t[0] == '#':
			continue
		case line[0] != '%':
			return bytes.Join(lines, []byte("\n"))
		case reYAMLDirective.Match(line):
			lines[i] = append([]byte("%YAML 1.1"), line[len(reYAMLDirective.Find(line)):]...)
		case !bytes.HasPrefix(line, []byte("%YAML")) && !bytes.HasPrefix(line, []byte("%TAG")):
			lines[i] = nil
		}
	}
	return bytes.Join(lines, []byte("\n"))
}

// checkAnchors: PyYAML refuses an anchor defined twice in one document.
func checkAnchors(n *yaml.Node, seen map[string]bool) error {
	if n.Kind == yaml.AliasNode {
		return nil
	}
	if n.Anchor != "" {
		if seen[n.Anchor] {
			return fmt.Errorf("found duplicate anchor %s", pyRepr(n.Anchor))
		}
		seen[n.Anchor] = true
	}
	for _, c := range n.Content {
		if err := checkAnchors(c, seen); err != nil {
			return err
		}
	}
	return nil
}

// checkSimpleKeys: PyYAML forgets a possible implicit key once the ':' is more than 1024
// characters from where the key began, and then refuses the ':'. widened is where loadYAML
// rewrote `\/` as `\x2f`, two characters longer.
func checkSimpleKeys(n *yaml.Node, lines []string, widened map[int][]int) error {
	if n.Kind == yaml.AliasNode {
		return nil
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Line < 1 || k.Line > len(lines) {
				continue
			}
			line := []rune(lines[k.Line-1])
			start := k.Column - 1
			if start < 0 || start > len(line) || explicitKey(line, start) {
				continue
			}
			end := scalarEnd(line, start, k)
			for end < len(line) && (line[end] == ' ' || line[end] == '\t') {
				end++
			}
			length := end - start
			for _, c := range widened[k.Line] {
				if c >= start && c < end {
					length -= 2
				}
			}
			if end < len(line) && line[end] == ':' && length > 1024 {
				return errors.New("mapping values are not allowed here")
			}
		}
	}
	for _, c := range n.Content {
		if err := checkSimpleKeys(c, lines, widened); err != nil {
			return err
		}
	}
	return nil
}

// explicitKey reports whether a '?' indicator comes before the key on its line.
func explicitKey(line []rune, start int) bool {
	for i := start - 1; i >= 0; i-- {
		switch line[i] {
		case ' ', '\t':
			continue
		case '?':
			return true
		}
		return false
	}
	return false
}

// scalarEnd is where a one-line key scalar starting at start ends in its line.
func scalarEnd(line []rune, start int, k *yaml.Node) int {
	if start >= len(line) {
		return start
	}
	switch line[start] {
	case '\'':
		for i := start + 1; i < len(line); i++ {
			if line[i] == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
					continue
				}
				return i + 1
			}
		}
		return len(line)
	case '"':
		for i := start + 1; i < len(line); i++ {
			if line[i] == '\\' {
				i++
				continue
			}
			if line[i] == '"' {
				return i + 1
			}
		}
		return len(line)
	}
	return start + utf8.RuneCountInString(k.Value)
}

const (
	tagNull      = "tag:yaml.org,2002:null"
	tagBool      = "tag:yaml.org,2002:bool"
	tagInt       = "tag:yaml.org,2002:int"
	tagFloat     = "tag:yaml.org,2002:float"
	tagStr       = "tag:yaml.org,2002:str"
	tagTimestamp = "tag:yaml.org,2002:timestamp"
	tagBinary    = "tag:yaml.org,2002:binary"
	tagMerge     = "tag:yaml.org,2002:merge"
	tagValue     = "tag:yaml.org,2002:value"
	tagYAML      = "tag:yaml.org,2002:yaml"
	tagSeq       = "tag:yaml.org,2002:seq"
	tagMap       = "tag:yaml.org,2002:map"
	tagSet       = "tag:yaml.org,2002:set"
	tagOmap      = "tag:yaml.org,2002:omap"
	tagPairs     = "tag:yaml.org,2002:pairs"
)

// PyYAML's implicit resolvers (yaml/resolver.py), in the order it tries them for each first
// character. Its patterns are written for re.X; the spaces are gone here.
type resolver struct {
	tag string
	re  *regexp.Regexp
}

var (
	reBool      = regexp.MustCompile(`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$`)
	reFloat     = regexp.MustCompile(`^(?:[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?|\.[0-9][0-9_]*(?:[eE][-+][0-9]+)?|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`)
	reInt       = regexp.MustCompile(`^(?:[-+]?0b[0-1_]+|[-+]?0[0-7_]+|[-+]?(?:0|[1-9][0-9_]*)|[-+]?0x[0-9a-fA-F_]+|[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+)$`)
	reMerge     = regexp.MustCompile(`^(?:<<)$`)
	reNull      = regexp.MustCompile(`^(?:~|null|Null|NULL|)$`)
	reTimestamp = regexp.MustCompile(`^(?:[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]|[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?(?:[Tt]|[ \t]+)[0-9][0-9]?:[0-9][0-9]:[0-9][0-9](?:\.[0-9]*)?(?:[ \t]*(?:Z|[-+][0-9][0-9]?(?::[0-9][0-9])?))?)$`)
	reValue     = regexp.MustCompile(`^(?:=)$`)
	reYAML      = regexp.MustCompile(`^(?:!|&|\*)$`)

	resolvers = func() map[string][]resolver {
		m := map[string][]resolver{}
		add := func(tag string, re *regexp.Regexp, first string) {
			for _, c := range first {
				m[string(c)] = append(m[string(c)], resolver{tag, re})
			}
		}
		add(tagBool, reBool, "yYnNtTfFoO")
		add(tagFloat, reFloat, "-+0123456789.")
		add(tagInt, reInt, "-+0123456789")
		add(tagMerge, reMerge, "<")
		add(tagNull, reNull, "~nN")
		m[""] = append(m[""], resolver{tagNull, reNull})
		add(tagTimestamp, reTimestamp, "0123456789")
		add(tagValue, reValue, "=")
		add(tagYAML, reYAML, "!&*")
		return m
	}()
)

// resolveImplicit is Resolver.resolve for a plain scalar.
func resolveImplicit(s string) string {
	first := ""
	if s != "" {
		r, _ := utf8.DecodeRuneInString(s)
		first = string(r)
	}
	for _, r := range resolvers[first] {
		if r.re.MatchString(s) {
			return r.tag
		}
	}
	return tagStr
}

func normalizeTag(t string) string {
	if strings.HasPrefix(t, "!<") && strings.HasSuffix(t, ">") {
		t = t[2 : len(t)-1]
	}
	if strings.HasPrefix(t, "!!") {
		return "tag:yaml.org,2002:" + t[2:]
	}
	return t
}

func explicitTag(n *yaml.Node) (string, bool) {
	if n.Style&yaml.TaggedStyle == 0 {
		return "", false
	}
	return normalizeTag(n.Tag), true
}

func scalarTag(n *yaml.Node) string {
	if t, ok := explicitTag(n); ok {
		if t == "!" {
			return tagStr
		}
		return t
	}
	if n.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return tagStr
	}
	return resolveImplicit(n.Value)
}

func deref(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	return n
}

type pair struct{ key, value *yaml.Node }

type constructor struct {
	built      map[*yaml.Node]*Value
	flat       map[*yaml.Node][]pair
	flattening map[*yaml.Node]bool
}

func constructorError(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// value is construct_object: each node is built once, so aliases share one value.
func (c *constructor) value(n *yaml.Node) (*Value, error) {
	n = deref(n)
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return &Value{Kind: KNull}, nil
		}
		return c.value(n.Content[0])
	}
	if v, ok := c.built[n]; ok {
		return v, nil
	}
	switch n.Kind {
	case yaml.ScalarNode:
		v, err := constructScalar(scalarTag(n), n.Value)
		if err != nil {
			return nil, err
		}
		c.built[n] = v
		return v, nil
	case yaml.SequenceNode:
		return c.sequence(n)
	case yaml.MappingNode:
		return c.mapping(n)
	}
	return nil, constructorError("unknown node kind %v", n.Kind)
}

func (c *constructor) sequence(n *yaml.Node) (*Value, error) {
	tag, explicit := explicitTag(n)
	if !explicit || tag == "!" {
		tag = tagSeq
	}
	switch tag {
	case tagSeq:
		v := &Value{Kind: KList}
		c.built[n] = v
		for _, item := range n.Content {
			x, err := c.value(item)
			if err != nil {
				return nil, err
			}
			v.Items = append(v.Items, x)
		}
		return v, nil
	case tagOmap, tagPairs:
		v := &Value{Kind: KList}
		c.built[n] = v
		for _, item := range n.Content {
			sub := deref(item)
			if sub.Kind != yaml.MappingNode || len(sub.Content) != 2 {
				return nil, constructorError("expected a mapping of length 1")
			}
			k, err := c.value(sub.Content[0])
			if err != nil {
				return nil, err
			}
			x, err := c.value(sub.Content[1])
			if err != nil {
				return nil, err
			}
			v.Items = append(v.Items, &Value{Kind: KList, Items: []*Value{k, x}})
		}
		return v, nil
	case tagMap, tagSet:
		return nil, constructorError("expected a mapping node, but found sequence")
	case tagStr, tagNull, tagBool, tagInt, tagFloat, tagTimestamp, tagBinary:
		return nil, constructorError("expected a scalar node, but found sequence")
	}
	return nil, constructorError("could not determine a constructor for the tag %s", pyRepr(tag))
}

func (c *constructor) mapping(n *yaml.Node) (*Value, error) {
	tag, explicit := explicitTag(n)
	if !explicit || tag == "!" {
		tag = tagMap
	}
	switch tag {
	case tagMap, tagSet:
		kind := KMap
		if tag == tagSet {
			kind = KSet
		}
		v := &Value{Kind: kind, Map: newMap()}
		c.built[n] = v
		pairs, err := c.flatten(n)
		if err != nil {
			return nil, err
		}
		for _, p := range pairs {
			var k *Value
			if p.key == nil { // a `=` key, which flatten_mapping turns into text
				k = text("=")
			} else {
				k, err = c.value(p.key)
				if err != nil {
					return nil, err
				}
			}
			if !hashable(k) {
				return nil, constructorError("while constructing a mapping\nfound unhashable key")
			}
			x, err := c.value(p.value)
			if err != nil {
				return nil, err
			}
			v.Map.Set(k, x)
		}
		if err := c.keysOnce(n); err != nil {
			return nil, err
		}
		return v, nil
	case tagSeq, tagOmap, tagPairs:
		return nil, constructorError("expected a sequence node, but found mapping")
	case tagStr, tagNull, tagBool, tagInt, tagFloat, tagTimestamp, tagBinary:
		return nil, constructorError("expected a scalar node, but found mapping")
	}
	return nil, constructorError("could not determine a constructor for the tag %s", pyRepr(tag))
}

// keysOnce: the YAML spec allows a key once in a mapping, where PyYAML kept the last (#55).
// Only text keys count, as every key the format defines is text; a key a merge brings in may
// be given again, which is what merging is for.
func (c *constructor) keysOnce(n *yaml.Node) error {
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, key := n.Content[i], "="
		switch keyTag(deref(k)) {
		case tagMerge:
			continue
		case tagValue: // a `=` key, which flatten_mapping turns into text
		default:
			v, err := c.value(k)
			if err != nil {
				return err
			}
			if v.Kind != KStr {
				continue
			}
			key = v.Str
		}
		first, ok := seen[key]
		switch {
		case !ok:
			seen[key] = k.Line
		case first == k.Line:
			return fmt.Errorf("key %s is given twice in one mapping, on line %d", pyRepr(key), first)
		default:
			return fmt.Errorf("key %s is given twice in one mapping, at lines %d and %d", pyRepr(key),
				first, k.Line)
		}
	}
	return nil
}

// keyTag is the tag PyYAML gives a key node, for spotting `<<` and `=` keys.
func keyTag(n *yaml.Node) string {
	if n.Kind != yaml.ScalarNode {
		return ""
	}
	return scalarTag(n)
}

// flatten is SafeConstructor.flatten_mapping: merged pairs first, then the mapping's own, with
// `<<` keys removed and `=` keys made text (nil key).
func (c *constructor) flatten(n *yaml.Node) ([]pair, error) {
	if f, ok := c.flat[n]; ok {
		return f, nil
	}
	if c.flattening[n] { // merging a mapping into itself: PyYAML sees only its own pairs then
		var own []pair
		for i := 0; i+1 < len(n.Content); i += 2 {
			if keyTag(deref(n.Content[i])) != tagMerge {
				own = append(own, pair{n.Content[i], n.Content[i+1]})
			}
		}
		return own, nil
	}
	c.flattening[n] = true
	defer delete(c.flattening, n)
	var merge, own []pair
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		switch keyTag(deref(k)) {
		case tagMerge:
			vd := deref(v)
			switch vd.Kind {
			case yaml.MappingNode:
				sub, err := c.flatten(vd)
				if err != nil {
					return nil, err
				}
				merge = append(merge, sub...)
			case yaml.SequenceNode:
				var subs [][]pair
				for _, item := range vd.Content {
					sd := deref(item)
					if sd.Kind != yaml.MappingNode {
						return nil, constructorError("while constructing a mapping\nexpected a mapping for merging")
					}
					sub, err := c.flatten(sd)
					if err != nil {
						return nil, err
					}
					subs = append(subs, sub)
				}
				for j := len(subs) - 1; j >= 0; j-- {
					merge = append(merge, subs[j]...)
				}
			default:
				return nil, constructorError("while constructing a mapping\nexpected a mapping or list of mappings for merging")
			}
		case tagValue:
			own = append(own, pair{nil, v})
		default:
			own = append(own, pair{k, v})
		}
	}
	result := append(merge, own...)
	c.flat[n] = result
	return result, nil
}

var errSharedNaN = errors.New("shared nan")

var boolValues = map[string]bool{"yes": true, "no": false, "true": true, "false": false, "on": true, "off": false}

// constructScalar is the SafeConstructor method for a scalar's tag.
func constructScalar(tag, s string) (*Value, error) {
	switch tag {
	case tagNull:
		return &Value{Kind: KNull}, nil
	case tagBool:
		b, ok := boolValues[strings.ToLower(s)]
		if !ok {
			return nil, constructorError("%s", pyRepr(strings.ToLower(s)))
		}
		return &Value{Kind: KBool, Bool: b}, nil
	case tagInt:
		i, err := pyYAMLInt(s)
		if err != nil {
			return nil, err
		}
		return &Value{Kind: KInt, Int: i}, nil
	case tagFloat:
		f, err := pyYAMLFloat(s)
		if err == errSharedNaN { // PyYAML returns one nan object for every .nan: one dict key
			return &Value{Kind: KFloat, Float: f, Str: "shared nan"}, nil
		}
		if err != nil {
			return nil, err
		}
		return &Value{Kind: KFloat, Float: f}, nil
	case tagTimestamp:
		return pyYAMLTimestamp(s)
	case tagStr:
		return text(s), nil
	case tagBinary:
		return pyYAMLBinary(s)
	case tagSeq, tagOmap, tagPairs:
		return nil, constructorError("expected a sequence node, but found scalar")
	case tagMap, tagSet:
		return nil, constructorError("expected a mapping node, but found scalar")
	}
	return nil, constructorError("could not determine a constructor for the tag %s", pyRepr(tag))
}

// pyInt is Python's int(s, base) for ASCII digits: surrounding whitespace, a sign, and for
// bases 2, 8 and 16 the matching prefix, are allowed.
func pyInt(s string, base int) (*big.Int, error) {
	t := pyStrip(s)
	neg := false
	if t != "" && (t[0] == '+' || t[0] == '-') {
		neg = t[0] == '-'
		t = t[1:]
	}
	if len(t) > 2 && t[0] == '0' {
		p := strings.ToLower(t[:2])
		if (base == 2 && p == "0b") || (base == 8 && p == "0o") || (base == 16 && p == "0x") {
			t = t[2:]
		}
	}
	if t == "" || strings.ContainsAny(t, "+-_ ") {
		return nil, fmt.Errorf("invalid literal for int() with base %d: %s", base, pyRepr(s))
	}
	i, ok := new(big.Int).SetString(t, base)
	if !ok {
		return nil, fmt.Errorf("invalid literal for int() with base %d: %s", base, pyRepr(s))
	}
	if neg {
		i.Neg(i)
	}
	return i, nil
}

// pyYAMLInt is SafeConstructor.construct_yaml_int.
func pyYAMLInt(s string) (*big.Int, error) {
	v := strings.ReplaceAll(s, "_", "")
	if v == "" {
		return nil, errors.New("string index out of range")
	}
	sign := int64(1)
	if v[0] == '-' {
		sign = -1
	}
	if v[0] == '+' || v[0] == '-' {
		v = v[1:]
	}
	var i *big.Int
	var err error
	switch {
	case v == "0":
		return big.NewInt(0), nil
	case strings.HasPrefix(v, "0b"):
		i, err = pyInt(v[2:], 2)
	case strings.HasPrefix(v, "0x"):
		i, err = pyInt(v[2:], 16)
	case v == "":
		return nil, errors.New("string index out of range")
	case v[0] == '0':
		i, err = pyInt(v, 8)
	case strings.Contains(v, ":"):
		parts := strings.Split(v, ":")
		total, base := big.NewInt(0), big.NewInt(1)
		for j := len(parts) - 1; j >= 0; j-- {
			d, err := pyInt(parts[j], 10)
			if err != nil {
				return nil, err
			}
			total.Add(total, new(big.Int).Mul(d, base))
			base.Mul(base, big.NewInt(60))
		}
		i = total
	default:
		i, err = pyInt(v, 10)
	}
	if err != nil {
		return nil, err
	}
	return i.Mul(i, big.NewInt(sign)), nil
}

var rePyFloat = regexp.MustCompile(`^(?i:[+-]?(?:(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:e[+-]?[0-9]+)?|inf|infinity|nan))$`)

// pyFloat is Python's float(s) for ASCII text.
func pyFloat(s string) (float64, error) {
	t := pyStrip(s)
	if !rePyFloat.MatchString(t) {
		return 0, fmt.Errorf("could not convert string to float: %s", pyRepr(s))
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return 0, fmt.Errorf("could not convert string to float: %s", pyRepr(s))
	}
	return f, nil
}

// pyYAMLFloat is SafeConstructor.construct_yaml_float.
func pyYAMLFloat(s string) (float64, error) {
	v := strings.ToLower(strings.ReplaceAll(s, "_", ""))
	if v == "" {
		return 0, errors.New("string index out of range")
	}
	sign := 1.0
	if v[0] == '-' {
		sign = -1
	}
	if v[0] == '+' || v[0] == '-' {
		v = v[1:]
	}
	switch {
	case v == ".inf":
		return sign * math.Inf(1), nil
	case v == ".nan":
		return math.NaN(), errSharedNaN
	case strings.Contains(v, ":"):
		parts := strings.Split(v, ":")
		total, base := 0.0, 1.0
		for j := len(parts) - 1; j >= 0; j-- {
			d, err := pyFloat(parts[j])
			if err != nil {
				return 0, err
			}
			total += d * base
			base *= 60
		}
		return sign * total, nil
	}
	f, err := pyFloat(v)
	return sign * f, err
}

var reTimestampParts = regexp.MustCompile(`^(?P<year>[0-9][0-9][0-9][0-9])-(?P<month>[0-9][0-9]?)-(?P<day>[0-9][0-9]?)(?:(?:[Tt]|[ \t]+)(?P<hour>[0-9][0-9]?):(?P<minute>[0-9][0-9]):(?P<second>[0-9][0-9])(?:\.(?P<fraction>[0-9]*))?(?:[ \t]*(?P<tz>Z|(?P<tz_sign>[-+])(?P<tz_hour>[0-9][0-9]?)(?::(?P<tz_minute>[0-9][0-9]))?))?)?$`)

func daysIn(year, month int) int {
	switch month {
	case 4, 6, 9, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	}
	return 31
}

// pyYAMLTimestamp is SafeConstructor.construct_yaml_timestamp with Python's date and datetime
// range checks. The value keeps a text form only for telling equal keys apart.
func pyYAMLTimestamp(s string) (*Value, error) {
	m := reTimestampParts.FindStringSubmatch(s)
	if m == nil {
		return nil, errors.New("'NoneType' object has no attribute 'groupdict'")
	}
	g := map[string]string{}
	for i, name := range reTimestampParts.SubexpNames() {
		if name != "" {
			g[name] = m[i]
		}
	}
	atoi := func(x string) int { n, _ := strconv.Atoi(x); return n }
	year, month, day := atoi(g["year"]), atoi(g["month"]), atoi(g["day"])
	if year < 1 {
		return nil, fmt.Errorf("year %d is out of range", year)
	}
	if month < 1 || month > 12 {
		return nil, errors.New("month must be in 1..12")
	}
	if day < 1 || day > daysIn(year, month) {
		return nil, errors.New("day is out of range for month")
	}
	date := fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	if g["hour"] == "" {
		return &Value{Kind: KDate, Str: date}, nil
	}
	hour, minute, second := atoi(g["hour"]), atoi(g["minute"]), atoi(g["second"])
	if hour > 23 {
		return nil, errors.New("hour must be in 0..23")
	}
	if minute > 59 {
		return nil, errors.New("minute must be in 0..59")
	}
	if second > 59 {
		return nil, errors.New("second must be in 0..59")
	}
	fraction := g["fraction"]
	if len(fraction) > 6 {
		fraction = fraction[:6]
	}
	for len(fraction) < 6 {
		fraction += "0"
	}
	micro := atoi(fraction)
	clock := (int64(hour)*3600+int64(minute)*60+int64(second))*1_000_000 + int64(micro)
	if g["tz_sign"] == "" && g["tz"] == "" {
		return &Value{Kind: KTime, Str: fmt.Sprintf("naive %s %d", date, clock)}, nil
	}
	offset := int64(0)
	if g["tz_sign"] != "" {
		offset = int64(atoi(g["tz_hour"])*60+atoi(g["tz_minute"])) * 60
		if g["tz_sign"] == "-" {
			offset = -offset
		}
		if offset <= -86400 || offset >= 86400 {
			return nil, errors.New("offset must be a timedelta strictly between -timedelta(hours=24) and timedelta(hours=24)")
		}
	}
	days := daysFromCivil(year, month, day)
	instant := days*86_400_000_000 + clock - offset*1_000_000
	return &Value{Kind: KTime, Str: fmt.Sprintf("aware %d", instant)}, nil
}

// daysFromCivil counts days from 1970-01-01 to a proleptic Gregorian date.
func daysFromCivil(y, m, d int) int64 {
	if m <= 2 {
		y--
	}
	era := y / 400
	if y < 0 && y%400 != 0 {
		era--
	}
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return int64(era)*146097 + int64(doe) - 719468
}

// pyYAMLBinary is construct_yaml_binary: base64, ignoring characters outside the alphabet.
func pyYAMLBinary(s string) (*Value, error) {
	var b strings.Builder
	for _, r := range s {
		if r > 0x7f {
			return nil, errors.New("failed to convert base64 data into ascii characters")
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' {
			b.WriteRune(r)
		}
	}
	data, err := base64.StdEncoding.DecodeString(b.String())
	if err != nil {
		return nil, errors.New("failed to decode base64 data")
	}
	return &Value{Kind: KBytes, Str: string(data)}, nil
}
