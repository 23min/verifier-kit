package verify

// The copy --full runs the oracle in (#113): the whole root, once, but for its .git and src/,
// which inCopy gives each run afresh.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyRoot(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		".git/HEAD":         "ref: refs/heads/main\n",
		"src/lamp.py":       "state = 'dark'\n",
		"mix.exs":           "defmodule Lamp.MixProject do end\n",
		"deps/oban/mix.exs": "# a dependency\n",
		"checks/run.cmd":    "sh -c \"exit 0\"\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := copyRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, name := range []string{"mix.exs", "deps/oban/mix.exs", "checks/run.cmd"} {
		if !exists(filepath.Join(dir, name)) {
			t.Errorf("the copy lacks %s", name)
		}
	}
	for _, name := range []string{".git", "src"} {
		if exists(filepath.Join(dir, name)) {
			t.Errorf("the copy holds %s, which it leaves out", name)
		}
	}
}
