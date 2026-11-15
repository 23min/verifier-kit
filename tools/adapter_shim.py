#!/usr/bin/env python3
"""Serve a Python adapter over the adapter protocol, so `spine conform --adapter-cmd` can drive it.

  env PYTHONPATH=src adapter_shim.py conformance/adapter_notes.py

as a line in conformance/adapter_notes.cmd, which ./verify runs with the toolkit's folder, where
this file is, first on PATH.

Reads one JSON request per line on stdin and writes one JSON reply per line on stdout, flushed
(docs/reference/tool-contract.md, "The adapter protocol"). When the adapter also defines
outputs(), the reply to each apply carries what it returns, the outputs the step sent (#116). The
adapter's own printing goes to stderr, so it cannot break the protocol. Exits 0 at the end of
input, and 1 when the adapter cannot be imported, does not define new(), apply() and observe(),
or raises in outputs(), with the reason on stderr as one line: "adapter_shim: ADAPTER_FILE
<reason>". No dependencies.
"""
import importlib.util
import json
import sys
from pathlib import Path


class LoadError(Exception):
    """The adapter file cannot be imported or lacks a function; the message says which."""


class OutputsError(Exception):
    """outputs() raised: the adapter can't read what a step sent, so nothing can be compared."""


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
    """Returns (reply, handle). An exception from the adapter becomes an error reply, except one
    from outputs(), which raises OutputsError."""
    try:
        op = request["op"]
        if op == "new":
            return {"ok": True}, adapter.new()
        if op == "apply":
            answer = {"result": adapter.apply(handle, request["event"], request["env"])}
            if callable(getattr(adapter, "outputs", None)):
                try:
                    answer["outputs"] = adapter.outputs(handle)
                except Exception as exc:
                    raise OutputsError(f"raised in outputs(): {describe(exc)}") from exc
            return answer, handle
        if op == "observe":
            return {"state": adapter.observe(handle)}, handle
        return {"error": f"unknown op {op!r}"}, handle
    except OutputsError:
        raise
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
            try:
                answer, handle = reply(adapter, request, handle)
            except OutputsError as exc:
                print(f"adapter_shim: {argv[0]} {exc}", file=sys.stderr)
                return 1
        out.write(json.dumps(answer) + "\n")
        out.flush()
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
