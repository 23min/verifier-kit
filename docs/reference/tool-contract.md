# Tool contract

Every tool in this repo follows the rules below. They are what let the tools plug into any
agent harness, git hook or CI system without knowing about it. Changing anything on this page
needs an issue and the owner's approval first (see `CLAUDE.md`).

## Rules for every tool

- **Input is files in the repo.** No network access, no model calls. The one planned
  exception is `interpret`, whose output is files that `spine diff` then judges.
- **No knowledge of callers.** A tool never calls aiwf, a CI API, an agent or another tool's
  internals. Callers call tools; tools call nothing.
- **Same input, same output.** Results are sorted and carry no timestamps or random ids.
- **Exit codes:** `0` no errors, `1` at least one finding at error severity, `2` usage error.
- **Output:** text by default; `--format json` for machines; `--junit FILE` additionally
  writes a JUnit XML report (one test case per error, or one passing case).
- **Dependencies:** the Python standard library plus PyYAML (`requirements.txt`).
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

## `tools/spine.py`

| Command | Arguments | Does |
|---|---|---|
| `check` | `SPEC` | Validates the file, then: every state, event and variable combination has exactly one rule; every state is reachable; terminal states have no exits; worked examples replay to their expected end |
| `diff` | `A B [--depth N]` | Compares two spines case by case, then by behavior over every sequence up to N events (default 4), reporting the shortest witness for each difference |
| `hash` | `SPEC` | Prints one content hash per state/event cell: `state/event@hash` |
| `stale` | `SPEC CHECKS_DIR` | Finds `covers: state/event@hash` citations in files under `CHECKS_DIR` and reports stale, orphaned and uncovered cells |
| `behaviors` | `SPEC DIR` | Replays every file in `DIR/valid/` (must be allowed) and `DIR/invalid/` (must be refused) |
| `trace` | `SPEC FILE` | Replays one behavior file; for callers that run a validator once per fixture |
| `conform` | `SPEC --adapter FILE [--depth N]` | Runs every event sequence up to N steps (default 4) against the spine and against the code through the adapter, and reports where they differ |
| `guard` | `POLICY --branch NAME [FILES...]` | Maps the branch to a role and reports changed files that role may not touch; reads files from stdin when none are given |

All commands accept `--format text|json` and `--junit FILE`. Every command that takes a spine
validates it first and stops with the validation findings if it is invalid.

### Finding codes

| Code | Severity | From | Meaning |
|---|---|---|---|
| `spec-unreadable` | error | all | the file can't be read or parsed as YAML |
| `spec-shape` | error | all | a section has the wrong type |
| `missing-key` | error | all | a required key is missing |
| `unknown-key` | error | all | a key outside the format |
| `yaml-boolean-key` | error | all | a key YAML read as a boolean (`on`, `yes`, `y`) |
| `yaml-boolean-value` | error | all | a value YAML read as a boolean where a name was meant |
| `bad-value` | error | all | a value outside its closed set, such as a state kind |
| `bad-outcome` | error | all | a rule without exactly one outcome |
| `bad-condition` | error | all | a condition outside the condition language |
| `unknown-state`, `unknown-event` | error | `check` | a rule or `initial` names something undeclared |
| `undecided-cell` | error | `check` | no rule at all for a state and event |
| `missing-case` | error | `check` | no rule matches some variable values |
| `overlap` | error | `check` | two rules with different outcomes match the same case |
| `terminal-has-exit` | error | `check` | a terminal state has a transition out |
| `unreachable` | error | `check` | no event sequence reaches a state |
| `example-fails` | error | `check` | a worked example ends somewhere else |
| `only-in-a`, `only-in-b` | error | `diff` | a state or event exists in one reading only |
| `vars-differ` | error | `diff` | the readings declare different variables |
| `case-disagrees` | error | `diff` | the readings decide a case differently |
| `behavior-differs` | error | `diff` | a shortest event sequence on which the readings end differently |
| `stale-check` | error | `stale` | a check cites a cell whose hash has changed |
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
| `path-denied` | error | `guard` | a deny pattern for the role matches a changed file |
| `path-not-allowed` | error | `guard` | no allow pattern for the role matches a changed file |
| `unknown-role` | error | `guard` | no branch pattern matches |
| `role` | info | `guard` | which role the branch mapped to |

## The adapter

A Python file with three functions, owned by the oracle side:

- `new()` returns a handle to a fresh instance of the system under test, in the spine's initial state.
- `apply(handle, event, env)` performs one spine event, with `env` holding the variable values
  (for example `{"role": "editor"}`), and returns `"ok"` or `"rejected"`. Any exception counts as a divergence.
- `observe(handle)` returns the name of the spine state the system is in, read from its real data.

The adapter translates and never decides whether an action is allowed.

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
`fnmatch`, where `*` also matches `/`.

## `./verify`

`./verify [ROOT] [--full]`. A root is a folder laid out like a project, with `spec/`,
`checks/`, `conformance/`, `src/` and `mutation/`; every path below is relative to it. A
relative ROOT is taken from the caller's working directory. Without ROOT, the roots are every
`examples/*/` folder in turn (sorted) when the folder `verify` is in has no `spec/` but has
`examples/*/`; otherwise the root is that folder itself.

Runs, per `spec/<machine>/spine.yaml`: `check`, then `behaviors` over `spec/<machine>/behaviors`,
`stale` over `checks/<machine>`, and `conform` with `conformance/adapter_<machine>.py`, with
`src/` and `conformance/` on `PYTHONPATH`. Then the content checks under `checks/`, with `src/`
on `PYTHONPATH`, and with `--full` every `mutation/mutate_*.py`. With `VERIFY_BASE` set it
first runs `guard` over the diff, once, with the `policy.yaml` next to `verify`, whatever the
roots are. Output is one `ok`, `FAIL` or `skip` line per step, with error lines under failing
steps, and a final `--` summary line. A root other than the folder `verify` is in starts with a
`== <root>` line, and its failed steps are named `<root>: <step>` in the summary. JUnit
report names then start with the root, slashes turned into dashes. Environment:
`VERIFY_BASE`, `VERIFY_BRANCH`, `VERIFY_REPORTS`, `VERIFY_VERBOSE`, `PYTHON`. Exit codes as
above; more than one ROOT, or a ROOT that is not a folder, is a usage error.

Every step that does not run prints `skip  <step> (<reason>)`: the guard without `VERIFY_BASE`,
behaviors without `spec/<machine>/behaviors/`, `stale` without `checks/<machine>/`, mutation
without `--full`, and (in one line) a machine's remaining steps when its spine is not sound. A
missing piece is an error instead, reported per root:

| Code | Severity | From | Meaning |
|---|---|---|---|
| `no-machines` | error | `./verify` | no `spec/*/spine.yaml` at all, including an `examples/*/` folder without `spec/` |
| `no-adapter` | error | `./verify` | a machine has no `conformance/adapter_<machine>.py` |
| `no-checks` | error | `run_checks.py --require-checks` | no `test_*` functions under the folder, which may be missing; `./verify` passes the flag |
| `no-mutation` | error | `./verify --full` | no `mutation/mutate_*.py` |

`tools/run_checks.py` and `mutation/mutate_*.py` follow the same exit codes and line format.
Without `--require-checks`, `run_checks.py` passes on zero checks, so that the mutation script,
which counts a failing run as a caught mutant, is not fooled by an empty folder.

## Known gaps

- A worked example or behavior file missing `steps` or `expect`, or a step without `=` in a
  variable, crashes instead of producing a finding.
- No SARIF output yet.
- Adapters are Python and run in-process; other languages need a process protocol.
- `guard` trusts the branch name; protect branch patterns in the forge.
