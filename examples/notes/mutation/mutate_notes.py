#!/usr/bin/env python3
"""Checks the checker: break the service on purpose and confirm the oracle notices.

Each mutant is one small edit to src/notes_app.py, placed ahead of the real module on the
import path. A mutant is killed when conformance or the content checks fail on it. A
surviving mutant marks behavior nothing in the oracle describes. Conformance and the content
checks must first pass on the service as it is, or every mutant would look caught.

Real projects can use a mutation tool for their language (mutmut, Stryker, PIT); this
file shows the principle with hand-picked breaks.
"""
import os
import shlex
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
# The toolkit's tools/: in ROOT for a project, further up for an example under examples/.
TOOLS = next((d / "tools" for d in (ROOT, *ROOT.parents)
              if (d / "tools" / "run_checks.py").is_file()), None)
# The command line that runs the toolkit's spine, which ./verify sets.
SPINE = shlex.split(os.environ.get("VERIFY_SPINE", ""))

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
    "conformance": [*SPINE, "conform", "spec/notes/spine.yaml",
                    "--adapter-cmd", shlex.join([sys.executable, str(TOOLS / "adapter_shim.py"),
                                                 "conformance/adapter_notes.py"])],
    "checks": [sys.executable, str(TOOLS / "run_checks.py"), "checks"],
} if TOOLS else {}


def failing(env):
    """The parts of the oracle that don't pass."""
    return [label for label, cmd in ORACLE.items()
            if subprocess.run(cmd, cwd=ROOT, env=env, capture_output=True).returncode != 0]


def main():
    if TOOLS is None:
        print(f"mutate_notes: no tools/run_checks.py in {ROOT} or above it", file=sys.stderr)
        return 2
    if not SPINE:
        print("mutate_notes: VERIFY_SPINE is not set; ./verify --full sets it", file=sys.stderr)
        return 2
    source = (ROOT / "src" / "notes_app.py").read_text()
    failed = failing(dict(os.environ, PYTHONPATH=os.pathsep.join(["src", "conformance"])))
    if failed:
        print("error   oracle-fails       on src/notes_app.py as it is: "
              f"{', '.join(failed)} did not pass")
        print("-- 1 errors, 0 mutants")
        return 1
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
