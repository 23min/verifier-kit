package spine

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
)

// ---------- conform ----------

// adapterFailed is AdapterFailed in tools/spine.py: no comparison is possible past it.
type adapterFailed struct{ msg string }

func (e *adapterFailed) Error() string { return e.msg }

// adapterError is an {"error": ...} reply: the exception the adapter reports, already described.
type adapterError struct{ msg string }

func (e *adapterError) Error() string { return e.msg }

func stepsText(steps []parsedStep) string {
	parts := make([]string, len(steps))
	for i, st := range steps {
		parts[i] = st.event + "(" + st.env.String() + ")"
	}
	return strings.Join(parts, " -> ")
}

// commandAdapter is CommandAdapter in tools/spine.py: an adapter program spoken to in JSON
// lines.
type commandAdapter struct {
	proc   *os.Process
	stdin  *os.File
	stdout *bufio.Reader
	status *int
}

var replies = map[string][2]string{
	"new": {"ok", `{"ok": true}`}, "apply": {"result", `{"result": ...}`}, "observe": {"state", `{"state": ...}`},
}

// startAdapter starts the program, its argv split from command.
func startAdapter(command string) (*commandAdapter, error) {
	a := &commandAdapter{}
	argv, err := shlexSplit(command)
	if err != nil {
		return nil, &adapterFailed{"cannot be started: ValueError: " + err.Error()}
	}
	if len(argv) == 0 {
		return nil, &adapterFailed{"cannot be started: ValueError: the command is empty"}
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

// startProgram is subprocess.Popen(argv): a name without a slash is looked for in each
// directory of PATH, as Python does, and the first error other than "not found" is the one
// reported, as Python's OSError reports it.
func startProgram(argv []string, files []*os.File) (*os.Process, error) {
	candidates := []string{argv[0]}
	if !strings.Contains(argv[0], "/") {
		path, ok := os.LookupEnv("PATH")
		if !ok {
			path = "/bin:/usr/bin"
		}
		candidates = nil
		for _, dir := range strings.Split(path, ":") {
			switch {
			case dir == "":
				candidates = append(candidates, argv[0])
			case strings.HasSuffix(dir, "/"):
				candidates = append(candidates, dir+argv[0])
			default:
				candidates = append(candidates, dir+"/"+argv[0])
			}
		}
	}
	var saved, last syscall.Errno
	for _, c := range candidates {
		proc, err := os.StartProcess(c, argv, &os.ProcAttr{Files: files})
		if err == nil {
			return proc, nil
		}
		var errno syscall.Errno
		if !errors.As(err, &errno) {
			return nil, err
		}
		last = errno
		if errno != syscall.ENOENT && errno != syscall.ENOTDIR && saved == 0 {
			saved = errno
		}
	}
	if saved != 0 {
		last = saved
	}
	return nil, fmt.Errorf("%s: [Errno %d] %s: %s", pyOSErrorClass(last), int(last), strerror(last), pyRepr(argv[0]))
}

// pyOSErrorClass is the OSError subclass Python raises for an errno.
func pyOSErrorClass(errno syscall.Errno) string {
	switch errno {
	case syscall.ENOENT:
		return "FileNotFoundError"
	case syscall.EACCES, syscall.EPERM:
		return "PermissionError"
	case syscall.ENOTDIR:
		return "NotADirectoryError"
	case syscall.EISDIR:
		return "IsADirectoryError"
	case syscall.EAGAIN, syscall.EALREADY, syscall.EINPROGRESS:
		return "BlockingIOError"
	case syscall.EINTR:
		return "InterruptedError"
	}
	return "OSError"
}

func (a *commandAdapter) new() error {
	_, err := a.ask("new", `{"op": "new"}`)
	return err
}

func (a *commandAdapter) apply(event string, env Env) (*jsonValue, error) {
	pairs := make([]string, len(env.Names))
	for i, n := range env.Names {
		pairs[i] = pyJSONString(n) + ": " + pyJSONString(env.Values[n])
	}
	return a.ask("apply", `{"op": "apply", "event": `+pyJSONString(event)+`, "env": {`+strings.Join(pairs, ", ")+`}}`)
}

func (a *commandAdapter) observe() (*jsonValue, error) {
	return a.ask("observe", `{"op": "observe"}`)
}

func (a *commandAdapter) ask(op, request string) (*jsonValue, error) {
	var line []byte
	if _, err := io.WriteString(a.stdin, request+"\n"); err == nil {
		line = readTextLine(a.stdout)
	}
	if len(line) == 0 {
		return nil, a.exited(op)
	}
	text := pyDecode(line)
	reply, ok := pyJSONLoads(text)
	if !ok {
		return nil, &adapterFailed{fmt.Sprintf("replied to %s with a line that is not JSON: %s", op, pyRepr(pyRStrip(text)))}
	}
	key, shape := replies[op][0], replies[op][1]
	if reply.kind == jObject && len(reply.keys) == 1 && reply.keys[0] == "error" {
		return nil, &adapterError{reply.vals[0].str()}
	}
	if reply.kind != jObject || len(reply.keys) != 1 || reply.keys[0] != key ||
		(op == "new" && reply.vals[0].kind != jTrue) {
		return nil, &adapterFailed{fmt.Sprintf(`replied to %s with %s, not %s or {"error": ...}`, op, pyRStrip(text), shape)}
	}
	return reply.vals[0], nil
}

// exited is the failure when the program exits before replying to op.
func (a *commandAdapter) exited(op string) error {
	return &adapterFailed{fmt.Sprintf("exited with status %d before replying to %s", a.wait(), op)}
}

// wait is Popen.wait(): the exit status, or minus the signal that ended the program.
func (a *commandAdapter) wait() int {
	if a.status == nil {
		status := 0
		if state, err := a.proc.Wait(); err == nil {
			ws := state.Sys().(syscall.WaitStatus)
			if ws.Signaled() {
				status = -int(ws.Signal())
			} else {
				status = ws.ExitStatus()
			}
		}
		a.status = &status
	}
	return *a.status
}

func (a *commandAdapter) close(kill bool) {
	if kill && a.status == nil {
		a.proc.Kill()
	}
	a.stdin.Close() // the end of input ends the adapter
	a.wait()
}

// readTextLine is readline() on a pipe Python opened in text mode: a line ends at \n, \r\n or
// \r, and comes back ending in \n; at the end of output, the rest. A \r waits for the next byte,
// to see whether \n follows, as Python's decoder does.
func readTextLine(r *bufio.Reader) []byte {
	var line []byte
	for {
		c, err := r.ReadByte()
		if err != nil {
			return line
		}
		switch c {
		case '\n':
			return append(line, '\n')
		case '\r':
			if next, err := r.Peek(1); err == nil && next[0] == '\n' {
				r.ReadByte()
			}
			return append(line, '\n')
		}
		line = append(line, c)
	}
}

// pyDecode is bytes.decode("utf-8", "replace"): each invalid sequence, as far as it is a valid
// beginning, becomes one replacement character.
func pyDecode(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var out strings.Builder
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r != utf8.RuneError || size > 1 {
			out.Write(b[i : i+size])
			i += size
			continue
		}
		out.WriteRune(utf8.RuneError)
		i += invalidLength(b[i:])
	}
	return out.String()
}

// invalidLength is how many bytes Python's decoder replaces at the start of b, which doesn't
// begin with a valid sequence: the lead byte and the continuation bytes that could follow it.
func invalidLength(b []byte) int {
	c, need, lo, hi := b[0], 0, byte(0x80), byte(0xbf)
	switch {
	case c >= 0xc2 && c <= 0xdf:
		need = 1
	case c >= 0xe0 && c <= 0xef:
		need = 2
		if c == 0xe0 {
			lo = 0xa0
		} else if c == 0xed {
			hi = 0x9f
		}
	case c >= 0xf0 && c <= 0xf4:
		need = 3
		if c == 0xf0 {
			lo = 0x90
		} else if c == 0xf4 {
			hi = 0x8f
		}
	default:
		return 1
	}
	n := 1
	for k := 1; k <= need && k < len(b); k++ {
		x := b[k]
		if (k == 1 && (x < lo || x > hi)) || (k > 1 && (x < 0x80 || x > 0xbf)) {
			break
		}
		n++
	}
	return n
}

// pyRStrip is str.rstrip() with no arguments.
func pyRStrip(s string) string {
	return strings.TrimRightFunc(s, pyIsSpace)
}

// ---------- shlex.split ----------

// shlexSplit is shlex.split(s): POSIX rules, no comments. Its special characters are ASCII, so
// it reads bytes, and a byte that isn't UTF-8 reaches the program as it was given.
func shlexSplit(s string) ([]string, error) {
	const whitespace, quotes = " \t\r\n", `'"`
	pos := 0
	var out []string
	for {
		var token []byte
		state, escapedState, quoted, any := rune(' '), rune('a'), false, false
	read:
		for {
			var next rune = -1
			if pos < len(s) {
				next = rune(s[pos])
				pos++
			}
			switch {
			case state == ' ':
				switch {
				case next < 0:
					break read
				case strings.ContainsRune(whitespace, next):
					if any || quoted {
						break read
					}
				case next == '\\':
					escapedState, state = 'a', '\\'
				case strings.ContainsRune(quotes, next):
					state = next
				default:
					token = append(token, byte(next))
					any, state = true, 'a'
				}
			case state == '\'' || state == '"':
				quoted = true
				switch {
				case next < 0:
					return nil, errors.New("No closing quotation")
				case next == state:
					state = 'a'
				case next == '\\' && state == '"':
					escapedState, state = state, '\\'
				default:
					token = append(token, byte(next))
					any = true
				}
			case state == '\\':
				if next < 0 {
					return nil, errors.New("No escaped character")
				}
				if (escapedState == '\'' || escapedState == '"') && next != state && next != escapedState {
					token = append(token, byte(state))
				}
				token = append(token, byte(next))
				any, state = true, escapedState
			default: // 'a', inside a word
				switch {
				case next < 0:
					break read
				case strings.ContainsRune(whitespace, next):
					break read
				case strings.ContainsRune(quotes, next):
					state = next
				case next == '\\':
					escapedState, state = 'a', '\\'
				default:
					token = append(token, byte(next))
					any = true
				}
			}
		}
		if !any && !quoted {
			return out, nil
		}
		out = append(out, string(token))
	}
}

// ---------- JSON as json.loads reads it ----------

type jsonKind int

const (
	jNull jsonKind = iota
	jTrue
	jFalse
	jInt
	jFloat
	jString
	jArray
	jObject
)

// jsonValue is what json.loads gives: None, True, False, int, float, str, list or dict, whose
// keys keep their first place and take their last value.
type jsonValue struct {
	kind  jsonKind
	s     string
	i     *big.Int
	f     float64
	items []*jsonValue
	keys  []string
	vals  []*jsonValue
}

// pyJSONLoads is json.loads(s), or false where it raises ValueError.
func pyJSONLoads(s string) (*jsonValue, bool) {
	p := &jsonParser{s: s}
	p.space()
	v, ok := p.value()
	if !ok {
		return nil, false
	}
	p.space()
	return v, p.i == len(p.s)
}

type jsonParser struct {
	s string
	i int
}

func (p *jsonParser) space() {
	for p.i < len(p.s) && strings.IndexByte(" \t\n\r", p.s[p.i]) >= 0 {
		p.i++
	}
}

func (p *jsonParser) literal(word string) bool {
	if strings.HasPrefix(p.s[p.i:], word) {
		p.i += len(word)
		return true
	}
	return false
}

func (p *jsonParser) value() (*jsonValue, bool) {
	if p.i >= len(p.s) {
		return nil, false
	}
	switch c := p.s[p.i]; {
	case c == '"':
		p.i++
		s, ok := p.str()
		return &jsonValue{kind: jString, s: s}, ok
	case c == '{':
		p.i++
		return p.object()
	case c == '[':
		p.i++
		return p.array()
	case p.literal("null"):
		return &jsonValue{kind: jNull}, true
	case p.literal("true"):
		return &jsonValue{kind: jTrue}, true
	case p.literal("false"):
		return &jsonValue{kind: jFalse}, true
	case p.literal("NaN"):
		return &jsonValue{kind: jFloat, f: math.NaN()}, true
	case p.literal("Infinity"):
		return &jsonValue{kind: jFloat, f: math.Inf(1)}, true
	case p.literal("-Infinity"):
		return &jsonValue{kind: jFloat, f: math.Inf(-1)}, true
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return nil, false
}

func (p *jsonParser) digits() int {
	start := p.i
	for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
	}
	return p.i - start
}

// number is json's NUMBER_RE: an int, or a float when a fraction or exponent follows. An int of
// more than 4,300 digits is a ValueError, as int() makes it.
func (p *jsonParser) number() (*jsonValue, bool) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	if p.i < len(p.s) && p.s[p.i] == '0' {
		p.i++
	} else if p.digits() == 0 {
		return nil, false
	}
	isFloat := false
	if p.i+1 < len(p.s) && p.s[p.i] == '.' && p.s[p.i+1] >= '0' && p.s[p.i+1] <= '9' {
		p.i++
		p.digits()
		isFloat = true
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		save := p.i
		p.i++
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		if p.digits() == 0 {
			p.i = save
		} else {
			isFloat = true
		}
	}
	text := p.s[start:p.i]
	if isFloat {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return nil, false
		}
		return &jsonValue{kind: jFloat, f: f}, true
	}
	if len(strings.TrimPrefix(text, "-")) > 4300 {
		return nil, false
	}
	n, _ := new(big.Int).SetString(text, 10)
	return &jsonValue{kind: jInt, i: n}, true
}

// str reads a string after its opening quote: no raw control characters, and \u escapes with
// surrogate pairs joined. A lone surrogate, which Go can't hold, becomes U+FFFD.
func (p *jsonParser) str() (string, bool) {
	var b strings.Builder
	for p.i < len(p.s) {
		r, size := utf8.DecodeRuneInString(p.s[p.i:])
		p.i += size
		switch {
		case r == '"':
			return b.String(), true
		case r < 0x20:
			return "", false
		case r != '\\':
			b.WriteRune(r)
			continue
		}
		if p.i >= len(p.s) {
			return "", false
		}
		c := p.s[p.i]
		p.i++
		switch c {
		case '"', '\\', '/':
			b.WriteByte(c)
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			u, ok := p.hex4()
			if !ok {
				return "", false
			}
			if u >= 0xd800 && u <= 0xdbff && strings.HasPrefix(p.s[p.i:], `\u`) {
				save := p.i
				p.i += 2
				if lo, ok := p.hex4(); ok && lo >= 0xdc00 && lo <= 0xdfff {
					u = 0x10000 + (u-0xd800)<<10 + (lo - 0xdc00)
				} else {
					p.i = save
				}
			}
			b.WriteRune(rune(u)) // a lone surrogate is written as U+FFFD
		default:
			return "", false
		}
	}
	return "", false
}

func (p *jsonParser) hex4() (int, bool) {
	if p.i+4 > len(p.s) {
		return 0, false
	}
	h := p.s[p.i : p.i+4]
	for _, c := range h {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return 0, false
		}
	}
	p.i += 4
	v, _ := strconv.ParseUint(h, 16, 32)
	return int(v), true
}

func (p *jsonParser) array() (*jsonValue, bool) {
	v := &jsonValue{kind: jArray}
	p.space()
	if p.i < len(p.s) && p.s[p.i] == ']' {
		p.i++
		return v, true
	}
	for {
		p.space()
		item, ok := p.value()
		if !ok {
			return nil, false
		}
		v.items = append(v.items, item)
		p.space()
		if p.i >= len(p.s) {
			return nil, false
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case ']':
			p.i++
			return v, true
		default:
			return nil, false
		}
	}
}

func (p *jsonParser) object() (*jsonValue, bool) {
	v := &jsonValue{kind: jObject}
	p.space()
	if p.i < len(p.s) && p.s[p.i] == '}' {
		p.i++
		return v, true
	}
	place := map[string]int{}
	for {
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != '"' {
			return nil, false
		}
		p.i++
		key, ok := p.str()
		if !ok {
			return nil, false
		}
		p.space()
		if p.i >= len(p.s) || p.s[p.i] != ':' {
			return nil, false
		}
		p.i++
		p.space()
		val, ok := p.value()
		if !ok {
			return nil, false
		}
		if n, seen := place[key]; seen {
			v.vals[n] = val
		} else {
			place[key] = len(v.keys)
			v.keys = append(v.keys, key)
			v.vals = append(v.vals, val)
		}
		p.space()
		if p.i >= len(p.s) {
			return nil, false
		}
		switch p.s[p.i] {
		case ',':
			p.i++
		case '}':
			p.i++
			return v, true
		default:
			return nil, false
		}
	}
}

// str is Python's str() of the value; repr is its repr(), as inside a list or dict.
func (v *jsonValue) str() string {
	if v.kind == jString {
		return v.s
	}
	return v.repr()
}

func (v *jsonValue) repr() string {
	switch v.kind {
	case jNull:
		return "None"
	case jTrue:
		return "True"
	case jFalse:
		return "False"
	case jInt:
		return v.i.String()
	case jFloat:
		return pyFloatRepr(v.f)
	case jString:
		return pyRepr(v.s)
	case jArray:
		parts := make([]string, len(v.items))
		for i, x := range v.items {
			parts[i] = x.repr()
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	parts := make([]string, len(v.keys))
	for i, k := range v.keys {
		parts[i] = pyRepr(k) + ": " + v.vals[i].repr()
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// equals is v == s in Python for a str s: only a str can equal one.
func (v *jsonValue) equals(s string) bool {
	return v.kind == jString && v.s == s
}

// pyFloatRepr is repr() of a float: the shortest digits that read back the same, in fixed
// notation from 1e-4 up to 1e16 and with an exponent outside it.
func pyFloatRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	sign := ""
	if math.Signbit(f) {
		sign, f = "-", -f
	}
	if f == 0 {
		return sign + "0.0"
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // d.ddde+XX or d.ddde-XX
	mantissa, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	point := x + 1 // where the decimal point goes in digits
	switch {
	case point < -3 || point > 16:
		m := digits[:1]
		if len(digits) > 1 {
			m += "." + digits[1:]
		}
		es := strconv.Itoa(int(math.Abs(float64(x))))
		if len(es) < 2 {
			es = "0" + es
		}
		if x < 0 {
			return sign + m + "e-" + es
		}
		return sign + m + "e+" + es
	case point <= 0:
		return sign + "0." + strings.Repeat("0", -point) + digits
	case point >= len(digits):
		return sign + digits + strings.Repeat("0", point-len(digits)) + ".0"
	}
	return sign + digits[:point] + "." + digits[point:]
}

// ---------- replaying sequences ----------

// divergence is where code and spine first disagree on one sequence.
type divergence struct {
	key, where, message string
}

// callAdapter is call_adapter: an error reply to new or observe is the adapter's failure.
func callAdapter(name string, steps []parsedStep, err error) error {
	var failed *adapterFailed
	if errors.As(err, &failed) {
		return err
	}
	after := ""
	if len(steps) > 0 {
		after = " after " + stepsText(steps)
	}
	return &adapterFailed{fmt.Sprintf("%s() raised %s%s", name, err.Error(), after)}
}

// stepKey identifies a state, event and values, as the tuple tools/spine.py keys them by.
func stepKey(state string, st parsedStep) string {
	values := make([]string, len(st.env.Names))
	for i, n := range st.env.Names {
		values[i] = st.env.Values[n]
	}
	return fmt.Sprintf("%q %q %q", state, st.event, values)
}

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
	if !observed.equals(state) {
		return &divergence{"start", "start", fmt.Sprintf("spine starts in %s, code starts in %s", state, observed.str())}, state, "", nil
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
			resultOK, resultText = result.equals(expectResult), result.str()
		}
		got, err := a.observe()
		if err != nil {
			return nil, "", "", callAdapter("observe", seq[:i+1], err)
		}
		if !resultOK || !got.equals(expectState) {
			return &divergence{
				stepKey(state, st),
				fmt.Sprintf("%s/%s %s", state, st.event, st.env),
				fmt.Sprintf("after %s: spine expects %s in %s, code gave %s in %s",
					stepsText(seq[:i+1]), expectResult, expectState, resultText, got.str()),
			}, state, kind, nil
		}
		state = expectState
	}
	return nil, state, kind, nil
}

// conform compares code with the spine through an adapter program, started from command.
// depthText is the depth as Python prints it.
func conform(s *Spec, command string, depth int, depthText string) Findings {
	var out Findings
	a, err := startAdapter(command)
	if err != nil {
		out.Add("adapter-failed", command, err.Error())
		return out
	}
	err = compareCode(s, a, depth, depthText, &out)
	if err != nil { // keep what was found; past this, nothing compares
		out.Add("adapter-failed", command, err.Error())
	}
	a.close(err != nil)
	return out
}

// compareCode is compare() in tools/spine.py: it replays every sequence up to depth, shortest
// first, in the order of tools/spine.py's queue.
func compareCode(s *Spec, a *commandAdapter, depth int, depthText string, out *Findings) error {
	envs := s.assignments()
	var steps []parsedStep
	for _, e := range s.Events {
		for _, env := range envs {
			steps = append(steps, parsedStep{e, env})
		}
	}
	exercised, reported, runs := map[string]bool{}, map[string]bool{}, 0
	level := [][]parsedStep{nil}
	for len(level) > 0 {
		var next [][]parsedStep
		for _, seq := range level {
			runs++
			d, state, kind, err := replay(s, a, seq, exercised)
			if err != nil {
				return err
			}
			if d != nil {
				if !reported[d.key] {
					reported[d.key] = true
					out.Add("diverges", d.where, d.message)
				}
				continue
			}
			if len(seq) < depth && !s.terminal[state] && kind != "reject" && kind != "undefined" {
				for _, st := range steps {
					next = append(next, append(seq[:len(seq):len(seq)], st))
				}
			}
		}
		level = next
	}
	live := 0
	for _, st := range s.States {
		if !s.terminal[st] {
			live++
		}
	}
	out.AddSeverity("coverage", fmt.Sprintf("%d sequences up to %s steps", runs, depthText),
		fmt.Sprintf("%d of %d state/event/variable cases exercised", len(exercised), live*len(steps)), "info")
	return nil
}
