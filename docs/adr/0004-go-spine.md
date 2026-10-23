# 0004. `spine` in Go, checked against the Python one

- Status: accepted; the module's place in decision 1 superseded by `0006-retire-spine-py.md`
  (it lives in `tools/`), and decision 3 ended there: `tools/spine.py` retired; decision 8
  superseded by `0007-no-language-built-in.md` (`conform` takes `--adapter-cmd` only)
- Date: 2026-10-15
- Issue: #27

## Context

`docs/design.md` listed "Python or a single Go binary" as an open decision. Every caller needed
Python with PyYAML to run `spine`, even to check a project not written in Python. The owner
settled it on 2026-10-15 in #27.

## Decision

1. **`spine` moves to Go:** `cmd/spine/` and `internal/spine/`, one module at the repo root
   (`github.com/23min/verifier-kit`), built with `go build -o bin/spine ./cmd/spine` (`bin/` is
   ignored by git).
2. **The examples stay in Python,** and so do `tools/run_checks.py` and
   `tools/adapter_shim.py`, as helpers for projects written in Python. The examples are code
   under test, a project that uses the toolkit, and the toolkit's own tests. What a project in
   another language still lacks is in `./verify`, which gets its own issue.
3. **`tools/spine.py` is the reference until the two agree.** `go test ./...` runs both on the
   same inputs and fails on any difference:
   - the repo's spines, readings, behaviors, checks, adapters and policy;
   - the faults `demo/faults.sh` seeds;
   - handmade adapters: Python files, and a program that bends each part of the adapter
     protocol;
   - small inputs that between them give every finding code the contract lists for the ported
     commands;
   - every seed file with an odd value or key put in each place;
   - YAML spellings where names go;
   - about 3,100 conditions;
   - argparse's corners.

   Exit codes must match. JSON findings and JUnit reports are compared after parsing, text and
   `hash` output byte for byte. Where Python's own libraries write the message, a PyYAML error
   inside `spec-unreadable` or an argparse usage error, everything but the message is compared.
4. **Three steps:** `check`, `hash`, `stale`, `behaviors`, `trace` and `guard`; then `diff`;
   then `conform`; until each was ported the Go `spine` refused it as a usage error (exit 2).
   All three are done. `./verify`, `demo/faults.sh` and the mutation scripts keep running
   `tools/spine.py` until it retires, in its own issue.
5. **YAML:** `go.yaml.in/yaml/v4`, pinned at v4.0.0-rc.6 until 4.0.0 is out, for its node tree
   only. `internal/spine` builds values from the tree with PyYAML's rules: YAML 1.1 booleans,
   `012`, `1:20`, dates and merge keys. It adds the checks v4
   doesn't make: duplicate anchors, UTF-8, and simple keys over 1,024 characters. It reads the
   escape `\/` as `/`, as PyYAML does; v4 refuses it. On 205 test inputs, v4's parser agreed
   with PyYAML on 200 and goccy/go-yaml v1.19.2 on 194; both matched every real file.
6. **Conditions:** a Go tokenizer and parser for the Python expression grammar. It gives
   `bad-condition` for the same texts as `ast.parse`, and evaluates with Python's semantics.
7. **The command line follows Python 3.13's argparse,** the version CI runs. Help is
   argparse's text at 80 columns.
8. **`--adapter FILE` (with `conform`):** both run the file through `tools/adapter_shim.py`,
   the Go `spine` with `$PYTHON` or `python3`, `tools/spine.py` with its own Python. Run in
   process, as before, it differed from the shim: a missing function, a value JSON can't encode
   (a set), a tuple (which JSON makes a list) and an adapter's printing (which went into spine's
   own output, breaking `--format json`). The owner chose on 2026-10-17 to run both through the
   shim, which now checks for the three functions, rather than record those as differences.
   This supersedes ADR 0003's decision 7. It costs time: `./verify --full` took 25 s instead of
   10 s, and `demo/faults.sh` 97 s instead of 47 s, measured before the Go `spine` ran either.
   (Superseded on 2026-10-23: `conform --adapter` and the shim built into `spine` are gone; a
   Python adapter's command line runs the shim itself; see `0007-no-language-built-in.md`.)

While porting, the owner settled behavior the port would otherwise have copied by accident:

- PyYAML doesn't read `y` as a boolean. The hint and the `yaml-boolean-key` row name `on`,
  `off`, `yes` and `no`.
- Every name and value the format defines is text ("Text only" in `spine-format.md`). An `in`
  that can't be evaluated is a `bad-condition`.
- A step names a declared event and gives every declared variable exactly once, with one of
  its values, and no other variable; anything else is a `bad-step`. `expect` names a declared
  state (a worked example may also say `rejected` or `undefined`), or it is a `bad-value`.
  Before, a misspelling in a behavior under `invalid/` made it pass.
- `hash` refuses `--format` and `--junit`.
- `spec-unreadable` covers any file PyYAML fails on, and a check file `stale` can't read.
- A file that escapes a surrogate, such as `"\ud800"`, is `spec-unreadable`. PyYAML read it, and
  printing the name crashed `tools/spine.py`; go-yaml refuses it.
- A mapping that gives a key twice is `spec-unreadable`, as the YAML spec allows a key once;
  PyYAML kept the last. Settled on 2026-10-19 in #55, after `tools/spine.py` retired.
- `guard` checks a role's patterns in a fixed order, and accepts files after `--branch`.
- The JUnit report writes each character XML can't hold as an escape such as `\x1b`, and a
  byte of a file name that isn't UTF-8 as `\xff`. Before, such a report didn't parse.

## Consequences

- Once `./verify` runs `bin/spine`, the spine steps need no Python. Content checks, adapters
  and mutation still run as Python until `./verify` can run them as commands, which is planned
  after this port.
  Until `spine.py` retires, two implementations are kept in step: a change to either fails
  `go test ./...` until the other follows.
- Known differences, none in a realistic spine:
  - Characters Python's Unicode tables don't have yet, 15.1 in Python 3.13: Go's newer tables
    call them letters, digits and printable, in `stale`'s `covers:` names, in messages and
    in `diff --depth`.
  - `\N{name}` escapes in conditions: Go has no table of character names, so it accepts any
    name.
  - Identifiers that NFKC-normalize to a variable, such as a fullwidth `ｒole`.
  - YAML's non-specific tag `!` on a plain scalar.
  - Help wraps at 80 columns whatever the terminal's width.
  - Where Python crashes, the Go `spine` reports: a JUnit report that can't be written exits
    1 with a message instead of a traceback, and an adapter's reply nested about 10,000 deep,
    past Python's recursion limit, is read.
  - An adapter's reply that escapes a lone surrogate, such as `"\ud800"`: the Go `spine`
    reads it as U+FFFD; `tools/spine.py` keeps the surrogate, and its text output crashes.
  - The shim's own folder is on the adapter's import path: `tools/` for `tools/spine.py`, a
    temporary folder for the Go `spine`, which carries the shim inside it. An adapter that
    imports `spine` or `run_checks` from `tools/` loads only under `tools/spine.py`.
- Command lines that Python 3.12 and 3.13 parse differently follow 3.13, such as files
  starting with `-` after `--branch`.
- The differential test takes about 20 s on 16 cores. It needs Python with PyYAML, and fails
  rather than skips without it.

## Alternatives considered

- **Stay in Python.** No second implementation to keep in step, but every project needs a
  Python with PyYAML to be checked.
- **Decoding with goccy/go-yaml, or with v4 into Go values.** Common Go YAML libraries decode
  as YAML 1.2, so `on`, `012` and dates would need undoing afterwards. Building values from
  the node tree keeps PyYAML's rules in one place.
- **Comparing findings by code and place only.** Easier to keep passing, but messages carry
  the witnesses, and a wrong one would go unnoticed.
