// Package finding writes a finding as one line of text, as both tools print it
// (docs/reference/tool-contract.md, "Findings").
package finding

import "fmt"

// Line is a finding as one line: severity, code, where it applies, and why.
func Line(severity, code, where, message string) string {
	return fmt.Sprintf("%-7s %-18s %s: %s", severity, code, where, message)
}
