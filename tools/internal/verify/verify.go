// Package verify is the verify program behind ./verify, the one entry point for every gate:
// it finds the roots and their pieces, runs spine and each root's commands, and prints a line
// per step. See docs/reference/tool-contract.md.
package verify

// The imperative shell: reading the environment and the files, starting programs and printing.
// What to do, and what it means, is decided in plan.go.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/23min/verifier-kit/tools/internal/program"
	"github.com/23min/verifier-kit/tools/internal/shlex"
)

const help = `One entry point for every gate: CI, git hooks and agent hooks all call ./verify and
nothing else, so they cannot drift apart.

  ./verify [ROOT]          fast checks (seconds): spine soundness, behaviors, stale checks,
                           conformance, content checks, and the path guard when a base is given
  ./verify [ROOT] --full   also checks the checker (mutation)

ROOT is a folder laid out like a project: spec/, checks/, conformance/, src/, mutation/.
Without ROOT: this folder when it has spec/, otherwise every examples/*/ in turn.

Nothing is skipped silently: a step that does not run prints a skip line with the reason,
and a missing piece (no spine, no adapter, no content checks, no mutants under --full) is an
error.

The adapter and the content checks are .cmd files holding one command line, in any language:
conformance/adapter_<machine>.cmd, checks/run.cmd. A command runs from its root, without a
shell, with tools/ first on its PATH, so a line can name a helper such as adapter_shim.py or
run_checks.py, and with VERIFY_SPINE set to the command line that runs spine; checks pass
when it exits 0.

The mutants are mutation/mutants.yaml, a list of edits to src/, each a name, file, find and
replace. With --full, each mutant is made in a copy of the root, and the oracle must catch it:
conform or the content checks must fail there, though they pass on the code as it is. A
mutation tool's command, mutation/mutate_<name>.cmd, runs as well, and passes when it exits 0.

Every run builds spine and verify from tools/ into tools/bin/ with Go; Go's build cache makes
that quick when nothing changed. Without Go, nothing runs.

Environment:
  VERIFY_BASE     commit to diff against; turns on the path guard (CI sets this)
  VERIFY_BRANCH   branch name the guard maps to a role (default: current git branch)
  VERIFY_REPORTS  folder for JUnit reports from the spine steps
  VERIFY_VERBOSE  1 prints every step's output, not only failures

Exit 0 when everything passes, 1 when any step fails, 2 when ./verify is misused.
`

// Main runs verify with its arguments and returns its exit code. It is tools/bin/verify, so the
// toolkit's folder is the one two above it.
func Main(args []string, stdout, stderr io.Writer) int {
	o, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %s\n", err)
		return 2
	}
	if o.help {
		fmt.Fprint(stdout, help)
		return 0
	}
	sh, err := newShell(stdout)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %s\n", err)
		return 2
	}

	var prefixes []string
	if o.root != "" {
		abs, ok := folder(o.root)
		if !ok {
			fmt.Fprintf(stderr, "verify: ROOT is not a folder: %s\n", o.root)
			return 2
		}
		prefixes = []string{prefix(abs, sh.here)}
	} else {
		prefixes = roots(isDir(filepath.Join(sh.here, "spec")), folders(filepath.Join(sh.here, "examples")))
	}
	reports := os.Getenv("VERIFY_REPORTS")
	if reports != "" && !filepath.IsAbs(reports) {
		reports = sh.here + "/" + reports
	}
	c := config{spine: filepath.Join(sh.here, "tools", "bin", "spine"), reports: reports, full: o.full}

	if base := os.Getenv("VERIFY_BASE"); base == "" {
		sh.do(step{name: "guard", skip: "VERIFY_BASE not set"}, "")
	} else {
		branch := os.Getenv("VERIFY_BRANCH")
		if branch == "" {
			branch, _, _ = sh.git(false, "rev-parse", "--abbrev-ref", "HEAD")
		}
		changed, ok, err := sh.git(true, "diff", "--name-only", base+"...HEAD")
		if err != nil || !ok {
			if err != nil {
				changed = program.Strerror(err)
			}
			fmt.Fprintf(stderr, "verify: cannot diff against %s: %s\n", base, changed)
			return 2
		}
		sh.do(guardStep(branch, changed, c), "")
	}

	for _, p := range prefixes {
		if p != "" {
			fmt.Fprintf(stdout, "== %s\n", strings.TrimSuffix(p, "/"))
		}
		for _, g := range planRoot(sh.facts(p), c) {
			for i, s := range g.steps {
				if !sh.do(s, label(p)) && i == 0 && g.unsound != "" {
					fmt.Fprintf(stdout, "skip  %s\n", g.unsound)
					break
				}
			}
		}
	}
	line, code := summary(sh.failures)
	fmt.Fprintln(stdout, line)
	return code
}

// shell runs the steps and prints them.
type shell struct {
	here     string // the toolkit's folder, where ./verify is
	stdout   io.Writer
	verbose  bool
	rootEnv  []string // the environment of a root's commands
	failures []string // the failed steps, as the summary names them
}

func newShell(stdout io.Writer) (*shell, error) {
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot find the toolkit's folder: %v", err)
	}
	here := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	spine := filepath.Join(here, "tools", "bin", "spine")
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "PATH=") && !strings.HasPrefix(e, "VERIFY_SPINE=") {
			env = append(env, e)
		}
	}
	env = append(env, "PATH="+filepath.Join(here, "tools")+":"+os.Getenv("PATH"), "VERIFY_SPINE="+shlex.Quote(spine))
	return &shell{here: here, stdout: stdout, verbose: os.Getenv("VERIFY_VERBOSE") == "1", rootEnv: env}, nil
}

// do runs one step, prints it, and says whether it passed.
func (sh *shell) do(s step, label string) bool {
	var o outcome
	switch {
	case s.cmd != nil:
		o = sh.run(s.cmd)
	case s.mutate != nil:
		o = sh.mutate(s.mutate)
	}
	for _, l := range verdict(s, o, sh.verbose) {
		fmt.Fprintln(sh.stdout, l)
	}
	ok := passed(s, o)
	if !ok {
		sh.failures = append(sh.failures, label+s.name)
	}
	return ok
}

// run starts a command and waits for it, with its standard output and standard error together.
func (sh *shell) run(c *command) outcome {
	dir := sh.here
	if c.dir != "" {
		dir = sh.path(c.dir)
	}
	return sh.runFrom(c, dir)
}

// runFrom runs a command from dir, wherever its plan says it runs from.
func (sh *shell) runFrom(c *command, dir string) outcome {
	var env []string
	if c.inRoot {
		env = sh.rootEnv
	}
	out, ok, err := sh.start(c.argv, dir, env, c.stdin, true)
	if err != nil {
		return outcome{startErr: program.Strerror(err)}
	}
	return outcome{ok: ok, out: out}
}

// start runs argv from dir and gives its standard output, with its standard error too when
// both is set, and whether it exited 0. Without stdin it reads verify's own.
func (sh *shell) start(argv []string, dir string, env []string, stdin *string, both bool) (string, bool, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return "", false, err
	}
	in := os.Stdin
	if stdin != nil {
		inR, inW, err := os.Pipe()
		if err != nil {
			r.Close()
			w.Close()
			return "", false, err
		}
		go func() {
			io.WriteString(inW, *stdin)
			inW.Close()
		}()
		defer inR.Close()
		in = inR
	}
	errOut, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		r.Close()
		w.Close()
		return "", false, err
	}
	defer errOut.Close()
	if both {
		errOut = w
	}
	proc, err := program.Start(argv, &os.ProcAttr{Dir: dir, Env: env, Files: []*os.File{in, w, errOut}})
	w.Close()
	if err != nil {
		r.Close()
		return "", false, err
	}
	data, _ := io.ReadAll(r)
	r.Close()
	state, err := proc.Wait()
	if err != nil {
		return "", false, err
	}
	return string(data), state.Success(), nil
}

// git runs git from the toolkit's folder and gives its output without the line endings after
// it, with its standard error too when both is set.
func (sh *shell) git(both bool, args ...string) (string, bool, error) {
	out, ok, err := sh.start(append([]string{"git"}, args...), sh.here, nil, nil, both)
	return strings.TrimRight(out, "\n"), ok, err
}

// path is a prefix's folder, or a path under it, as an absolute path.
func (sh *shell) path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(sh.here, p)
}

// mutate runs the oracle on the code as it is, then on each mutant, each time in a copy of the
// root, and gives the lines under the step: oracle-fails alone when the code as it is fails,
// otherwise a line per mutant, then the tally.
func (sh *shell) mutate(m *mutation) outcome {
	root := sh.path(m.prefix)
	asIs, err := sh.inCopy(root, "", "", m.oracle)
	if err != nil {
		return outcome{out: "cannot make a copy of " + root + ": " + program.Strerror(err)}
	}
	if line := oracleFails(m.shown, asIs); line != "" {
		return outcome{out: line + "\n" + tally(1, 0) + "\n"}
	}
	var lines []string
	errors := 0
	for _, mu := range m.mutants {
		var mutated, why string
		source, err := os.ReadFile(filepath.Join(root, mu.file))
		if err != nil {
			why = mu.file + " cannot be read: " + program.Strerror(err)
		} else {
			mutated, why = apply(string(source), mu)
		}
		if why != "" {
			lines = append(lines, staleLine(mu.name, why))
			errors++
			continue
		}
		runs, err := sh.inCopy(root, mu.file, mutated, m.oracle)
		if err != nil {
			return outcome{out: "cannot make a copy of " + root + ": " + program.Strerror(err)}
		}
		line, isError := verdictOf(mu.name, runs)
		lines = append(lines, line)
		if isError {
			errors++
		}
	}
	lines = append(lines, tally(errors, len(m.mutants)))
	return outcome{ok: errors == 0, out: strings.Join(lines, "\n") + "\n"}
}

// inCopy runs the oracle's steps in a new copy of the root's folders that the steps read, with
// file, when given, holding text; the copy is removed after. Output names the root, not the copy.
func (sh *shell) inCopy(root, file, text string, oracle []step) ([]ran, error) {
	dir, err := os.MkdirTemp("", "verify-mutant-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	for _, f := range copied {
		if exists(filepath.Join(root, f)) {
			if err := copyTree(filepath.Join(root, f), filepath.Join(dir, f)); err != nil {
				return nil, err
			}
		}
	}
	if file != "" {
		info, err := os.Stat(filepath.Join(root, file))
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(text), info.Mode().Perm()); err != nil {
			return nil, err
		}
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	var runs []ran
	for _, s := range oracle {
		r := ran{name: s.name, failed: true}
		switch {
		case s.lines != nil:
			r.out = strings.Join(s.lines, "\n")
		default:
			o := sh.runFrom(s.cmd, dir)
			r.failed, r.out = !o.ok, o.out
			if o.startErr != "" {
				r.failed, r.out = true, startFailure(s.cmd, o.startErr)
			}
		}
		r.out = strings.ReplaceAll(strings.ReplaceAll(r.out, real, root), dir, root)
		runs = append(runs, r)
	}
	return runs, nil
}

// copyTree copies the folder from to the new folder to: folders, files with their modes, and
// links as links.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		target := filepath.Join(to, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
		return nil
	})
}

// facts reads what the root at prefix p holds.
func (sh *shell) facts(p string) rootFacts {
	r := rootFacts{prefix: p}
	root := sh.path(p)
	for _, m := range folders(filepath.Join(root, "spec")) {
		if !exists(filepath.Join(root, "spec", m, "spine.yaml")) {
			continue
		}
		mf := machineFacts{
			name:      m,
			behaviors: isDir(filepath.Join(root, "spec", m, "behaviors")),
			checks:    isDir(filepath.Join(root, "checks", m)),
		}
		if f := filepath.Join(root, "conformance", "adapter_"+m+".cmd"); isFile(f) {
			mf.adapter = readPiece(f, p+"conformance/adapter_"+m+".cmd")
		}
		r.machines = append(r.machines, mf)
	}
	if f := filepath.Join(root, "checks", "run.cmd"); isFile(f) {
		r.checks = readPiece(f, p+"checks/run.cmd")
	}
	if f := filepath.Join(root, "mutation", "mutants.yaml"); exists(f) {
		r.mutants = readPiece(f, p+"mutation/mutants.yaml")
	}
	entries, _ := os.ReadDir(filepath.Join(root, "mutation"))
	for _, e := range entries {
		name := e.Name()
		f := filepath.Join(root, "mutation", name)
		if strings.HasPrefix(name, "mutate_") && strings.HasSuffix(name, ".cmd") && len(name) >= len("mutate_.cmd") && exists(f) {
			r.mutations = append(r.mutations, *readPiece(f, p+"mutation/"+name))
		}
	}
	return r
}

func readPiece(path, shown string) *pieceFile {
	data, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("%s", program.Strerror(err))
	}
	return &pieceFile{shown: shown, data: data, err: err}
}

// folder is the root given, from the working directory, as an absolute path with no links in
// it, and whether it is a folder.
func folder(root string) (string, bool) {
	if !filepath.IsAbs(root) {
		wd, err := os.Getwd()
		if err != nil {
			return "", false
		}
		root = filepath.Join(wd, root)
	}
	abs, err := filepath.EvalSymlinks(filepath.Clean(root))
	return abs, err == nil && isDir(abs)
}

// folders are the names of the folders in dir, as a shell's dir/*/ gives them: without hidden
// ones, links to folders included, in name order.
func folders(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") && isDir(filepath.Join(dir, e.Name())) {
			out = append(out, e.Name())
		}
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
