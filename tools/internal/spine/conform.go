package spine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ---------- conform: the adapter's requests and replies ----------

// adapterFailed is a failure of the adapter itself: no comparison is possible past it.
type adapterFailed struct{ msg string }

func (e *adapterFailed) Error() string { return e.msg }

// adapterError is an {"error": ...} reply: what the adapter says went wrong.
type adapterError struct{ msg string }

func (e *adapterError) Error() string { return e.msg }

// reply is what the adapter program gave back to a request: a line, or, when it ended before
// replying, how it ended.
type reply struct {
	line  []byte
	ended string
}

var replies = map[string][2]string{
	"new": {"ok", `{"ok": true}`}, "apply": {"result", `{"result": "..."}`}, "observe": {"state", `{"state": "..."}`},
}

const (
	newRequest     = `{"op": "new"}`
	observeRequest = `{"op": "observe"}`
)

// applyRequest is the request to apply a step.
func applyRequest(st parsedStep) string {
	pairs := make([]string, len(st.env.Names))
	for i, n := range st.env.Names {
		pairs[i] = jsonString(n) + ": " + jsonString(st.env.Values[n])
	}
	return `{"op": "apply", "event": ` + jsonString(st.event) + `, "env": {` + strings.Join(pairs, ", ") + `}}`
}

// jsonString is s as a JSON string, as encoding/json writes it, with <, > and & as they are.
func jsonString(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// parseReply reads the reply to op, a line ending at \n: a JSON object in UTF-8 with one key, the
// op's key or "error", each given once. Its value is true for new's "ok", and text otherwise;
// parseReply gives that text.
func parseReply(op string, r reply) (string, error) {
	if len(r.line) == 0 {
		return "", &adapterFailed{fmt.Sprintf("%s before replying to %s", r.ended, op)}
	}
	text := strings.TrimRight(string(r.line), "\r\n")
	if !utf8.Valid(r.line) {
		return "", &adapterFailed{fmt.Sprintf("replied to %s with a line that is not UTF-8", op)}
	}
	if !json.Valid(r.line) {
		return "", &adapterFailed{fmt.Sprintf("replied to %s with a line that is not JSON: %s", op, strconv.Quote(text))}
	}
	key, value, ok := oneKey(r.line)
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

// ---------- the search ----------

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

// run is one sequence to replay against the spine and the code.
type run struct {
	seq   []parsedStep
	extra int // steps past the shortest path it extends
}

// search is conform's search as a step function: next takes the adapter's reply to the last
// request and gives the next request, until it gives none; out then holds the findings. It never
// talks to the adapter itself.
//
// It tries every case of every live state: from the first shortest path to the state, it runs
// every sequence of 1 to margin+1 further steps, each once and shortest first, stopping at a
// terminal state, a refusal or a divergence. A case the spine decides that no run tried is
// untried-case.
type search struct {
	s        *Spec
	command  string // the adapter's command, where its failure is reported
	margin   int
	steps    []parsedStep
	paths    []statePath
	byLength map[int][]*run
	queued   map[string]bool
	longest  int

	n, started int     // runs of n steps are replaying; started of them have begun
	run        *run    // the run replaying
	at         int     // the step of the run being compared
	state      string  // the spine's state before that step
	want       Outcome // what the spine does at that step; of kind "undefined" when it doesn't say
	result     string  // what the code gave for the step: its result, or how it crashed
	waiting    string  // the reply next answers: "new", "start", "apply" or "observe"; none at the end

	exercised, reported map[string]bool
	runs                int
	wrongStart          string
	failed              bool // the adapter failed, so the shell kills it rather than closing it
	out                 Findings
}

// newSearch queues the first runs: each step from the first shortest path to each live state.
// When there is none, because the initial state is terminal or there are no events, it queues
// the empty sequence, which still checks that the adapter answers and that the code starts in
// initial (#93). command names the adapter in its findings.
func newSearch(s *Spec, command string, margin int) *search {
	c := &search{s: s, command: command, margin: margin,
		byLength: map[int][]*run{}, queued: map[string]bool{}, exercised: map[string]bool{}, reported: map[string]bool{}}
	c.steps = stepsOf(s.Events, s.assignments())
	c.paths = s.shortestPaths(c.steps)
	for _, p := range c.paths {
		for _, st := range c.steps {
			c.queue(append(p.steps[:len(p.steps):len(p.steps)], st), 1)
		}
		c.longest = max(c.longest, len(p.steps)+1)
	}
	if len(c.queued) == 0 {
		c.queue(nil, 0)
	}
	return c
}

// queue adds a run, unless the same sequence is queued: a state's start, queued first, that
// extends another state's run.
func (c *search) queue(seq []parsedStep, extra int) {
	keys := make([]string, len(seq))
	for i, st := range seq {
		keys[i] = st.key()
	}
	key := strings.Join(keys, " ")
	if c.queued[key] {
		return
	}
	c.queued[key] = true
	c.byLength[len(seq)] = append(c.byLength[len(seq)], &run{seq, extra})
}

// first is the first request; there is always one, if only the empty sequence's.
func (c *search) first() string { return c.nextRun() }

// next takes the reply to the last request and gives the next request.
func (c *search) next(r reply) string {
	switch c.waiting {
	case "new":
		if _, err := parseReply("new", r); err != nil {
			return c.fail(callAdapter("new", nil, err))
		}
		return c.await("start", observeRequest)
	case "start":
		observed, err := parseReply("observe", r)
		if err != nil {
			return c.fail(callAdapter("observe", nil, err))
		}
		if observed != c.state {
			c.diverged("start", "start", fmt.Sprintf("spine starts in %s, code starts in %s", c.state, observed))
			return c.nextRun()
		}
		return c.nextStep()
	case "apply":
		result, err := parseReply("apply", r)
		var failed *adapterFailed
		if errors.As(err, &failed) {
			return c.fail(err)
		}
		c.result = result
		if err != nil { // a crash is a divergence too
			c.result = "crashed (" + err.Error() + ")"
		}
		return c.await("observe", observeRequest)
	case "observe":
		seq := c.run.seq[:c.at+1]
		got, err := parseReply("observe", r)
		if err != nil {
			return c.fail(callAdapter("observe", seq, err))
		}
		wantResult, wantState := "ok", c.state
		if c.want.Kind == "reject" {
			wantResult = "rejected"
		}
		if c.want.Kind == "to" {
			wantState = c.want.Target
		}
		if c.result != wantResult || got != wantState {
			st := seq[c.at]
			c.diverged(stepKey(c.state, st), caseWhere(c.state, st), fmt.Sprintf("after %s: spine expects %s in %s, code gave %s in %s",
				stepsText(seq), wantResult, wantState, c.result, got))
			return c.nextRun()
		}
		c.state = wantState
		c.at++
		return c.nextStep()
	}
	panic("spine: next with no request waiting for its reply")
}

// await gives a request, and notes which reply answers it.
func (c *search) await(waiting, request string) string {
	c.waiting = waiting
	return request
}

// nextRun starts the next run, shortest first, and gives its first request. When every run is
// done, it reports each case the spine decides that no run tried, and the coverage, and gives
// none.
func (c *search) nextRun() string {
	for c.n <= c.longest {
		if c.started < len(c.byLength[c.n]) {
			c.run = c.byLength[c.n][c.started]
			c.started++
			c.runs++
			c.state, c.want, c.at = c.s.Initial, Outcome{}, 0
			return c.await("new", newRequest)
		}
		c.n, c.started = c.n+1, 0
	}
	cases := 0
	for _, p := range c.paths {
		for _, st := range c.steps {
			if len(c.s.decide(p.state, st.event, st.env)) != 1 {
				continue // a case the spine leaves undecided, which spine check reports
			}
			cases++
			if !c.exercised[stepKey(p.state, st)] {
				why := "not tried: the code diverged on the way to " + p.state
				if c.wrongStart != "" {
					why = c.wrongStart
				}
				c.out.Add("untried-case", caseWhere(p.state, st), why)
			}
		}
	}
	c.out.AddSeverity("coverage", fmt.Sprintf("%d sequences, margin %d", c.runs, c.margin),
		fmt.Sprintf("%d of %d state/event/variable cases exercised", len(c.exercised), cases), "info")
	c.waiting = ""
	return ""
}

// nextStep gives the request that applies the run's next step. The run ends after its last step,
// or at a case the spine leaves undecided, which spine check reports and nothing compares.
func (c *search) nextStep() string {
	if c.at == len(c.run.seq) {
		return c.ended()
	}
	st := c.run.seq[c.at]
	hits := c.s.decide(c.state, st.event, st.env)
	if len(hits) != 1 {
		c.want = Outcome{Kind: "undefined"}
		return c.ended()
	}
	c.want = hits[0]
	c.exercised[stepKey(c.state, st)] = true
	return c.await("apply", applyRequest(st))
}

// ended extends a run that ended with code and spine agreeing by every step, while it is within
// the margin and the code can go on: not at a terminal state, after a refusal or at an undecided
// case. It gives the next run's first request.
func (c *search) ended() string {
	if c.run.extra <= c.margin && !c.s.terminal[c.state] && c.want.Kind != "reject" && c.want.Kind != "undefined" {
		seq := c.run.seq
		for _, st := range c.steps {
			c.queue(append(seq[:len(seq):len(seq)], st), c.run.extra+1)
		}
		c.longest = max(c.longest, c.n+1)
	}
	return c.nextRun()
}

// diverged reports where code and spine first disagree, once for each case.
func (c *search) diverged(key, where, message string) {
	if !c.reported[key] {
		c.reported[key] = true
		c.out.Add("diverges", where, message)
	}
	if key == "start" {
		c.wrongStart = "not tried: the code doesn't start in " + c.s.Initial
	}
}

// fail reports the adapter's failure, past which nothing compares, and ends the search.
func (c *search) fail(err error) string {
	c.failed, c.waiting = true, ""
	c.out.Add("adapter-failed", c.command, err.Error())
	return ""
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
