package verify

// The installer, install.sh (#99), from a checkout and so offline: what it writes, that an update
// replaces the toolkit whole and leaves the project alone, that it refuses a verify or a
// .verifier-kit/ it didn't install, and that ./verify then builds from .verifier-kit/ whatever
// else the project holds. The record test installs every fixture with it too.

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// install runs install.sh in dir with args, and gives its output and exit code.
func install(t *testing.T, repo, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("bash", append([]string{filepath.Join(repo, "install.sh")}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(out), exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

// tree is every path below dir with its contents, a folder's as "/".
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			files[rel] = "/"
			return nil
		}
		data, err := os.ReadFile(p)
		files[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestInstall(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err == nil {
		repo, err = filepath.EvalSymlinks(repo) // as the installer names it
	}
	if err != nil {
		t.Fatal(err)
	}
	installed := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		out, code := install(t, repo, dir, args...)
		if code != 0 {
			t.Fatalf("exit %d: %s", code, out)
		}
		return out
	}

	t.Run("fresh", func(t *testing.T) {
		dir := t.TempDir()
		installed(t, dir, "--from", repo)
		files := tree(t, dir)
		if files[".verifier-kit/VERSION"] != "checkout "+repo+"\n" {
			t.Errorf("VERSION is %q", files[".verifier-kit/VERSION"])
		}
		if files[".verifier-kit/.gitignore"] != "bin/\n__pycache__/\n" {
			t.Errorf(".gitignore is %q", files[".verifier-kit/.gitignore"])
		}
		for _, want := range []string{"verify", ".verifier-kit/go.mod", ".verifier-kit/cmd/spine/main.go", ".verifier-kit/adapter_shim.py"} {
			if _, ok := files[want]; !ok {
				t.Errorf("no %s", want)
			}
		}
		if info, err := os.Stat(filepath.Join(dir, "verify")); err != nil || info.Mode().Perm()&0o111 == 0 {
			t.Errorf("verify is not executable: %v", err)
		}
		for p := range files {
			base := filepath.Base(p)
			if strings.HasSuffix(p, "_test.go") || base == "testdata" || base == "bin" || base == "__pycache__" || strings.HasPrefix(base, ".verifier-kit.") {
				t.Errorf("left %s, which a project doesn't need", p)
			}
		}
	})

	t.Run("update", func(t *testing.T) {
		dir, other := t.TempDir(), t.TempDir() // other: a second checkout, to update to
		writeFile(t, filepath.Join(other, "verify"), "#!/bin/sh\n", 0o755)
		writeFile(t, filepath.Join(other, "tools", "go.mod"), "module other\n", 0o644)
		installed(t, dir, "--from", repo)
		writeFile(t, filepath.Join(dir, ".verifier-kit", "stale.go"), "package stale\n", 0o644)
		writeFile(t, filepath.Join(dir, "keep.txt"), "the project's\n", 0o644)
		if out := installed(t, dir, "--from", repo); !strings.Contains(out, "is installed already") {
			t.Errorf("again: %s", out)
		}
		if out := installed(t, dir, "--from", other); !strings.Contains(out, "updated verifier-kit from checkout "+repo+" to checkout "+other) {
			t.Errorf("update: %s", out)
		}
		files := tree(t, dir)
		if files[".verifier-kit/VERSION"] != "checkout "+other+"\n" || files[".verifier-kit/go.mod"] != "module other\n" {
			t.Errorf("not updated: VERSION %q", files[".verifier-kit/VERSION"])
		}
		if _, ok := files[".verifier-kit/stale.go"]; ok {
			t.Error("a file gone from the toolkit stayed")
		}
		if files["keep.txt"] != "the project's\n" {
			t.Error("the project's own file changed")
		}
	})

	for _, tc := range []struct{ name, path, want string }{
		{"foreign folder", ".verifier-kit/theirs.txt", ".verifier-kit/ is here already"},
		{"foreign verify", "verify", "verify is here already"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, tc.path), "not the toolkit's\n", 0o644)
			before := tree(t, dir)
			out, code := install(t, repo, dir, "--from", repo)
			if code != 1 || !strings.Contains(out, tc.want) {
				t.Errorf("exit %d: %s", code, out)
			}
			if after := tree(t, dir); !reflect.DeepEqual(after, before) {
				t.Errorf("changed the project: %v", after)
			}
		})
	}

	t.Run("misuse", func(t *testing.T) {
		for _, args := range [][]string{{"--from"}, {"--from", ""}, {"nonsense"}, {"v1"}, {"v1", "v2"}} {
			if out, code := install(t, repo, t.TempDir(), args...); code != 2 {
				t.Errorf("%q: exit %d: %s", args, code, out)
			}
		}
	})

	// A Go project may have its own tools/, which would not even build, and a go.work: ./verify
	// builds from .verifier-kit/ all the same.
	t.Run("a Go project", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "tools", "go.mod"), "module theirs\n", 0o644)
		writeFile(t, filepath.Join(dir, "tools", "cmd", "spine", "main.go"), "not Go\n", 0o644)
		writeFile(t, filepath.Join(dir, "go.work"), "go 1.27\n\nuse ./app\n", 0o644)
		writeFile(t, filepath.Join(dir, "app", "go.mod"), "module app\n\ngo 1.27\n", 0o644)
		installed(t, dir, "--from", repo)
		cmd := exec.Command("bash", "./verify", "--help")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "./verify [ROOT]") {
			t.Errorf("%v: %s", err, out)
		}
	})
}
