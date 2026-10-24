# Tool contract

Every tool in this repo follows the rules below. They are what let the tools plug into any
agent harness, git hook or CI system without knowing about it. Changing anything on this page
needs an issue and the owner's approval first (see `CLAUDE.md`).

## Rules for every tool

- **Input is files in the repo.** No network access, no model calls. The one planned
  exception is `interpret`, whose output is files that `spine diff` then judges.
- **No knowledge of callers.** A tool never calls aiwf, a CI API, an agent or another tool's
  internals. Callers call tools, and a tool runs no program but the adapter command `conform`
  is given.
- **Same input, same output.** Results are sorted and carry no timestamps or random ids.
- **Exit codes:** `0` no errors, `1` at least one finding at error severity, `2` usage error.
- **Output:** text by default; `--format json` for machines; `--junit FILE` additionally
  writes a JUnit XML report (one test case per error, or one passing case). The report writes
  each character XML can't hold as an escape, such as `\x1b`, and a byte of a file name that isn't
  UTF-8 as `\xff`, so it always parses. `spine hash` is the exception: it prints hashes to copy,
  not findings, and takes neither option.
- **Dependencies:** `spine` uses the Go standard library plus `go.yaml.in/yaml/v4`
  (`tools/go.mod`). `tools/run_checks.py` and `tools/adapter_shim.py` use the Python standard
  library only.
- **Nothing is skipped silently.** Whatever is not checked is reported with the reason; a
  missing piece that a check needs is an error finding, never a pass.

## Findings

Text output is one line per finding, then a summary:

```
error   diverges           draft/trash role=owner: after trash(role=owner): spine expects rejected in draft, code gave ok in trashed
-- 1 errors, 2 findings
```

JSON output:

```json
{"tool": "spine", "command": "conform", "findings": [
  {"code": "diverges", "severity": "error", "where": "draft/trash role=owner", "message": "..."}
]}
```

`severity` is `error`, `warning` or `info`. Only `error` affects the exit code. Finding codes
are part of the contract: new codes may be added, existing codes are never renamed or reused.

## `spine`

| Command | Arguments | Does |
|---|---|---|
| `check` | `SPEC` | Validates the file, then: every state, event and variable combination has exactly one rule; every state is reachable; terminal states have no exits; worked examples replay to their expected end |
| `diff` | `A B [--depth N]` | Compares two spines case by case, then by behavior over every sequence up to N events (default 4), reporting the shortest witness for each difference |
| `hash` | `SPEC` | Prints the whole spine's hash, `spine@hash`, then one content hash per state/event cell: `state/event@hash`. Text only: `--format` and `--junit` are usage errors |
| `stale` | `SPEC CHECKS_DIR` | Finds `covers: state/event@hash` and `covers: spine@hash` citations in files under `CHECKS_DIR` and reports stale, orphaned and uncovered cells. A check that cites the current whole spine counts every transition as cited |
| `behaviors` | `SPEC DIR` | Replays every file in `DIR/valid/` (must be allowed) and `DIR/invalid/` (must be refused) |
| `trace` | `SPEC FILE` | Replays one behavior file; for callers that run a validator once per fixture |
| `conform` | `SPEC --adapter-cmd COMMAND [--depth N]` | Runs every event sequence up to N steps (default 4) against the spine and against the code through the adapter, and reports where they differ. The adapter is a program that speaks the adapter protocol (below); a Python adapter's command runs it through `tools/adapter_shim.py` |
| `guard` | `POLICY --branch NAME [FILES...]` | Maps the branch to a role and reports changed files that role may not touch; reads files from stdin when none are given |

`spine` is `tools/bin/spine`, built from `tools/cmd/spine/` (`go -C tools build -o bin/spine
./cmd/spine`); `./verify` builds it on every run. It replaced `tools/spine.py`, of which it is a
port (`docs/adr/0004-go-spine.md`, `docs/adr/0006-retire-spine-py.md`). `go -C tools test ./...`
checks it against a record of what it gave on every test input.

All commands except `hash` accept `--format text|json` and `--junit FILE`. Every command that
takes a spine validates it first and stops with the validation findings if it is invalid.

### Finding codes

| Code | Severity | From | Meaning |
|---|---|---|---|
| `spec-unreadable` | error | all | an input file (spine, reading, behavior file, policy) can't be read, isn't UTF-8 or isn't YAML, gives a key twice in one mapping, or escapes a surrogate such as `"\ud800"`; or a check file `stale` searches can't be read |
| `spec-shape` | error | all | a section of an input file has the wrong type, or a name or value isn't text |
| `missing-key` | error | all | a required key of an input file is missing |
| `unknown-key` | error | all | a key outside an input file's format, or a key that isn't text |
| `yaml-boolean-key` | error | all | a key YAML read as a boolean (`on`, `off`, `yes`, `no`) |
| `yaml-boolean-value` | error | all | a value YAML read as a boolean where a name was meant |
| `bad-value` | error | all | a value outside its closed set, such as a state kind, a role the policy does not define, or an `expect` that isn't a declared state |
| `listed-twice` | error | all | an event, or a value of one variable, listed more than once in a spine or reading |
| `bad-outcome` | error | all | a rule without exactly one outcome |
| `bad-condition` | error | all | a condition outside the condition language, or an `in` that can't be evaluated |
| `bad-step` | error | all | a step in a worked example or behavior file that isn't `event var=value ...`, names an undeclared event, or doesn't give each declared variable exactly once, with one of its values, and no other variable |
| `unknown-state`, `unknown-event` | error | `check` | a rule or `initial` names something undeclared |
| `undecided-cell` | error | `check` | no rule at all for a state and event |
| `missing-case` | error | `check` | no rule matches some variable values |
| `overlap` | error | `check` | two rules with different outcomes match the same case |
| `terminal-has-exit` | error | `check` | a terminal state has a transition out |
| `unreachable` | error | `check` | no event sequence reaches a state |
| `example-fails` | error | `check` | a worked example ends somewhere else |
| `only-in-a`, `only-in-b` | error | `diff` | a state or event exists in one reading only |
| `initial-differs` | error | `diff` | the readings start in different states; every sequence then starts apart, so the behavior comparison stops there |
| `vars-differ` | error | `diff` | the readings declare different variable names, or different values for one; the order of the values doesn't count |
| `case-disagrees` | error | `diff` | the readings decide a case differently |
| `behavior-differs` | error | `diff` | a shortest event sequence on which the readings end differently |
| `stale-check` | error | `stale` | a check cites a cell, or the whole spine, whose hash has changed |
| `orphan-check` | error | `stale` | a check cites a cell that no longer exists |
| `uncovered` | info | `stale` | a transition no check cites |
| `behavior-refused` | error | `behaviors` | a file in `valid/` is refused |
| `behavior-allowed` | error | `behaviors` | a file in `invalid/` is allowed |
| `no-behaviors` | warning | `behaviors` | no behavior files found |
| `behaviors` | info | `behaviors` | how many files were checked |
| `refused` | error | `trace` | the behavior is refused |
| `spine-unsound` | error | `trace` | the spine fails `check` |
| `diverges` | error | `conform` | code and spine disagree; the message carries the sequence |
| `coverage` | info | `conform` | sequences run and cases exercised |
| `adapter-failed` | error | `conform` | the adapter command can't be started, exits before replying, breaks the protocol, or replies `{"error": ...}` to `new` or `observe`; the shim does when a Python adapter's `new()` or `observe()` raised, and exits before replying when the file can't be imported or lacks a function. `conform` stops there |
| `path-denied` | error | `guard` | a deny pattern for the role matches a changed file |
| `path-not-allowed` | error | `guard` | no allow pattern for the role matches a changed file |
| `unknown-role` | error | `guard` | no branch pattern matches |
| `role` | info | `guard` | which role the branch mapped to |

## The adapter

A program that speaks the adapter protocol (below), owned by the oracle side;
`conform --adapter-cmd COMMAND` runs it. In Python it is a file with three functions, which
`tools/adapter_shim.py` serves over the protocol:

- `new()` returns a handle to a fresh instance of the system under test, in the spine's initial state.
- `apply(handle, event, env)` performs one spine event, with `env` holding the variable values
  (for example `{"role": "editor"}`), and returns `"ok"` or `"rejected"`. Any exception counts as a divergence.
- `observe(handle)` returns the name of the spine state the system is in, read from its real data.

An exception from `new()` or `observe()`, or while importing the file, is `adapter-failed`, not a
divergence: without a fresh instance or a reading of its state there is nothing to compare.

The shim runs in a Python process of its own, the one its command line names. The adapter's
printing goes to stderr, and what `apply()` and `observe()` return reaches `conform` as JSON: a
tuple arrives as a list, and a value JSON can't encode, such as a set, makes the shim fail
(`adapter-failed`).

The adapter translates and never decides whether an action is allowed.

### The adapter protocol

`spine conform --adapter-cmd COMMAND` runs the adapter as a program, so the code under test can
be in any language. COMMAND is one string, split by shell quoting rules and run without a shell
(no pipes, variables or globs), in `conform`'s working directory and environment.

`conform` writes one JSON request per line to the program's stdin and reads one JSON reply per
line from its stdout, in UTF-8. The program must flush stdout after every reply. Its stderr is its own;
`conform` passes it through and never reads it.

| Request | Reply | Same as |
|---|---|---|
| `{"op": "new"}` | `{"ok": true}` | `new()`: a fresh instance in the spine's initial state; it replaces the previous one |
| `{"op": "apply", "event": "trash", "env": {"role": "owner"}}` | `{"result": "ok"}` or `{"result": "rejected"}` | `apply()`, on the current instance |
| `{"op": "observe"}` | `{"state": "draft"}` | `observe()`, on the current instance |

There are no handles: a program holds one instance at a time. Any request may instead get
`{"error": "<what went wrong>"}`. For `apply` that counts as a divergence, as an exception from
`apply()` does, and the text appears as `crashed (<text>)`; for `new` or `observe` it is
`adapter-failed`. A program replies `{"error": ...}` to a request it does not recognize.

`conform` starts the program once, before the first `new`, and closes its stdin when done; the
program then exits. Each of these is `adapter-failed`, after which `conform` stops the program:
it can't be started, it exits before replying, a reply is not JSON, or a reply has the wrong
shape (other keys, more than one key, or anything but `true` for `ok`). There is no timeout: a
program that never replies makes `conform` wait.

`tools/adapter_shim.py ADAPTER_FILE` serves a Python adapter over this protocol unchanged, for
example `--adapter-cmd "python3 tools/adapter_shim.py conformance/adapter_notes.py"`. It sends
the adapter's own printing to stderr. When the adapter can't be imported or doesn't define
`new()`, `apply()` and `observe()`, it exits 1 with one line on stderr,
`adapter_shim: ADAPTER_FILE <reason>`, before replying to `new`.

## The policy file

```yaml
branches:              # first matching pattern wins
  "build/*": builder
  "spec/*": spec-author
  "*": person
roles:
  builder:
    allow: ["src/*", "docs/*", "README.md"]
    deny: ["spec/*", "checks/*", ...]
```

A changed file fails if any `deny` pattern matches, or if no `allow` pattern does. Patterns use
`fnmatch`, where `*` also matches `/`. `branches` and `roles` are required and nothing else is
allowed; every role a branch pattern names must be in `roles`, and a role holds only `allow`
and `deny`, each a list of patterns and each optional. Anything else is a finding.

## `./verify`

`./verify [ROOT] [--full]`. A root is a folder laid out like a project, with `spec/`,
`checks/`, `conformance/`, `src/` and `mutation/`; every path below is relative to it. A
relative ROOT is taken from the caller's working directory. Without ROOT, the roots are every
`examples/*/` folder in turn (sorted) when the folder `verify` is in has no `spec/` but has
`examples/*/`; otherwise the root is that folder itself.

`./verify` is a short Bash script. First it builds the toolkit's `spine` and `verify`, from the
`tools/` next to it: `go -C tools build -trimpath -o bin/ ./cmd/spine ./cmd/verify`. Every step
below runs that `spine`. When the build fails, or Go is missing, it prints
`FAIL  spine and verify built` with the reason and exits 1 before any other step. Otherwise it
runs `tools/bin/verify` with its arguments, which does the rest.

Runs, per `spec/<machine>/spine.yaml`: `check`, then `behaviors` over `spec/<machine>/behaviors`,
`stale` over `checks/<machine>`, and `conform` with the machine's adapter. Then the content
checks, and with `--full` every mutation command. Each of these three pieces is a command, in
any language:

| Piece | File | Run |
|---|---|---|
| the adapter, per machine | `conformance/adapter_<machine>.cmd` | its line given to `conform --adapter-cmd`, with `conform` run from the root |
| the content checks | `checks/run.cmd` | from the root |
| mutation, under `--full` | each `mutation/mutate_<name>.cmd` | from the root |

A `.cmd` file holds one command line, read as `--adapter-cmd` reads one: split by shell quoting
rules and run without a shell. One line ending, `\n` or `\r\n`, may follow it. A file that can't
be read, is empty, holds a second line, or can't be split is `bad-command`, and so is a checks or
mutation command whose program can't be started; an adapter's is `conform`'s `adapter-failed`.
The command runs from the root, with `./verify`'s environment, with the `tools/` next to
`verify` first on `PATH`, so that a line can name a helper such as `adapter_shim.py` or
`run_checks.py` alone, and with `VERIFY_SPINE`, the command line that runs the toolkit's
`spine` (absolute paths, quoted by shell rules). It sets nothing for any language: a Python
line puts `src/` on the import path itself, for example
`env PYTHONPATH=src adapter_shim.py conformance/adapter_notes.py`, and finds Python on `PATH`.
A content-check command passes when it exits 0, which must mean that at least one check
ran and every one passed: a command that finds no checks must fail. A mutation command passes
when it exits 0, which must mean that at least one mutant ran and every one was caught; it runs
the oracle on each mutant itself, through `VERIFY_SPINE`. Either one's output is shown when it
fails. With `VERIFY_BASE` set it
first runs `guard` over the diff, once, with the `policy.yaml` next to `verify`, whatever the
roots are. Output is one `ok`, `FAIL` or `skip` line per step, with error lines under failing
steps, and a final `--` summary line. A root other than the folder `verify` is in starts with a
`== <root>` line, and its failed steps are named `<root>: <step>` in the summary. JUnit
report names then start with the root, slashes turned into dashes. Environment:
`VERIFY_BASE`, `VERIFY_BRANCH`, `VERIFY_REPORTS`, `VERIFY_VERBOSE`. Exit codes as above; more
than one ROOT, or a ROOT that is not a folder, is a usage error.

Every step that does not run prints `skip  <step> (<reason>)`: the guard without `VERIFY_BASE`,
behaviors without `spec/<machine>/behaviors/`, `stale` without `checks/<machine>/`, mutation
without `--full`, and (in one line) a machine's remaining steps when its spine is not sound. A
missing piece, or one given wrongly, is an error instead, reported per root:

| Code | Severity | From | Meaning |
|---|---|---|---|
| `no-machines` | error | `./verify` | no `spec/*/spine.yaml` at all, including an `examples/*/` folder without `spec/` |
| `no-adapter` | error | `./verify` | a machine has no `conformance/adapter_<machine>.cmd` |
| `no-checks` | error | `./verify`; `run_checks.py --require-checks` | from `./verify`: no `checks/run.cmd`; from `run_checks.py` with the flag: no `test_*` functions under the folder, which may be missing |
| `no-mutation` | error | `./verify --full` | no `mutation/mutate_*.cmd` |
| `bad-command` | error | `./verify` | a `.cmd` file can't be read, is empty, holds more than one line, or can't be split; or a checks or mutation command's program can't be started |

`ambiguous-piece`, for a piece given both as a Python file and as a command, is retired: with
one form per piece it can't arise (#69). It is never reused.

`tools/run_checks.py` follows the same exit codes and line format.
`run_checks.py` reports `check-failed` (error) for a `test_*` function that raised, with the
exception's last line, and `check-unloadable` (error) for a `test_*.py` file it can't import,
then runs the other files.
Without `--require-checks`, `run_checks.py` passes on zero checks, so that a mutation script,
which counts a failing run as a caught mutant, is not fooled by an empty folder. A Python
example's `checks/run.cmd` passes the flag: `env PYTHONPATH=src run_checks.py checks --require-checks`.

## Known gaps

- No SARIF output yet.
- An adapter command that never replies makes `conform` wait; there is no timeout.
- `guard` trusts the branch name; protect branch patterns in the forge.
