#!/usr/bin/env python3
"""Run every test_* function in test_*.py files under a folder. No dependencies.

Use pytest instead if you have it; this exists so the sample runs on a bare Python.
Exit 1 if any check fails or any test file cannot be imported; the other files still run.
With --require-checks, finding no checks at all is an error too (./verify passes it; callers
that only want a pass/fail on existing checks leave it off).
"""
import importlib.util
import sys
import traceback
from pathlib import Path


def main(folder, require=False):
    failures = passed = unloadable = 0
    for path in sorted(Path(folder).rglob("test_*.py")):
        spec = importlib.util.spec_from_file_location(path.stem, path)
        module = importlib.util.module_from_spec(spec)
        try:
            spec.loader.exec_module(module)
        except Exception as exc:
            unloadable += 1
            print(f"{'error':7} {'check-unloadable':18} {path}: cannot be imported: {type(exc).__name__}: {exc}")
            continue
        for name in sorted(n for n in dir(module) if n.startswith("test_")):
            try:
                getattr(module, name)()
                passed += 1
            except Exception:
                failures += 1
                last = traceback.format_exc().strip().splitlines()[-1]
                print(f"error   check-failed       {path}::{name}: {last}")
    run = passed + failures
    errors = failures + unloadable
    if run == 0 and require:
        errors += 1
        print(f"{'error':7} {'no-checks':18} {folder}: no test_* functions found")
    print(f"-- {errors} errors, {run} checks run")
    return 1 if errors else 0


if __name__ == "__main__":
    args = sys.argv[1:]
    require = "--require-checks" in args
    folders = [a for a in args if a != "--require-checks"]
    sys.exit(main(folders[0] if folders else "checks", require))
