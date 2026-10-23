// Package record keeps what a program gave on each of its test inputs, in text files a person
// can review: for each run its exit code and its outputs. A test hands it every run; the run
// must give exactly what is recorded, and every entry must still have its run. With -update the
// test writes the record again instead; review the diff before committing it, since the record
// is the only reference.
//
// Only tests use it: the spine's record (#44) and ./verify's (#68).
package record

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// Entry is one run: its exit code and its outputs, each a named section, in order.
type Entry struct {
	Exit     int
	Sections []Section
}

// Section is one output of a run, such as its standard output or a report it wrote.
type Section struct {
	Name, Text string
}

// Record is one record folder: the entries read from it, and the runs the test gave.
type Record struct {
	dir     string // the folder, holding one .txt file per group of runs
	shown   string // the folder as messages name it, such as testdata/record
	program string // what gave the runs, as messages name it, such as "the Go spine"
	rewrite string // the command that writes the record again
	update  bool
	mu      sync.Mutex
	want    map[string]map[string]Entry // file, then key
	got     map[string]map[string]Entry
}

// Load reads the record in dir, unless update, when the test is to write it.
func Load(t testing.TB, dir string, update bool, program, rewrite string) *Record {
	t.Helper()
	r := &Record{
		dir: dir, shown: filepath.Join(filepath.Base(filepath.Dir(dir)), filepath.Base(dir)),
		program: program, rewrite: rewrite, update: update,
		want: map[string]map[string]Entry{}, got: map[string]map[string]Entry{},
	}
	if update {
		return r
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no %s/*.txt (%v); run %s", r.shown, err, rewrite)
	}
	for _, p := range paths {
		entries, err := readRecord(p)
		if err != nil {
			t.Fatal(err)
		}
		r.want[strings.TrimSuffix(filepath.Base(p), ".txt")] = entries
	}
	return r
}

// Check compares one run, named name in messages, with the record's entry for key in file, or
// keeps it to write the record.
func (r *Record) Check(t testing.TB, name, file, key string, got Entry) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.got[file] == nil {
		r.got[file] = map[string]Entry{}
	}
	if _, dup := r.got[file][key]; dup {
		t.Errorf("%s: two runs have the same key in %s/%s.txt: %s", name, r.shown, file, key)
	}
	r.got[file][key] = got
	if r.update {
		return
	}
	want, ok := r.want[file][key]
	switch {
	case !ok:
		t.Errorf("%s: not in %s/%s.txt: %s\nrun %s, and review the diff", name, r.shown, file, key, r.rewrite)
	case !equal(want, got):
		t.Errorf("%s: %s\n%s differs from %s/%s.txt\nrecorded:\n%s\ngot:\n%s", name, key, r.program, r.shown, file,
			formatEntry(key, want), formatEntry(key, got))
	}
}

func equal(a, b Entry) bool {
	if a.Exit != b.Exit || len(a.Sections) != len(b.Sections) {
		return false
	}
	for i := range a.Sections {
		if a.Sections[i] != b.Sections[i] {
			return false
		}
	}
	return true
}

// Filtered tells a run of only some cases, by -run with a subtest pattern or by -skip.
func Filtered() bool {
	run, skip := flag.Lookup("test.run"), flag.Lookup("test.skip")
	return run != nil && strings.Contains(run.Value.String(), "/") || skip != nil && skip.Value.String() != ""
}

// Finish writes the record, or reports the entries no run gave.
func (r *Record) Finish(t testing.TB) {
	t.Helper()
	if r.update {
		if Filtered() {
			t.Fatal("-update writes the whole record, so it runs every case: drop the subtest pattern or -skip")
		}
		if err := os.MkdirAll(r.dir, 0o755); err != nil {
			t.Fatal(err)
		}
		old, _ := filepath.Glob(filepath.Join(r.dir, "*.txt"))
		for _, p := range old {
			if _, ok := r.got[strings.TrimSuffix(filepath.Base(p), ".txt")]; !ok {
				os.Remove(p)
			}
		}
		for file, entries := range r.got {
			if err := writeRecord(filepath.Join(r.dir, file+".txt"), entries); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	if Filtered() {
		return
	}
	for file, entries := range r.want {
		var stale []string
		for k := range entries {
			if _, ok := r.got[file][k]; !ok {
				stale = append(stale, k)
			}
		}
		sort.Strings(stale)
		if len(stale) > 0 {
			t.Errorf("%s/%s.txt has %d entries no case gives, first: %s\nrun %s, and review the diff",
				r.shown, file, len(stale), stale[0], r.rewrite)
		}
	}
}

// Word is s as one word of a key: as it is when that is unambiguous, else Go-quoted.
func Word(s string) string {
	if s != "" && Plain(s) && !strings.ContainsAny(s, " \"'\\") {
		return s
	}
	return strconv.Quote(s)
}

// ---------- the record's files ----------
//
// Each entry starts "=== " and its key, then "exit N", then a section per output, "--- " and
// the output's name. Each line of an output is "| " and the line, or "|" alone for an empty
// line; a line that isn't plain text is "|q " and the line Go-quoted in ASCII. An output ending
// in a newline ends in an empty line.

// Plain tells a line that can be written as it is: UTF-8, no control characters but tab, and
// no space or tab at the end, which editors strip.
func Plain(s string) bool {
	if !utf8.ValidString(s) || strings.HasSuffix(s, " ") || strings.HasSuffix(s, "\t") {
		return false
	}
	for _, r := range s {
		if r < 0x20 && r != '\t' || r == 0x7f || r == utf8.RuneError {
			return false
		}
	}
	return true
}

func formatOutput(b *strings.Builder, name, s string) {
	fmt.Fprintf(b, "--- %s\n", name)
	for _, line := range strings.Split(s, "\n") {
		switch {
		case line == "":
			b.WriteString("|\n")
		case Plain(line):
			b.WriteString("| " + line + "\n")
		default:
			b.WriteString("|q " + strconv.QuoteToASCII(line) + "\n")
		}
	}
}

func formatEntry(k string, e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "=== %s\nexit %d\n", k, e.Exit)
	for _, s := range e.Sections {
		formatOutput(&b, s.Name, s.Text)
	}
	return b.String()
}

func writeRecord(path string, entries map[string]Entry) error {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(formatEntry(k, entries[k]))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func readRecord(path string) (map[string]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries := map[string]Entry{}
	var k string
	var e *Entry
	var section string
	var lines []string
	flush := func() {
		if e == nil || section == "" {
			return
		}
		e.Sections = append(e.Sections, Section{section, strings.Join(lines, "\n")})
		section, lines = "", nil
	}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 0, 1<<16), 1<<26)
	for n := 1; scan.Scan(); n++ {
		line := scan.Text()
		bad := func() error { return fmt.Errorf("%s:%d: cannot read %q", path, n, line) }
		switch {
		case strings.HasPrefix(line, "=== "):
			flush()
			if e != nil {
				entries[k] = *e
			}
			k, e = line[4:], &Entry{}
			if _, dup := entries[k]; dup {
				return nil, fmt.Errorf("%s:%d: the key is there twice: %s", path, n, k)
			}
		case e == nil:
			return nil, bad()
		case strings.HasPrefix(line, "exit ") && section == "" && len(e.Sections) == 0:
			if e.Exit, err = strconv.Atoi(line[5:]); err != nil {
				return nil, bad()
			}
		case strings.HasPrefix(line, "--- "):
			flush()
			section = line[4:]
			if section == "" || strings.ContainsAny(section, " \t") {
				return nil, bad()
			}
		case section != "" && line == "|":
			lines = append(lines, "")
		case section != "" && strings.HasPrefix(line, "|q "):
			s, err := strconv.Unquote(line[3:])
			if err != nil {
				return nil, bad()
			}
			lines = append(lines, s)
		case section != "" && strings.HasPrefix(line, "| "):
			lines = append(lines, line[2:])
		default:
			return nil, bad()
		}
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	flush()
	if e != nil {
		entries[k] = *e
	}
	return entries, nil
}
