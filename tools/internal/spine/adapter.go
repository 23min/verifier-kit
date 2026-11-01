package spine

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"

	"github.com/23min/verifier-kit/tools/internal/program"
	"github.com/23min/verifier-kit/tools/internal/shlex"
)

// ---------- the adapter program ----------

// conform compares code with the spine through an adapter program, started from command. The
// search decides each request and judges each reply; this loop only carries them.
func conform(s *Spec, command string, margin int) Findings {
	c := newSearch(s, command, margin)
	a, err := startAdapter(command)
	if err != nil {
		c.fail(err)
		return c.out
	}
	for request := c.first(); request != ""; request = c.next(a.exchange(request)) {
	}
	a.close(c.failed)
	return c.out
}

// commandAdapter is an adapter program, spoken to in JSON lines.
type commandAdapter struct {
	proc   *os.Process
	stdin  *os.File
	stdout *bufio.Reader
	ended  *string // how the program ended, once it has
}

// startAdapter starts the program, its argv split from command.
func startAdapter(command string) (*commandAdapter, error) {
	a := &commandAdapter{}
	argv, err := shlex.Split(command)
	if err != nil {
		return nil, errors.New("cannot be split: " + err.Error())
	}
	if len(argv) == 0 {
		return nil, errors.New("the command is empty")
	}
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		return nil, errors.New("cannot be started: " + err.Error())
	}
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, errors.New("cannot be started: " + err.Error())
	}
	proc, err := startProgram(argv, []*os.File{stdinR, stdoutW, os.Stderr})
	stdinR.Close()
	stdoutW.Close()
	if err != nil {
		stdinW.Close()
		stdoutR.Close()
		return nil, errors.New("cannot be started: " + err.Error())
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

// exchange sends one request and reads one reply, a line ending at \n; when the program ends
// first, the reply says how.
func (a *commandAdapter) exchange(request string) reply {
	var line []byte
	if _, err := io.WriteString(a.stdin, request+"\n"); err == nil {
		line, _ = a.stdout.ReadBytes('\n')
	}
	if len(line) == 0 {
		return reply{ended: a.wait()}
	}
	return reply{line: line}
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
