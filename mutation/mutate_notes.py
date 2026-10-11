#!/usr/bin/env python3
"""Checks the checker: break the service on purpose and confirm the oracle notices.

Each mutant is one small edit to src/notes_app.py, placed ahead of the real module on the
import path. A mutant is killed when conformance or the content checks fail on it. A
surviving mutant marks behavior nothing in the oracle describes.

Real projects can use a mutation tool for their language (mutmut, Stryker, PIT); this
file shows the principle with hand-picked breaks.
"""
import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

MUTANTS = [
    ("editors may restore",
     'raise PermissionDenied("only the owner can restore")', "pass"),
    ("drafts may be trashed",
     'raise InvalidState("only published notes can be trashed")', "pass"),
    ("active notes may be deleted forever",
     'raise InvalidState("only notes in the trash can be deleted")', "pass"),
    ("trashed notes may be published",
     'raise InvalidState("cannot publish a trashed note")', "pass"),
    ("edits are stored in upper case",
     "note.text = text", "note.text = text.upper()"),
]

ORACLE = {
    "conformance": [sys.executable, "tools/spine.py", "conform", "spec/notes/spine.yaml",
                    "--adapter", "conformance/adapter_notes.py"],
    "checks": [sys.executable, "tools/run_checks.py", "checks"],
}


def main():
    source = (ROOT / "src" / "notes_app.py").read_text()
    survivors = 0
    for name, find, replace in MUTANTS:
        if source.count(find) != 1:
            print(f"error   mutant-stale       {name}: anchor not found once in src/notes_app.py")
            survivors += 1
            continue
        with tempfile.TemporaryDirectory() as tmp:
            Path(tmp, "notes_app.py").write_text(source.replace(find, replace))
            env = dict(os.environ, PYTHONPATH=os.pathsep.join([tmp, "src", "conformance"]))
            killed_by = [label for label, cmd in ORACLE.items()
                         if subprocess.run(cmd, cwd=ROOT, env=env, capture_output=True).returncode == 1]
        if killed_by:
            print(f"info    mutant-killed      {name}: caught by {', '.join(killed_by)}")
        else:
            survivors += 1
            print(f"error   mutant-survived    {name}: no part of the oracle noticed")
    print(f"-- {survivors} errors, {len(MUTANTS)} mutants")
    return 1 if survivors else 0


if __name__ == "__main__":
    sys.exit(main())
