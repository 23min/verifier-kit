#!/usr/bin/env python3
"""spine: mechanical checks for state-machine specs, and conformance of code to them.

Prototype. Every command validates the spine file first.

  check      completeness, determinism, reachability, worked examples
  diff       two readings of the same section: disagreeing cases and shortest witnesses
  hash       one content hash per state/event cell
  stale      checks that cite cells which changed, vanished, or were never covered
  behaviors  a folder of behaviors: valid/ must be allowed, invalid/ must be refused
  trace      one behavior file (for tools that call a validator per fixture)
  conform    run generated event sequences against code through an adapter
  guard      changed files a role may not touch

Exit codes: 0 no errors, 1 findings at error severity, 2 usage error.
"""
import argparse
import ast
import fnmatch
import hashlib
import importlib.util
import itertools
import json
import re
import sys
import xml.etree.ElementTree as ET
from collections import deque
from pathlib import Path

import yaml


class Findings(list):
    def add(self, code, where, message, severity="error"):
        self.append({"code": code, "severity": severity, "where": str(where), "message": message})

    @property
    def errors(self):
        return [f for f in self if f["severity"] == "error"]


# ---------- loading and validation ----------

TOP_KEYS = {"machine", "section", "vars", "states", "initial", "events", "rules", "examples"}
REQUIRED_KEYS = {"machine", "states", "initial", "events", "rules"}
STATE_KEYS = {"kind", "terminal"}
STATE_KINDS = {"fact", "policy", "design"}
RULE_KEYS = {"from", "event", "when", "to", "reject", "ignore", "stay"}
OUTCOMES = ("to", "reject", "ignore", "stay")
BOOL_KEY_HINT = "YAML reads keys such as `on`, `yes` and `y` as true; spell the key as `event` or quote it"


def load_yaml(path):
    with open(path) as f:
        return yaml.safe_load(f)


def load_spec(path):
    """Load and validate a spine file. Returns (spec, findings); spec is None when invalid."""
    try:
        spec = load_yaml(path)
    except (OSError, yaml.YAMLError) as exc:
        out = Findings()
        out.add("spec-unreadable", path, str(exc).splitlines()[0])
        return None, out
    out = validate(spec, path)
    return (None if out.errors else spec), out


def validate(spec, path):
    out = Findings()
    if not isinstance(spec, dict):
        out.add("spec-shape", path, "the top level must be a mapping")
        return out
    for key in spec:
        if not isinstance(key, str):
            out.add("yaml-boolean-key", path, f"top-level key read as {key!r}; {BOOL_KEY_HINT}")
        elif key not in TOP_KEYS:
            out.add("unknown-key", path, f'unknown top-level key "{key}"')
    for key in sorted(REQUIRED_KEYS - set(spec)):
        out.add("missing-key", path, f'required key "{key}" is missing')
    if out.errors:
        return out

    states, events, variables = spec["states"], spec["events"], spec.get("vars") or {}
    if not isinstance(states, dict) or not states:
        out.add("spec-shape", "states", "states must be a non-empty mapping")
        return out
    for name, meta in states.items():
        meta = meta or {}
        if not isinstance(meta, dict):
            out.add("spec-shape", f"states/{name}", "state details must be a mapping")
            continue
        for key in meta:
            if key not in STATE_KEYS:
                out.add("unknown-key", f"states/{name}", f'unknown state key "{key}"')
        if "kind" in meta and meta["kind"] not in STATE_KINDS:
            out.add("bad-value", f"states/{name}", f'kind must be one of {sorted(STATE_KINDS)}')
    if not isinstance(events, list) or not all(isinstance(e, str) for e in events):
        out.add("spec-shape", "events", "events must be a list of names")
        return out
    if not isinstance(variables, dict):
        out.add("spec-shape", "vars", "vars must map each variable to its list of values")
        return out
    for name, values in variables.items():
        if not isinstance(values, list) or not values:
            out.add("spec-shape", f"vars/{name}", "each variable needs a non-empty list of values")
        elif any(isinstance(v, bool) for v in values):
            out.add("yaml-boolean-value", f"vars/{name}", "a value was read as true/false; quote it")
    if spec["initial"] not in states:
        out.add("unknown-state", "initial", f'initial state "{spec["initial"]}" is not declared')

    if not isinstance(spec["rules"], list):
        out.add("spec-shape", "rules", "rules must be a list")
        return out
    for i, rule in enumerate(spec["rules"], 1):
        where = f"rules[{i}]"
        if not isinstance(rule, dict):
            out.add("spec-shape", where, "each rule must be a mapping")
            continue
        for key in rule:
            if not isinstance(key, str):
                out.add("yaml-boolean-key", where, f"key read as {key!r}; {BOOL_KEY_HINT}")
            elif key not in RULE_KEYS:
                out.add("unknown-key", where, f'unknown rule key "{key}"')
        for key in ("from", "event"):
            if key not in rule:
                out.add("missing-key", where, f'rule needs "{key}"')
        given = [k for k in OUTCOMES if k in rule]
        if len(given) != 1:
            out.add("bad-outcome", where, f"a rule needs exactly one of {', '.join(OUTCOMES)}")
        elif given[0] != "to" and rule[given[0]] is not True:
            out.add("bad-outcome", where, f"{given[0]} must be true")
        if "when" in rule:
            try:
                compile_condition(rule["when"], variables)
            except ValueError as exc:
                out.add("bad-condition", where, str(exc))
    return out


# ---------- conditions: a small, safe expression language ----------
# Allowed: variable names, string/number literals, lists of literals,
# ==, !=, in, not in, and, or, not, parentheses.

_COMPILED = {}


def compile_condition(text, variables):
    key = (text, tuple(sorted(variables)))
    if key in _COMPILED:
        return _COMPILED[key]
    if not isinstance(text, str):
        raise ValueError(f"condition must be text, got {text!r}")
    try:
        tree = ast.parse(text, mode="eval").body
    except SyntaxError:
        raise ValueError(f"cannot parse condition {text!r}") from None
    _check_condition(tree, variables, text)
    _COMPILED[key] = tree
    return tree


def _check_condition(node, variables, text):
    if isinstance(node, ast.BoolOp):
        for value in node.values:
            _check_condition(value, variables, text)
    elif isinstance(node, ast.UnaryOp) and isinstance(node.op, ast.Not):
        _check_condition(node.operand, variables, text)
    elif isinstance(node, ast.Compare):
        if not all(isinstance(op, (ast.Eq, ast.NotEq, ast.In, ast.NotIn)) for op in node.ops):
            raise ValueError(f"only ==, !=, in and not in are allowed in {text!r}")
        for part in [node.left, *node.comparators]:
            _check_condition(part, variables, text)
    elif isinstance(node, ast.Name):
        if node.id not in variables:
            raise ValueError(f'condition {text!r} uses undeclared variable "{node.id}"')
    elif isinstance(node, ast.Constant) and isinstance(node.value, (str, int, float)):
        pass
    elif isinstance(node, (ast.List, ast.Tuple)):
        for element in node.elts:
            if not (isinstance(element, ast.Constant) and isinstance(element.value, (str, int, float))):
                raise ValueError(f"lists may hold only literals in {text!r}")
    else:
        raise ValueError(f"unsupported element {type(node).__name__} in {text!r}")


_CMP = {ast.Eq: lambda a, b: a == b, ast.NotEq: lambda a, b: a != b,
        ast.In: lambda a, b: a in b, ast.NotIn: lambda a, b: a not in b}


def _evaluate(node, env):
    if isinstance(node, ast.BoolOp):
        values = (_evaluate(v, env) for v in node.values)
        return all(values) if isinstance(node.op, ast.And) else any(values)
    if isinstance(node, ast.UnaryOp):
        return not _evaluate(node.operand, env)
    if isinstance(node, ast.Compare):
        left = _evaluate(node.left, env)
        for op, comparator in zip(node.ops, node.comparators):
            right = _evaluate(comparator, env)
            if not _CMP[type(op)](left, right):
                return False
            left = right
        return True
    if isinstance(node, ast.Name):
        return env[node.id]
    if isinstance(node, ast.Constant):
        return node.value
    return [e.value for e in node.elts]


# ---------- the machine ----------

def assignments(spec):
    """Every combination of the declared variable values, in a stable order."""
    variables = spec.get("vars") or {}
    names = sorted(variables)
    for combo in itertools.product(*(variables[n] for n in names)):
        yield dict(zip(names, combo))


def holds(spec, rule, env):
    if "when" not in rule:
        return True
    return bool(_evaluate(compile_condition(rule["when"], spec.get("vars") or {}), env))


def outcome(rule):
    if "to" in rule:
        return ("to", rule["to"])
    return next(((k, None) for k in ("reject", "ignore", "stay") if rule.get(k)), ("invalid", None))


def rules_for(spec, state, event):
    return [r for r in spec["rules"] if r["from"] == state and r["event"] == event]


def decide(spec, state, event, env):
    """Every distinct outcome the spec allows for one event, in one state, with one set of values."""
    return sorted({outcome(r) for r in rules_for(spec, state, event) if holds(spec, r, env)}, key=str)


def terminal_states(spec):
    return {s for s, meta in spec["states"].items() if (meta or {}).get("terminal")}


def show(outcomes):
    if not outcomes:
        return "nothing (no rule matches)"
    return " and ".join(f"-> {t}" if k == "to" else k for k, t in outcomes)


def envstr(env):
    return ", ".join(f"{k}={v}" for k, v in env.items())


def parse_step(text):
    event, *pairs = text.split()
    return event, dict(p.split("=", 1) for p in pairs)


def step(spec, state, event, env):
    """Apply one event. Returns (new_state, verdict); verdict is None, 'rejected' or 'undefined'."""
    hits = decide(spec, state, event, env)
    if len(hits) != 1:
        return state, "undefined"
    kind, target = hits[0]
    if kind == "reject":
        return state, "rejected"
    return (target if kind == "to" else state), None


def run(spec, steps):
    state = spec["initial"]
    for event, env in steps:
        state, verdict = step(spec, state, event, env)
        if verdict:
            return verdict
    return state


# ---------- check ----------

def check(spec):
    out = Findings()
    states, events = spec["states"], spec["events"]
    terminal = terminal_states(spec)

    for r in spec["rules"]:
        where = f'{r["from"]}/{r["event"]}'
        if r["from"] not in states:
            out.add("unknown-state", where, f'rule starts in undeclared state "{r["from"]}"')
        if "to" in r and r["to"] not in states:
            out.add("unknown-state", where, f'rule goes to undeclared state "{r["to"]}"')
        if r["event"] not in events:
            out.add("unknown-event", where, f'rule uses undeclared event "{r["event"]}"')

    for s in states:
        for e in events:
            where, rules = f"{s}/{e}", rules_for(spec, s, e)
            if s in terminal:
                if any(outcome(r)[0] == "to" for r in rules):
                    out.add("terminal-has-exit", where, f'"{s}" is terminal but "{e}" leaves it')
                continue
            if not rules:
                out.add("undecided-cell", where, f'nothing says what "{e}" does in "{s}"')
                continue
            for env in assignments(spec):
                hits = decide(spec, s, e, env)
                if not hits:
                    out.add("missing-case", where, f"no rule covers {envstr(env) or 'this case'}")
                elif len(hits) > 1:
                    out.add("overlap", where, f"{envstr(env) or 'this case'} matches conflicting rules: {show(hits)}")

    seen, queue = {spec["initial"]}, deque([spec["initial"]])
    while queue:
        s = queue.popleft()
        for e in events:
            for env in assignments(spec):
                for kind, t in decide(spec, s, e, env):
                    if kind == "to" and t in states and t not in seen:
                        seen.add(t)
                        queue.append(t)
    for s in states:
        if s not in seen:
            out.add("unreachable", s, f'no sequence of events reaches "{s}" from "{spec["initial"]}"')

    for ex in spec.get("examples") or []:
        got = run(spec, [parse_step(x) for x in ex["steps"]])
        if got != ex["expect"]:
            out.add("example-fails", ex["name"], f'expected {ex["expect"]}, got {got}')
    return out


# ---------- diff ----------

def diff(a, b, depth):
    out = Findings()
    for part in ("states", "events"):
        for x in sorted(set(a[part]) - set(b[part])):
            out.add("only-in-a", part, f'A has {part[:-1]} "{x}", B does not')
        for x in sorted(set(b[part]) - set(a[part])):
            out.add("only-in-b", part, f'B has {part[:-1]} "{x}", A does not')
    if (a.get("vars") or {}) != (b.get("vars") or {}):
        out.add("vars-differ", "vars", "A and B declare different variables; case comparison skipped")
        return out
    states = sorted(set(a["states"]) & set(b["states"]))
    events = [e for e in a["events"] if e in b["events"]]

    for s in states:
        for e in events:
            for env in assignments(a):
                oa, ob = decide(a, s, e, env), decide(b, s, e, env)
                if oa != ob:
                    out.add("case-disagrees", f"{s}/{e}", f"{envstr(env)}: A says {show(oa)}, B says {show(ob)}")

    steps = [(e, env) for e in events for env in assignments(a)]
    reported, queue = set(), deque([()])
    while queue:
        seq = queue.popleft()
        ra, rb = run(a, seq), run(b, seq)
        if ra != rb:
            key = (run(a, seq[:-1]), run(b, seq[:-1]), seq[-1][0], tuple(seq[-1][1].items()))
            if key not in reported:
                reported.add(key)
                trace_text = " -> ".join(f"{e}({envstr(env)})" for e, env in seq)
                out.add("behavior-differs", trace_text, f"A ends {ra}, B ends {rb}")
            continue
        if len(seq) < depth and ra not in ("rejected", "undefined"):
            queue.extend(seq + (st,) for st in steps)
    return out


# ---------- hash / stale ----------

def cell_hash(spec, s, e):
    canon = sorted(json.dumps(r, sort_keys=True) for r in rules_for(spec, s, e))
    return hashlib.sha256("\n".join(canon).encode()).hexdigest()[:12]


COVERS = re.compile(r"covers:\s*([\w-]+)/([\w-]+)@([0-9a-f]+)")


def stale(spec, checks_dir):
    out = Findings()
    covered = set()
    for path in sorted(Path(checks_dir).rglob("*")):
        if not path.is_file() or "__pycache__" in path.parts:
            continue
        for s, e, h in COVERS.findall(path.read_text(errors="replace")):
            cell = f"{s}/{e}"
            if s not in spec["states"] or e not in spec["events"] or not rules_for(spec, s, e):
                out.add("orphan-check", path, f"covers {cell}, which the spec no longer defines")
            elif cell_hash(spec, s, e) != h:
                out.add("stale-check", path,
                        f"covers {cell}@{h}, but that cell now hashes to {cell_hash(spec, s, e)}: re-ratify, then update the check")
            else:
                covered.add(cell)
    for cell in sorted({f'{r["from"]}/{r["event"]}' for r in spec["rules"] if "to" in r} - covered):
        out.add("uncovered", cell, "no check cites this transition; conformance still exercises it", "info")
    return out


# ---------- behaviors / trace ----------

def replay_behavior(spec, behavior):
    """Returns (allowed, explanation)."""
    state = spec["initial"]
    for i, text in enumerate(behavior.get("steps") or [], 1):
        event, env = parse_step(text)
        state, verdict = step(spec, state, event, env)
        if verdict:
            return False, f'step {i} "{text}" is {verdict} in state {state}'
    if "expect" in behavior and state != behavior["expect"]:
        return False, f'ends in {state}, expected {behavior["expect"]}'
    return True, f"allowed, ends in {state}"


def behaviors(spec, directory):
    out = Findings()
    counted = 0
    for folder, must_allow in (("valid", True), ("invalid", False)):
        for path in sorted(Path(directory, folder).glob("*.y*ml")):
            counted += 1
            allowed, why = replay_behavior(spec, load_yaml(path))
            if must_allow and not allowed:
                out.add("behavior-refused", path, f"marked valid, but the spine refuses it: {why}")
            elif not must_allow and allowed:
                out.add("behavior-allowed", path, f"marked invalid, but the spine allows it ({why})")
    if counted == 0:
        out.add("no-behaviors", directory, "no behavior files found under valid/ or invalid/", "warning")
    else:
        out.add("behaviors", directory, f"{counted} behavior files checked", "info")
    return out


def trace(spec, path):
    out = Findings()
    problems = check(spec).errors
    if problems:
        out.add("spine-unsound", "spec", f"{len(problems)} errors from spine check; run it for details")
        return out
    allowed, why = replay_behavior(spec, load_yaml(path))
    if not allowed:
        out.add("refused", path, why)
    return out


# ---------- conform ----------

class AdapterFailed(Exception):
    """The adapter could not be loaded, or its new() or observe() raised: no comparison is possible."""


def describe(exc):
    return f"{type(exc).__name__}: {exc}"


def steps_text(steps):
    return " -> ".join(f"{e}({envstr(v)})" for e, v in steps)


def load_adapter(path):
    sys.path.insert(0, str(Path(path).resolve().parent))
    module_spec = importlib.util.spec_from_file_location("spine_adapter", path)
    if module_spec is None:
        raise AdapterFailed("cannot be imported: not a Python file")
    module = importlib.util.module_from_spec(module_spec)
    try:
        module_spec.loader.exec_module(module)
    except Exception as exc:
        raise AdapterFailed(f"cannot be imported: {describe(exc)}") from exc
    missing = [f"{name}()" for name in ("new", "apply", "observe") if not callable(getattr(module, name, None))]
    if missing:
        raise AdapterFailed(f"does not define {', '.join(missing)}")
    return module


def call_adapter(adapter, name, steps, *args):
    """Call new() or observe(). Unlike apply(), an exception here is the adapter's failure."""
    try:
        return getattr(adapter, name)(*args)
    except Exception as exc:
        after = f" after {steps_text(steps)}" if steps else ""
        raise AdapterFailed(f"{name}() raised {describe(exc)}{after}") from exc


def replay(spec, adapter, seq, exercised):
    """Run one event sequence against the spine (prediction) and the code (through the adapter).

    Returns (divergence or None, spine state afterwards, kind of the last outcome). Raises
    AdapterFailed when new() or observe() raises.
    """
    handle = call_adapter(adapter, "new", ())
    state = spec["initial"]
    observed = call_adapter(adapter, "observe", (), handle)
    if observed != state:
        return ({"key": ("start",), "where": "start",
                 "message": f"spine starts in {state}, code starts in {observed}"}, state, None)
    kind = None
    for i, (event, env) in enumerate(seq):
        hits = decide(spec, state, event, env)
        if len(hits) != 1:
            return None, state, "undefined"   # spine check reports these; nothing to compare
        kind, target = hits[0]
        exercised.add((state, event, tuple(env.items())))
        expect = ("rejected" if kind == "reject" else "ok", target if kind == "to" else state)
        try:
            result = adapter.apply(handle, event, env)
        except Exception as exc:                # a crash is a divergence too
            result = f"crashed ({describe(exc)})"
        got = (result, call_adapter(adapter, "observe", seq[: i + 1], handle))
        if got != expect:
            return ({"key": (state, event, tuple(env.items())), "where": f"{state}/{event} {envstr(env)}",
                     "message": f"after {steps_text(seq[: i + 1])}: spine expects {expect[0]} in {expect[1]}, "
                                f"code gave {got[0]} in {got[1]}"}, state, kind)
        state = expect[1]
    return None, state, kind


def conform(spec, adapter_path, depth):
    out = Findings()
    try:
        adapter = load_adapter(adapter_path)
    except AdapterFailed as exc:
        out.add("adapter-failed", adapter_path, str(exc))
        return out
    terminal = terminal_states(spec)
    steps = [(e, env) for e in spec["events"] for env in assignments(spec)]
    exercised, reported, runs, queue = set(), set(), 0, deque([()])
    while queue:
        seq = queue.popleft()
        runs += 1
        try:
            divergence, state, kind = replay(spec, adapter, seq, exercised)
        except AdapterFailed as exc:          # keep what was found; past this, nothing compares
            out.add("adapter-failed", adapter_path, str(exc))
            return out
        if divergence:
            if divergence["key"] not in reported:
                reported.add(divergence["key"])
                out.add("diverges", divergence["where"], divergence["message"])
            continue
        if len(seq) < depth and state not in terminal and kind not in ("reject", "undefined"):
            queue.extend(seq + (st,) for st in steps)
    total = sum(1 for s in spec["states"] if s not in terminal) * len(steps)
    out.add("coverage", f"{runs} sequences up to {depth} steps",
            f"{len(exercised)} of {total} state/event/variable cases exercised", "info")
    return out


# ---------- guard ----------

def guard(policy, branch, files):
    out = Findings()
    role = next((r for pattern, r in policy["branches"].items() if fnmatch.fnmatch(branch, pattern)), None)
    if role is None:
        out.add("unknown-role", branch, "no policy entry matches this branch")
        return out
    rules = policy["roles"][role]
    for f in files:
        if any(fnmatch.fnmatch(f, p) for p in rules.get("deny", [])):
            out.add("path-denied", f, f"the {role} role may not change this path")
        elif not any(fnmatch.fnmatch(f, p) for p in rules.get("allow", [])):
            out.add("path-not-allowed", f, f"no allow rule for the {role} role covers this path")
    out.add("role", branch, f"checked {len(files)} changed files as role {role}", "info")
    return out


# ---------- output ----------

def emit_text(findings):
    for f in findings:
        print(f'{f["severity"]:7} {f["code"]:18} {f["where"]}: {f["message"]}')
    print(f"-- {len(findings.errors)} errors, {len(findings)} findings")


def write_junit(path, suite, findings):
    errors = findings.errors
    root = ET.Element("testsuite", name=suite, tests=str(max(1, len(errors))), failures=str(len(errors)))
    if not errors:
        ET.SubElement(root, "testcase", classname=suite, name="no errors")
    for f in errors:
        case = ET.SubElement(root, "testcase", classname=suite, name=f'{f["code"]} {f["where"]}')
        failure = ET.SubElement(case, "failure", message=f["message"])
        failure.text = f["message"]
    Path(path).parent.mkdir(parents=True, exist_ok=True)
    ET.ElementTree(root).write(path, encoding="utf-8", xml_declaration=True)


def main(argv=None):
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--format", choices=["text", "json"], default="text")
    common.add_argument("--junit", metavar="FILE", help="also write a JUnit XML report")
    p = argparse.ArgumentParser(prog="spine", description=__doc__.splitlines()[0])
    sub = p.add_subparsers(dest="cmd", required=True)
    for name in ("check", "hash"):
        sub.add_parser(name, parents=[common]).add_argument("spec")
    d = sub.add_parser("diff", parents=[common])
    d.add_argument("a")
    d.add_argument("b")
    d.add_argument("--depth", type=int, default=4)
    s = sub.add_parser("stale", parents=[common])
    s.add_argument("spec")
    s.add_argument("checks")
    b = sub.add_parser("behaviors", parents=[common])
    b.add_argument("spec")
    b.add_argument("directory")
    t = sub.add_parser("trace", parents=[common])
    t.add_argument("spec")
    t.add_argument("behavior")
    k = sub.add_parser("conform", parents=[common])
    k.add_argument("spec")
    k.add_argument("--adapter", required=True, help="Python file defining new(), apply(), observe()")
    k.add_argument("--depth", type=int, default=4)
    g = sub.add_parser("guard", parents=[common])
    g.add_argument("policy")
    g.add_argument("--branch", required=True)
    g.add_argument("files", nargs="*", help="changed files; read from stdin when omitted")
    args = p.parse_args(argv)

    findings = Findings()
    if args.cmd == "guard":
        files = args.files or [line.strip() for line in sys.stdin if line.strip()]
        findings = guard(load_yaml(args.policy), args.branch, files)
    else:
        paths = [args.a, args.b] if args.cmd == "diff" else [args.spec]
        specs = []
        for path in paths:
            spec, problems = load_spec(path)
            findings.extend(problems)
            specs.append(spec)
        if not findings.errors:
            if args.cmd == "hash":
                spec = specs[0]
                for st in spec["states"]:
                    for ev in spec["events"]:
                        if rules_for(spec, st, ev):
                            print(f"{st}/{ev}@{cell_hash(spec, st, ev)}")
                return 0
            handler = {
                "check": lambda: check(specs[0]),
                "diff": lambda: diff(specs[0], specs[1], args.depth),
                "stale": lambda: stale(specs[0], args.checks),
                "behaviors": lambda: behaviors(specs[0], args.directory),
                "trace": lambda: trace(specs[0], args.behavior),
                "conform": lambda: conform(specs[0], args.adapter, args.depth),
            }[args.cmd]
            findings.extend(handler())

    if args.format == "json":
        print(json.dumps({"tool": "spine", "command": args.cmd, "findings": findings}, indent=2))
    else:
        emit_text(findings)
    if args.junit:
        write_junit(args.junit, f"spine {args.cmd}", findings)
    return 1 if findings.errors else 0


if __name__ == "__main__":
    sys.exit(main())
