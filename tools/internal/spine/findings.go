// Package spine is the toolkit's spine, a port of tools/spine.py, since retired: the same
// commands, findings, messages and exit codes (docs/reference/tool-contract.md). The cases
// test in this package checks it against a record of its output, written while the two agreed.
package spine

// Finding is one result line: a code from the contract, its severity, where it applies and why.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Where    string `json:"where"`
	Message  string `json:"message"`
}

// Findings keeps the order findings were added in, as the output does.
type Findings []Finding

// Add appends an error finding.
func (f *Findings) Add(code, where, message string) {
	*f = append(*f, Finding{code, "error", where, message})
}

// AddSeverity appends a finding with the given severity.
func (f *Findings) AddSeverity(code, where, message, severity string) {
	*f = append(*f, Finding{code, severity, where, message})
}

// Errors are the findings at error severity, the only ones that change the exit code.
func (f Findings) Errors() int {
	n := 0
	for _, x := range f {
		if x.Severity == "error" {
			n++
		}
	}
	return n
}
