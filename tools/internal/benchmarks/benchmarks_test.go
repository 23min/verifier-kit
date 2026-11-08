package benchmarks

// The benchmarks (#106): spine conform against models of real systems, with faults planted in
// them, checked against what the theory behind it promises (docs/adr/0008-conform-margin.md).
// With observe() reading the state, conform at margin K must catch every mutant that differs from
// the spine and has at most K more states than it under names the spine has (extraStates), and
// pass every mutant that doesn't differ.
// Each model is tried twice: with its outputs dropped, and under the Moore fold, which names each
// state with the output it was entered by, so that output faults show as states.
//
// The harness runs spine only as a program and imports none of its packages, so that its
// verdicts share no code with what they judge. This test binary is also the adapter: run with
// codeEnv set, it serves the machine in that file over the adapter protocol.
//
// go test runs one mutant of each class, model and translation, chosen with hiddenSeed; -full runs
// all of them, in parallel, for a few minutes, and with -v prints the numbers docs/benchmarks.md
// reports: go -C tools test ./internal/benchmarks/ -full -v -timeout 1h.

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var full = flag.Bool("full", false, "run every mutant, not one of each class (#106)")

const codeEnv = "VERIFIER_KIT_BENCHMARKS_CODE"

var spineBin string // the binary TestMain builds

func TestMain(m *testing.M) {
	if path := os.Getenv(codeEnv); path != "" {
		os.Exit(serve(path, os.Stdin, os.Stdout))
	}
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "benchmarks-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	spineBin = filepath.Join(dir, "spine")
	build := exec.Command("go", "build", "-o", spineBin, "github.com/23min/verifier-kit/tools/cmd/spine")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building spine:", err)
		return 1
	}
	return m.Run()
}

// code is what the adapter runs: a machine, and whether observe() names the output a state was
// entered by.
type code struct {
	Machine mealy `json:"machine"`
	Moore   bool  `json:"moore"`
}

// serve answers the adapter protocol's requests on one instance of the machine in path.
func serve(path string, in io.Reader, out io.Writer) int {
	data, err := os.ReadFile(path)
	var c code
	if err == nil {
		err = json.Unmarshal(data, &c)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	state, last := c.Machine.Start, ""
	w := bufio.NewWriter(out)
	reply := func(v map[string]any) {
		line, _ := json.Marshal(v)
		w.Write(append(line, '\n'))
		w.Flush()
	}
	lines := bufio.NewScanner(in)
	for lines.Scan() {
		var req struct{ Op, Event string }
		if err := json.Unmarshal(lines.Bytes(), &req); err != nil {
			reply(map[string]any{"error": err.Error()})
			continue
		}
		switch i := slices.Index(c.Machine.Inputs, req.Event); {
		case req.Op == "new":
			state, last = c.Machine.Start, ""
			reply(map[string]any{"ok": true})
		case req.Op == "apply" && i >= 0:
			a := c.Machine.Next[state][i]
			state, last = a.To, a.Out
			reply(map[string]any{"result": "ok"})
		case req.Op == "observe":
			reply(map[string]any{"state": seen(c.Machine.Names[state], last, c.Moore)})
		default:
			reply(map[string]any{"error": "no such request: " + lines.Text()})
		}
	}
	if err := lines.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// ---------- running spine ----------

type finding struct{ Code, Severity, Where, Message string }

// spine runs the spine program and gives its exit code and findings.
func spine(env []string, args ...string) (int, []finding, error) {
	cmd := exec.Command(spineBin, append(args, "--format", "json")...)
	cmd.Env = append(os.Environ(), env...)
	stdout, err := cmd.Output()
	exit := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exit, err = exitErr.ExitCode(), nil
	}
	if err != nil {
		return 0, nil, err
	}
	var report struct{ Findings []finding }
	if err := json.Unmarshal(stdout, &report); err != nil {
		return 0, nil, fmt.Errorf("spine %s: exit %d, %v: %s", strings.Join(args, " "), exit, err, stdout)
	}
	return exit, report.Findings, nil
}

// verdict is what conform made of one machine as the code.
type verdict struct {
	margin    int // as the coverage line gives it
	killed    bool
	sequences int
	exercised string // the coverage line's message
	findings  []finding
}

// conform runs spine conform on spec with c as the code, written to a file in dir. most is the
// most sequences it may run: those of 1 to margin+1 steps after each state's path.
func conform(dir, spec string, c code, margin, most int) (verdict, error) {
	f, err := os.CreateTemp(dir, "code-*.json")
	if err != nil {
		return verdict{}, err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(c)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	self, eerr := os.Executable()
	if err == nil {
		err = eerr
	}
	exit, findings := 0, []finding(nil)
	if err == nil {
		exit, findings, err = spine([]string{codeEnv + "=" + f.Name()}, "conform", spec,
			"--adapter-cmd", "'"+strings.ReplaceAll(self, "'", `'\''`)+"'", "--margin", strconv.Itoa(margin))
	}
	if err != nil {
		return verdict{}, err
	}
	v := verdict{findings: findings}
	for _, f := range findings {
		switch f.Code {
		case "coverage":
			fmt.Sscanf(f.Where, "%d sequences, margin %d", &v.sequences, &v.margin)
			v.exercised = f.Message
		case "diverges":
			v.killed = true
		case "untried-case":
		default:
			return verdict{}, fmt.Errorf("conform %s: %s %s: %s", spec, f.Code, f.Where, f.Message)
		}
	}
	if (exit == 1) != v.killed || exit > 1 || v.sequences == 0 || v.sequences > most || v.margin != margin {
		return verdict{}, fmt.Errorf("conform %s at margin %d, at most %d sequences: exit %d with %v", spec, margin, most, exit, findings)
	}
	return v, nil
}

// ---------- the test ----------

// row is the numbers for one model, translation and class.
type row struct {
	model, translation, class string
	margin                    int
	mutants, covered, killed  int // covered: the guarantee says conform must kill it
	same, passed              int // same: no input sequence tells it from the spine
	beyond, beyondKilled      int // beyond: more extra states, under names the spine has, than the margin
	sequences                 int
	took                      time.Duration
}

func TestBenchmarks(t *testing.T) {
	type job struct {
		r     *row
		spec  string
		want  view
		k     int // the mutant's place in its class
		m     mutant
		c     code
		every string // for the model itself, the coverage every case tried gives
	}
	var rows []*row
	var jobs []job
	var sizes strings.Builder // each model's size, in each translation
	sizes.WriteString("| Model | States | States, Moore fold | Inputs | Outputs | Cells |\n|---|---|---|---|---|---|\n")
	dir := t.TempDir()
	for _, md := range models {
		m, err := readDot(filepath.Join("testdata", md.file), md.start)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&sizes, "| %s | %d | %d | %d | %d | %d |\n", md.name, len(m.Names), len(observe(m, true).names),
			len(m.Inputs), len(m.outputs()), len(m.Names)*len(m.Inputs))
		mutants := map[string][]mutant{}
		sample := map[string]int{} // the mutant of each class plain go test runs
		pick := rand.New(rand.NewPCG(hiddenSeed, uint64(len(rows))))
		for _, cl := range classes {
			mutants[cl.name] = cl.make(m)
			sample[cl.name] = pick.IntN(len(mutants[cl.name]))
		}
		for _, moore := range []bool{false, true} {
			translation := map[bool]string{false: "outputs dropped", true: "Moore fold"}[moore]
			spec := filepath.Join(dir, md.name, strconv.FormatBool(moore), "spine.yaml")
			want := observe(m, moore)
			if err := writeSpine(spec, md.name, want, m.Inputs); err != nil {
				t.Fatal(err)
			}
			exit, findings, err := spine(nil, "check", spec)
			if err != nil || exit != 0 || len(findings) > 0 {
				t.Fatalf("spine check %s, %s: exit %d, %v %v", md.name, translation, exit, err, findings)
			}
			r := &row{model: md.name, translation: translation, class: "none", margin: 1}
			rows = append(rows, r)
			cases := len(want.names) * len(m.Inputs)
			jobs = append(jobs, job{r, spec, want, 0, mutant{"the model itself", m}, code{m, moore},
				fmt.Sprintf("%d of %d state/event/variable cases exercised", cases, cases)})
			for _, cl := range classes {
				r := &row{model: md.name, translation: translation, class: cl.name, margin: cl.margin}
				rows = append(rows, r)
				for k, mu := range mutants[cl.name] {
					if *full || k == sample[cl.name] {
						jobs = append(jobs, job{r, spec, want, k, mu, code{mu.code, moore}, ""})
					}
				}
			}
		}
	}

	var mu sync.Mutex
	work := make(chan job)
	var wg sync.WaitGroup
	start := time.Now()
	for range runtime.NumCPU() {
		wg.Go(func() {
			for j := range work {
				got := observe(j.m.code, j.c.Moore)
				same := !differs(j.want, got)
				covered := !same && extraStates(j.want, got) <= j.r.margin
				most, steps := 0, 1
				for range j.r.margin + 1 {
					steps *= len(j.c.Machine.Inputs)
					most += len(j.want.names) * steps
				}
				began := time.Now()
				v, err := conform(dir, j.spec, j.c, j.r.margin, most)
				took := time.Since(began)
				if err == nil && j.every != "" && v.exercised != j.every {
					err = fmt.Errorf("not every case tried: %s", v.exercised)
				}
				if err != nil {
					t.Errorf("%s, %s, %s, mutant %d: %s: %v", j.r.model, j.r.translation, j.r.class, j.k, j.m.what, err)
					continue
				}

				mu.Lock()
				r := j.r
				r.mutants++
				r.sequences += v.sequences
				r.took += took
				switch {
				case same:
					r.same++
					if !v.killed {
						r.passed++
					}
				case covered:
					r.covered++
					if v.killed {
						r.killed++
					}
				default:
					r.beyond++
					if v.killed {
						r.beyondKilled++
					}
				}
				mu.Unlock()
				switch {
				case same && v.killed:
					t.Errorf("%s, %s, %s, mutant %d: conform reports a mutant no input sequence tells from the spine: %s: %v",
						r.model, r.translation, r.class, j.k, j.m.what, v.findings)
				case covered && !v.killed:
					t.Errorf("%s, %s, %s, mutant %d: conform passes a mutant the guarantee covers at margin %d: %s: %v",
						r.model, r.translation, r.class, j.k, r.margin, j.m.what, v.findings)
				}
			}
		})
	}
	for _, j := range jobs {
		work <- j
	}
	close(work)
	wg.Wait()

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%d conform runs in %s\n\n", sizes.String(), len(jobs), time.Since(start).Round(time.Second))
	b.WriteString("| Model | Translation | Class | Margin | Mutants | Covered, killed | Same, passed | Beyond, killed | Sequences | Time |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %d | %d, %d | %d, %d | %d, %d | %d | %s |\n", r.model, r.translation, r.class,
			r.margin, r.mutants, r.covered, r.killed, r.same, r.passed, r.beyond, r.beyondKilled, r.sequences, r.took.Round(time.Millisecond))
	}
	t.Log("\n" + b.String())
}
