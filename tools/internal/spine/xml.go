package spine

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// XML text and attributes, for the JUnit report and the drawing.

func xmlText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(xmlSafe(s))
}

func xmlAttr(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
		"\r", "&#13;", "\n", "&#10;", "\t", "&#09;").Replace(xmlSafe(s))
}

// xmlSafe is s with each character XML can't hold written as an
// escape, so the report always parses: \x1b for a control character, \ufffe for U+FFFE, and
// \xff for a byte that isn't UTF-8.
func xmlSafe(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == 0x9 || r == 0xa || r == 0xd || r >= 0x20 && r <= 0xd7ff || r >= 0xe000 && r <= 0xfffd || r >= 0x10000:
			b.WriteString(s[i : i+size])
		case r < 0x100:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
		i += size
	}
	return b.String()
}
