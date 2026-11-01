package spine

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/23min/verifier-kit/tools/internal/program"
	"github.com/23min/verifier-kit/tools/internal/shlex"
)

// ---------- conform ----------

// adapterFailed is a failure of the adapter itself: no comparison is possible past it.
type adapterFailed struct{ msg string }

func (e *adapterFailed) Error() string { return e.msg }

// adapterError is an {"error": ...} reply: what the adapter says went wrong.
type adapterError struct{ msg string }

func (e *adapterError) Error() string { return e.msg }

// commandAdapter is an adapter program, spoken to in JSON lines.
type commandAdapter struct {
	proc   *os.Process
	stdin  *os.File
	stdout *bufio.Reader
	ended  *string // how the program ended, once it has
}

var replies = map[string][2]string{
	"new": {"ok", `{"ok": true}`}, "apply": {"result", `{"result": "..."}`}, "observe": {"state", `{"state": "..."}`},
}

// startAdapter starts the program, its argv split from command.
func startAdapter(command string) (*commandAdapter, error) {
	a := &commandAdapter{}
	argv, err := shlex.Split(command)
	if err != nil {
		return nil, &adapterFailed{"cannot be split: " + err.Error()}
	}
	if len(argv) == 0 {
		return nil, &adapterFailed{"the command is empty"}
	}
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return nil, &adapterFailed{"cannot be started: " + err.Error()}
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, &adapterFailed{"cannot be started: " + err.Error()}
	}
	proc, err := startProgram(argv, []*os.File{stdinR, stdoutW, os.Stderr})
	stdinR.Close()
	stdoutW.Close()
	if err != nil {
		stdinW.Close()
		stdoutR.Close()
		return nil, &adapterFailed{"cannot be started: " + err.Error()}
	}
	a.proc, a.stdin, a.stdout = proc, stdinW, bufio.NewReader(stdoutR)
	return a, nil
}

// startProgram starts argv with files as its stdin, stdout and stderr; a failure names the
// program and gives the system's reason.
func startProgram(argv []string, files []*os.File) (*os.Process, error) {
	proc, err := program.Start(argv, &os.ProcAttr{Files: files})
	if err != nil {
		return nil, fmt.Errorf("%s: %s", argv[0], program.Strerror(err))
	}
	return proc, nil
}

func (a *commandAdapter) new() error {
	_, err := a.ask("new", `{"op": "new"}`)
	return err
}

func (a *commandAdapter) apply(event string, env Env) (string, error) {
	pairs := make([]string, len(env.Names))
	for i, n := range env.Names {
		pairs[i] = jsonString(n) + ": " + jsonString(env.Values[n])
	}
	return a.ask("apply", `{"op": "apply", "event": `+jsonString(event)+`, "env": {`+strings.Join(pairs, ", ")+`}}`)
}

func (a *commandAdapter) observe() (string, error) {
	return a.ask("observe", `{"op": "observe"}`)
}

// ask sends one request and reads one reply, a line ending at \n: a JSON object in UTF-8 with
// one key, the op's key or "error", each given once. Its value is true for new's "ok", and text
// otherwise; ask gives that text.
func (a *commandAdapter) ask(op, request string) (string, error) {
	var line []byte
	if _, err := io.WriteString(a.stdin, request+"\n"); err == nil {
		line, _ = a.stdout.ReadBytes('\n')
	}
	if len(line) == 0 {
		return "", a.exited(op)
	}
	text := strings.TrimRight(string(line), "\r\n")
	if !utf8.Valid(line) {
		return "", &adapterFailed{fmt.Sprintf("replied to %s with a line that is not UTF-8", op)}
	}
	if !json.Valid(line) {
		return "", &adapterFailed{fmt.Sprintf("replied to %s with a line that is not JSON: %s", op, strconv.Quote(text))}
	}
	key, value, ok := oneKey(line)
	var said string
	isText := ok && bytes.HasPrefix(value, []byte(`"`)) && json.Unmarshal(value, &said) == nil // null too reads as ""
	want := replies[op]
	switch {
	case key == "error" && isText:
		return "", &adapterError{said}
	case key == want[0] && op == "new" && string(value) == "true":
		return "", nil
	case key == want[0] && op != "new" && isText:
		return said, nil
	}
	return "", &adapterFailed{fmt.Sprintf(`replied to %s with %s, not %s or {"error": "..."}`, op, strconv.Quote(text), want[1])}
}

// oneKey reads a JSON object with exactly one key, given once: the key and its value.
func oneKey(line []byte) (string, json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(line))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return "", nil, false
	}
	var keys []string
	var value json.RawMessage
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return "", nil, false
		}
		keys = append(keys, t.(string))
		if err := dec.Decode(&value); err != nil {
			return "", nil, false
		}
	}
	if len(keys) != 1 {
		return "", nil, false
	}
	return keys[0], value, true
}

// exited is the failure when the program exits before replying to op.
func (a *commandAdapter) exited(op string) error {
	return &adapterFailed{fmt.Sprintf("%s before replying to %s", a.wait(), op)}
}

// wait waits for the program to end and says how it did: with an exit status, or by a signal.
func (a *commandAdapter) wait() string {
	if a.ended == nil {
		ended := "exited with status 0"
		if state, err := a.proc.Wait(); err == nil {
			ws := state.Sys().(syscall.WaitStatus)
			if ws.Signaled() {
				ended = fmt.Sprintf("was ended by signal %d (%s)", int(ws.Signal()), ws.Signal())
			} else {
				ended = fmt.Sprintf("exited with status %d", ws.ExitStatus())
			}
		}
		a.ended = &ended
	}
	return *a.ended
}

func (a *commandAdapter) close(kill bool) {
	if kill && a.ended == nil {
		a.proc.Kill()
	}
	a.stdin.Close() // the end of input ends the adapter
	a.wait()
}

// ---------- replaying sequences ----------

// divergence is where code and spine first disagree on one sequence.
type divergence struct {
	key, where, message string
}

// callAdapter makes an error reply to new or observe the adapter's failure.
func callAdapter(name string, steps []parsedStep, err error) error {
	var failed *adapterFailed
	if errors.As(err, &failed) {
		return err
	}
	after := ""
	if len(steps) > 0 {
		after = " after " + stepsText(steps)
	}
	return &adapterFailed{fmt.Sprintf("replied to %s with the error %s%s", name, strconv.Quote(err.Error()), after)}
}

// caseWhere names a case: state/event, then the variables' values when there are any.
func caseWhere(state string, st parsedStep) string {
	if len(st.env.Names) == 0 {
		return state + "/" + st.event
	}
	return fmt.Sprintf("%s/%s %s", state, st.event, st.env)
}

// stepKey identifies a state and a step.
func stepKey(state string, st parsedStep) string { return fmt.Sprintf("%q ", state) + st.key() }

// replay runs one sequence against the spine and the code: the divergence or nil, the spine's
// state afterwards, and the kind of the last outcome ("" before any).
func replay(s *Spec, a *commandAdapter, seq []parsedStep, exercised map[string]bool) (*divergence, string, string, error) {
	if err := a.new(); err != nil {
		return nil, "", "", callAdapter("new", nil, err)
	}
	state := s.Initial
	observed, err := a.observe()
	if err != nil {
		return nil, "", "", callAdapter("observe", nil, err)
	}
	if observed != state {
		return &divergence{"start", "start", fmt.Sprintf("spine starts in %s, code starts in %s", state, observed)}, state, "", nil
	}
	kind := ""
	for i, st := range seq {
		hits := s.decide(state, st.event, st.env)
		if len(hits) != 1 {
			return nil, state, "undefined", nil // spine check reports these; nothing to compare
		}
		kind = hits[0].Kind
		exercised[stepKey(state, st)] = true
		expectResult, expectState := "ok", state
		if kind == "reject" {
			expectResult = "rejected"
		}
		if kind == "to" {
			expectState = hits[0].Target
		}
		result, err := a.apply(st.event, st.env)
		var failed *adapterFailed
		if errors.As(err, &failed) {
			return nil, "", "", err
		}
		resultOK, resultText := false, ""
		if err != nil { // a crash is a divergence too
			resultText = "crashed (" + err.Error() + ")"
			resultOK = resultText == expectResult
		} else {
			resultOK, resultText = result == expectResult, result
		}
		got, err := a.observe()
		if err != nil {
			return nil, "", "", callAdapter("observe", seq[:i+1], err)
		}
		if !resultOK || got != expectState {
			return &divergence{
				stepKey(state, st),
				caseWhere(state, st),
				fmt.Sprintf("after %s: spine expects %s in %s, code gave %s in %s",
					stepsText(seq[:i+1]), expectResult, expectState, resultText, got),
			}, state, kind, nil
		}
		state = expectState
	}
	return nil, state, kind, nil
}

// conform compares code with the spine through an adapter program, started from command.
func conform(s *Spec, command string, margin int) Findings {
	var out Findings
	a, err := startAdapter(command)
	if err != nil {
		out.Add("adapter-failed", command, err.Error())
		return out
	}
	err = compareCode(s, a, margin, &out)
	if err != nil { // keep what was found; past this, nothing compares
		out.Add("adapter-failed", command, err.Error())
	}
	a.close(err != nil)
	return out
}

// compareCode tries every case of every live state: from the first shortest path to the state, it
// runs every sequence of 1 to margin+1 further steps, each once and shortest first, stopping at a
// terminal state, a refusal or a divergence. A case the spine decides that no run tried is
// untried-case.
func compareCode(s *Spec, a *commandAdapter, margin int, out *Findings) error {
	steps := stepsOf(s.Events, s.assignments())
	paths := s.shortestPaths(steps)
	type run struct {
		seq   []parsedStep
		extra int // steps past the shortest path it extends
	}
	byLength := map[int][]*run{}
	queued := map[string]bool{}
	queue := func(seq []parsedStep, extra int) {
		keys := make([]string, len(seq))
		for i, st := range seq {
			keys[i] = st.key()
		}
		key := strings.Join(keys, " ")
		if queued[key] { // a state's start, queued first, that extends another state's run
			return
		}
		queued[key] = true
		byLength[len(seq)] = append(byLength[len(seq)], &run{seq, extra})
	}
	longest := 0
	for _, p := range paths {
		for _, st := range steps {
			queue(append(p.steps[:len(p.steps):len(p.steps)], st), 1)
		}
		longest = max(longest, len(p.steps)+1)
	}
	exercised, reported, runs, wrongStart := map[string]bool{}, map[string]bool{}, 0, ""
	for n := 1; n <= longest; n++ {
		for _, r := range byLength[n] {
			runs++
			d, state, kind, err := replay(s, a, r.seq, exercised)
			if err != nil {
				return err
			}
			if d != nil {
				if !reported[d.key] {
					reported[d.key] = true
					out.Add("diverges", d.where, d.message)
				}
				if d.key == "start" {
					wrongStart = "not tried: the code doesn't start in " + s.Initial
				}
				continue
			}
			if r.extra <= margin && !s.terminal[state] && kind != "reject" && kind != "undefined" {
				for _, st := range steps {
					queue(append(r.seq[:len(r.seq):len(r.seq)], st), r.extra+1)
				}
				longest = max(longest, n+1)
			}
		}
	}
	cases := 0
	for _, p := range paths {
		for _, st := range steps {
			if len(s.decide(p.state, st.event, st.env)) != 1 {
				continue // a case the spine leaves undecided, which spine check reports
			}
			cases++
			if !exercised[stepKey(p.state, st)] {
				why := "not tried: the code diverged on the way to " + p.state
				if wrongStart != "" {
					why = wrongStart
				}
				out.Add("untried-case", caseWhere(p.state, st), why)
			}
		}
	}
	out.AddSeverity("coverage", fmt.Sprintf("%d sequences, margin %d", runs, margin),
		fmt.Sprintf("%d of %d state/event/variable cases exercised", len(exercised), cases), "info")
	return nil
}

// statePath is a live state and the first shortest sequence of steps that reaches it.
type statePath struct {
	state string
	steps []parsedStep
}

// shortestPaths is every live state with the first shortest path to it, found breadth first from
// initial in the order of steps; initial comes first, with no steps, unless it is terminal.
func (s *Spec) shortestPaths(steps []parsedStep) []statePath {
	if s.terminal[s.Initial] {
		return nil
	}
	paths := []statePath{{state: s.Initial}}
	found := map[string]bool{s.Initial: true}
	for i := 0; i < len(paths); i++ {
		p := paths[i]
		for _, st := range steps {
			next, verdict := s.step(p.state, st.event, st.env)
			if verdict != "" || found[next] || s.terminal[next] {
				continue
			}
			found[next] = true
			paths = append(paths, statePath{next, append(p.steps[:len(p.steps):len(p.steps)], st)})
		}
	}
	return paths
}
