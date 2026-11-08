package spine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/23min/verifier-kit/tools/internal/finding"
)

// ---------- output ----------

func emitText(w io.Writer, findings Findings) {
	for _, f := range findings {
		fmt.Fprintln(w, finding.Line(f.Severity, f.Code, f.Where, f.Message))
	}
	fmt.Fprintf(w, "-- %d errors, %d findings\n", findings.Errors(), len(findings))
}

// emitJSON writes the findings as JSON, indented by two spaces.
func emitJSON(w io.Writer, command string, findings Findings) {
	if findings == nil {
		findings = Findings{} // [] rather than null
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	enc.Encode(struct {
		Tool     string   `json:"tool"`
		Command  string   `json:"command"`
		Findings Findings `json:"findings"`
	}{"spine", command, findings})
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
