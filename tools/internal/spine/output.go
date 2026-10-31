package spine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ---------- output ----------

func emitText(w io.Writer, findings Findings) {
	for _, f := range findings {
		fmt.Fprintf(w, "%-7s %-18s %s: %s\n", f.Severity, f.Code, f.Where, f.Message)
	}
	fmt.Fprintf(w, "-- %d errors, %d findings\n", findings.Errors(), len(findings))
}

// emitJSON writes the findings as JSON, indented by two spaces.
func emitJSON(w io.Writer, command string, findings Findings) {
	var b strings.Builder
	b.WriteString("{\n  \"tool\": \"spine\",\n  \"command\": " + jsonText(command) + ",\n  \"findings\": ")
	if len(findings) == 0 {
		b.WriteString("[]")
	} else {
		b.WriteString("[\n")
		for i, f := range findings {
			b.WriteString("    {\n")
			fields := [][2]string{{"code", f.Code}, {"severity", f.Severity}, {"where", f.Where}, {"message", f.Message}}
			for j, kv := range fields {
				b.WriteString("      " + jsonText(kv[0]) + ": " + jsonText(kv[1]))
				if j < len(fields)-1 {
					b.WriteString(",")
				}
				b.WriteString("\n")
			}
			b.WriteString("    }")
			if i < len(findings)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("  ]")
	}
	b.WriteString("\n}\n")
	io.WriteString(w, b.String())
}

// writeJUnit writes the JUnit XML report: one test case per error, or one passing case.
func writeJUnit(path, suite string, findings Findings) error {
	var errs Findings
	for _, f := range findings {
		if f.Severity == "error" {
			errs = append(errs, f)
		}
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString(fmt.Sprintf(`<testsuite name="%s" tests="%d" failures="%d">`, xmlAttr(suite), max(1, len(errs)), len(errs)))
	if len(errs) == 0 {
		b.WriteString(fmt.Sprintf(`<testcase classname="%s" name="no errors" />`, xmlAttr(suite)))
	}
	for _, f := range errs {
		b.WriteString(fmt.Sprintf(`<testcase classname="%s" name="%s"><failure message="%s">%s</failure></testcase>`,
			xmlAttr(suite), xmlAttr(f.Code+" "+f.Where), xmlAttr(f.Message), xmlText(f.Message)))
	}
	b.WriteString("</testsuite>")
	if err := os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o666)
}

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

// jsonText is s as a JSON string in UTF-8: `"` and `\` escaped, and control characters as JSON
// writes them; a byte that isn't UTF-8 becomes U+FFFD.
func jsonText(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range strings.ToValidUTF8(s, "\uFFFD") {
		switch {
		case r == '"' || r == '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
