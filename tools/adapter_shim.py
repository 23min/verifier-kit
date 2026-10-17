#!/usr/bin/env python3
"""Serve a Python adapter over the adapter protocol, so `spine conform --adapter-cmd` can drive it.

  spine conform SPEC --adapter-cmd "python3 tools/adapter_shim.py conformance/adapter_notes.py"

Reads one JSON request per line on stdin and writes one JSON reply per line on stdout, flushed
(docs/reference/tool-contract.md, "The adapter protocol"). The adapter's own printing goes to
stderr, so it cannot break the protocol. Exits 0 at the end of input, and 1 when the adapter
cannot be imported or does not define new(), apply() and observe(), with the reason on stderr
as one line: "adapter_shim: ADAPTER_FILE <reason>". No dependencies.
"""
import importlib.util
import json
import sys
from pathlib import Path


class LoadError(Exception):
    """The adapter file cannot be imported or lacks a function; the message says which."""


def describe(exc):
    return f"{type(exc).__name__}: {exc}"


def load(path):
    sys.path.insert(0, str(Path(path).resolve().parent))
    module_spec = importlib.util.spec_from_file_location("shim_adapter", path)
    if module_spec is None:
        raise LoadError("cannot be imported: not a Python file")
    module = importlib.util.module_from_spec(module_spec)
    try:
        module_spec.loader.exec_module(module)
    except Exception as exc:
        raise LoadError(f"cannot be imported: {describe(exc)}") from exc
    missing = [f"{name}()" for name in ("new", "apply", "observe") if not callable(getattr(module, name, None))]
    if missing:
        raise LoadError(f"does not define {', '.join(missing)}")
    return module


def reply(adapter, request, handle):
    """Returns (reply, handle). An exception from the adapter becomes an error reply."""
    try:
        op = request["op"]
        if op == "new":
            return {"ok": True}, adapter.new()
        if op == "apply":
            return {"result": adapter.apply(handle, request["event"], request["env"])}, handle
        if op == "observe":
            return {"state": adapter.observe(handle)}, handle
        return {"error": f"unknown op {op!r}"}, handle
    except Exception as exc:
        return {"error": describe(exc)}, handle


def main(argv):
    if len(argv) != 1:
        print("usage: adapter_shim.py ADAPTER_FILE", file=sys.stderr)
        return 2
    out, sys.stdout = sys.stdout, sys.stderr
    try:
        adapter = load(argv[0])
    except LoadError as exc:
        print(f"adapter_shim: {argv[0]} {exc}", file=sys.stderr)
        return 1
    handle = None
    for line in iter(sys.stdin.readline, ""):
        try:
            request = json.loads(line)
        except ValueError as exc:
            answer = {"error": f"request is not JSON: {exc}"}
        else:
            answer, handle = reply(adapter, request, handle)
        out.write(json.dumps(answer) + "\n")
        out.flush()
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
