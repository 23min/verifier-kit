package spine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ---------- output, as in tools/spine.py ----------

func emitText(w io.Writer, findings Findings) {
	for _, f := range findings {
		fmt.Fprintf(w, "%-7s %-18s %s: %s\n", f.Severity, f.Code, f.Where, f.Message)
	}
	fmt.Fprintf(w, "-- %d errors, %d findings\n", findings.Errors(), len(findings))
}

// emitJSON writes what json.dumps(..., indent=2) gives.
func emitJSON(w io.Writer, command string, findings Findings) {
	var b strings.Builder
	b.WriteString("{\n  \"tool\": \"spine\",\n  \"command\": " + pyJSONString(command) + ",\n  \"findings\": ")
	if len(findings) == 0 {
		b.WriteString("[]")
	} else {
		b.WriteString("[\n")
		for i, f := range findings {
			b.WriteString("    {\n")
			fields := [][2]string{{"code", f.Code}, {"severity", f.Severity}, {"where", f.Where}, {"message", f.Message}}
			for j, kv := range fields {
				b.WriteString("      " + pyJSONString(kv[0]) + ": " + pyJSONString(kv[1]))
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

// writeJUnit writes what xml.etree.ElementTree writes for the report.
func writeJUnit(path, suite string, findings Findings) error {
	var errs Findings
	for _, f := range findings {
		if f.Severity == "error" {
			errs = append(errs, f)
		}
	}
	var b strings.Builder
	b.WriteString("<?xml version='1.0' encoding='utf-8'?>\n")
	b.WriteString(fmt.Sprintf(`<testsuite name="%s" tests="%d" failures="%d">`, xmlAttr(suite), max(1, len(errs)), len(errs)))
	if len(errs) == 0 {
		b.WriteString(fmt.Sprintf(`<testcase classname="%s" name="no errors" />`, xmlAttr(suite)))
	}
	for _, f := range errs {
		b.WriteString(fmt.Sprintf(`<testcase classname="%s" name="%s"><failure message="%s">%s</failure></testcase>`,
			xmlAttr(suite), xmlAttr(f.Code+" "+f.Where), xmlAttr(f.Message), xmlText(f.Message)))
	}
	b.WriteString("</testsuite>")
	if err := os.MkdirAll(filepath.Dir(pyPath(path)), 0o777); err != nil { // Path(path).parent
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o666)
}

func xmlText(s string) string {
	return charRefs(strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s))
}

func xmlAttr(s string) string {
	return charRefs(strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;",
		"\r", "&#13;", "\n", "&#10;", "\t", "&#09;").Replace(s))
}

// charRefs writes a byte that isn't UTF-8 as Python's xmlcharrefreplace writes the surrogate
// surrogateescape gave it.
func charRefs(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&b, "&#%d;", 0xdc00+int(s[i]))
		} else {
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}
