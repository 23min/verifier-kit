#!/usr/bin/env python3
"""Run every test_* function in test_*.py files under a folder. No dependencies.

Use pytest instead if you have it; this exists so the sample runs on a bare Python.
Exit 1 if any check fails.
"""
import importlib.util
import sys
import traceback
from pathlib import Path


def main(folder):
    failures = passed = 0
    for path in sorted(Path(folder).rglob("test_*.py")):
        spec = importlib.util.spec_from_file_location(path.stem, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        for name in sorted(n for n in dir(module) if n.startswith("test_")):
            try:
                getattr(module, name)()
                passed += 1
            except Exception:
                failures += 1
                last = traceback.format_exc().strip().splitlines()[-1]
                print(f"error   check-failed       {path}::{name}: {last}")
    print(f"-- {failures} errors, {passed + failures} checks run")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "checks"))
