#!/usr/bin/env python3
"""Checks the checker: break the client on purpose and confirm the oracle notices.

Each mutant is one small edit to src/ref_sync_client.py, placed ahead of the real module on
the import path. A mutant is killed when conformance or a content check fails on it; the
output names every check that failed and the first problem found. A surviving mutant marks
behavior nothing in the oracle describes.

The six are the ones the example was proposed with; the first, "remove the fast-forward
check", becomes "the client forces its push", since the remote, not the client, does that
check. Like mutate_notes.py, this shows the principle with hand-picked breaks.
"""
import os
import re
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
    ("the client forces its push",
     "self.remote.push(parent, files, force=False)", "self.remote.push(parent, files, force=True)"),
    ("event IDs are not deduplicated",
     "if event_path in files:", "if False:"),
    ("a push without an answer counts as confirmed",
     'event = "response_lost"', 'event = "accepted"'),
    ("a retry does not check the expected version again",
     'if entity["version"] != intent["expected_version"]:', "if False:"),
    ("the event is published without its entity change",
     "candidate[entity_path] = json.dumps({", "json.dumps({"),
    ("the journal drops an operation while its push is under way",
     "parent, files = self.candidate",
     'parent, files = self.candidate\n            del self.journal[record["intent"]["event_id"]]'),
]

ORACLE = {
    "conformance": [*SPINE, "conform", "spec/ref_sync/spine.yaml",
                    "--adapter", "conformance/adapter_ref_sync.py"],
    "checks": [sys.executable, str(TOOLS / "run_checks.py"), "checks"],
} if TOOLS else {}

# run_checks.py prints one line per failing check: "... check-failed  <file>::<test>: <error>"
FAILED_CHECK = re.compile(r"check-failed\s+\S*?([^/\s]+)\.py::(\w+): (?:\w+: )?(.*)")


def killers(env):
    """The parts of the oracle that fail, and the first problem one of them reports."""
    found, first = [], None
    for label, cmd in ORACLE.items():
        p = subprocess.run(cmd, cwd=ROOT, env=env, capture_output=True, text=True)
        if p.returncode != 1:
            continue
        failed = [m.groups() for m in map(FAILED_CHECK.search, p.stdout.splitlines()) if m]
        if not failed:
            found.append(label)
            first = first or (p.stdout + p.stderr).strip().splitlines()[-1]
            continue
        found += [test for _, test, _ in failed]
        # the shortest trace follows "; after:"; the problem alone says which invariant broke
        first = first or failed[0][2].split("; after:")[0]
    return found, first


def main():
    if TOOLS is None:
        print(f"mutate_ref_sync: no tools/run_checks.py in {ROOT} or above it", file=sys.stderr)
        return 2
    if not SPINE:
        print("mutate_ref_sync: VERIFY_SPINE is not set; ./verify --full sets it", file=sys.stderr)
        return 2
    source = (ROOT / "src" / "ref_sync_client.py").read_text()
    survivors = 0
    for name, find, replace in MUTANTS:
        if source.count(find) != 1:
            print(f"error   mutant-stale       {name}: anchor not found once in src/ref_sync_client.py")
            survivors += 1
            continue
        with tempfile.TemporaryDirectory() as tmp:
            Path(tmp, "ref_sync_client.py").write_text(source.replace(find, replace))
            env = dict(os.environ, PYTHONPATH=os.pathsep.join([tmp, "src", "conformance"]))
            killed_by, first = killers(env)
        if killed_by:
            print(f"info    mutant-killed      {name}: caught by {', '.join(killed_by)}; first: {first}")
        else:
            survivors += 1
            print(f"error   mutant-survived    {name}: no part of the oracle noticed")
    print(f"-- {survivors} errors, {len(MUTANTS)} mutants")
    return 1 if survivors else 0


if __name__ == "__main__":
    sys.exit(main())
