// Package program starts programs and words what the system says about them as Python does:
// shared by spine, which starts an adapter's command, and verify, which starts every other
// command of a root.
package program

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

// Start is os.execvp(argv[0], argv), run in a new process with attr: a name without a slash is
// looked for in each directory of the PATH in attr.Env (this process's when attr.Env is nil, and
// /bin:/usr/bin when there is none), and the first error other than "not found" is the one
// reported. The error is a syscall.Errno when the system refused every candidate.
func Start(argv []string, attr *os.ProcAttr) (*os.Process, error) {
	candidates := []string{argv[0]}
	if !strings.Contains(argv[0], "/") {
		path, ok := lookupPath(attr.Env)
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
		proc, err := os.StartProcess(c, argv, attr)
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
	return nil, last
}

// lookupPath is PATH's value in env, or in this process's environment when env is nil.
func lookupPath(env []string) (string, bool) {
	if env == nil {
		return os.LookupEnv("PATH")
	}
	path, ok := "", false
	for _, e := range env {
		if v, found := strings.CutPrefix(e, "PATH="); found {
			path, ok = v, true // the last one wins, as in os/exec
		}
	}
	return path, ok
}

// Strerror is the C library's text for an errno, which Python's OSError.strerror carries;
// Go's own texts are the same in lower case.
func Strerror(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		s := errno.Error()
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return err.Error()
}
