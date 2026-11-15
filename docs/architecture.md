# Architecture

How the repo fits together today: who calls what, what `./verify` runs, how the examples are
wired, and how the toolkit checks itself. The rules each piece follows are in
`docs/reference/`; the reasons are in `docs/design.md`. In the diagrams, `{m}` is a machine's
folder name, such as `notes`, and `{root}` is the folder `./verify` checks: a project laid out
with `spec/`, `checks/`, `conformance/`, `src/` and `mutation/`. In this repo each
`examples/<name>/` is a root; a project that uses the toolkit is its own root.

## The repo

```
verify                           the single entry point: builds spine and verify, then runs verify
install.sh                       installs the toolkit into a project, or updates it
tools/cmd/, tools/internal/      spine and verify, in Go, and their tests (no knowledge of any CI or agent)
tools/run_checks.py              runs Python content checks; a Python project's checks/run.cmd calls it
tools/adapter_shim.py            serves a Python adapter over the adapter protocol; an adapter line calls it
policy.yaml                      which branch role may change which paths
examples/<name>/                 one worked example each, laid out like a project (examples/README.md)
demo/                            the toolkit's demos on the notes example: faults.sh, live-builder.sh
integrations/                    CI, agent and git hook files, to copy when you want them
docs/                            introduction, guide, comparison, design (normative), architecture,
                                 decisions (adr/), reference, proposals (research/), benchmarks
.github/workflows/verify.yml     this repo's own CI: go vet, both go tests, ./verify --full, demo/faults.sh
```

## Layers

Callers know about `./verify`. `./verify` knows the folder layout. The tools know only the
files they are given.

![Layers. Five callers run ./verify: the Claude Code Stop hook
(integrations/claude-code/hooks/stop-verify.sh) for the fast checks, git pre-commit
(integrations/git-hooks/pre-commit), the CI job (.github/workflows/verify.yml) with --full,
demo/faults.sh on a broken copy, and a person. ./verify, in Bash, builds spine and verify, then runs
tools/bin/verify with the same arguments. verify finds the pieces by folder name and runs the steps:
tools/bin/spine (check, behaviors, stale, conform, draw, guard); tools/run_checks.py, from a Python
root's checks/run.cmd; and under --full each mutant in mutation/mutants.yaml, in a copy, with spine
conform and the checks run there. spine's conform runs a Python root's adapter line through
tools/adapter_shim.py, a Python adapter over JSON lines. spine works on policy.yaml and on each
root's oracle (spec/, checks/, conformance/) and code (src/); run_checks.py on the oracle and the
code. Colours show who owns what: ./verify, verify, spine, run_checks.py and adapter_shim.py are
green, the toolkit's programs; policy.yaml, the oracle and the mutants are purple, yours: the
builder can't change it; the code is pink, the builder's.](architecture/1-layers.svg)

## What `./verify` runs

![What ./verify runs. It builds tools/bin/spine and tools/bin/verify with go -C tools build, and
without Go fails with exit 1. The roots are the folder given as ROOT; without one, this folder if it
has spec/ or there is no examples/\*/; otherwise every examples/\*/, sorted. If VERIFY_BASE is set,
spine guard checks the files changed since VERIFY_BASE against policy.yaml, once, whatever the
roots. Then, once per root: for each {root}/spec/{m}/spine.yaml, spine check, and only if the spine
is sound, spine behaviors on spec/{m}/behaviors/, spine stale on checks/{m}/, spine conform with the
line in conformance/adapter_{m}.cmd, and spine draw with spec/{m}/drawing.yaml, its output compared
with spec/{m}/drawing.svg; then the content checks, the line in {root}/checks/run.cmd; then, under
--full, each mutant in {root}/mutation/mutants.yaml, in a copy, and the line in every mutate_\*.cmd.
One summary line ends the run, with exit 0 or 1. Colours show who owns what: the steps the toolkit
runs are green, the toolkit's programs; the content checks, the root's own command line, are purple,
yours: the builder can't change it; the decisions, roots and skips are
white.](architecture/2-what-verify-runs.svg)

Each adapter and the content checks are a `.cmd` file holding one command line, in any
language (`docs/adr/0007-no-language-built-in.md`), and so is a mutation tool's command,
`mutation/mutate_<name>.cmd`. How `./verify` runs them, and what it gives them, is in the
contract's section on `./verify` (`docs/reference/tool-contract.md`).

`./verify` is a short Bash script. Before anything else it builds `spine` and `verify` with Go,
into the `bin/` of the toolkit's folder next to it: `tools/` here, and `.verifier-kit/` in a
project, which gets both from `install.sh`. Then it runs `verify` with its arguments, and
`verify` does all of the above (`tools/cmd/verify/`, `tools/internal/verify/`). Its decisions are
pure functions, in `plan.go`: reading the arguments, planning each root's steps from what the
root holds, and what each step prints. `verify.go` reads the files and starts the programs around
them, and decides nothing (`docs/adr/0007-no-language-built-in.md`, decision 1). `verify` runs
`spine` only as a program. The two share what both need: `tools/internal/shlex`, which splits a command line, and
`tools/internal/program`, which starts one and words the system's errors.

`spine` (`tools/cmd/spine/`, `tools/internal/spine/`) is split into a core and a shell too, by
files (#94). Four files are the shell: `files.go` reads the files, `adapter.go` starts the
adapter and carries its lines, `cli.go` reads the arguments and runs the command, and `output.go`
prints. The others are the core: they validate what was read, check the spine, judge citations,
behaviors and paths, and run `conform`'s search. The search is a step function: given the
adapter's reply to its last request, it gives the next request, or the findings when it is done.

A step that cannot run because a piece is missing is an error, not a skip, reported per root:
no spine at all (`no-machines`), no adapter for a machine (`no-adapter`), no `checks/run.cmd`
(`no-checks`), or no mutants under `--full` (`no-mutation`). So is a `.cmd` file that
holds no single command line, or whose program can't be started (`bad-command`). A root other than the repo
itself starts with a `== <root>` line.

## The notes example

The first example, in `examples/notes/`; the paths below are relative to it. Its files
are tied together by the name `notes`, by Python imports, and by the cell hashes the checks
cite.

![The notes example's files. Two interpreters read spec/notes/chapter.md, the prose, into
spec/notes/readings/\*.yaml; spine diff compares the two readings and a person ratifies
spec/notes/spine.yaml. spine behaviors replays spec/notes/behaviors/, valid/ and invalid/, against
the spine. spine conform tries every case, with margin 1, through conformance/adapter_notes.py (new,
apply, observe), which imports and calls src/notes_app.py, the service. checks/notes/test_content.py
imports the service too, and cites the spine's cells as covers: state/event@hash, drawn dashed.
verify makes each of the five edits in mutation/mutants.yaml to the service, in a copy. Colours show
who owns what: the service, src/notes_app.py, is pink, the builder's; every other file is purple,
yours: the builder can't change it.](architecture/3-notes.svg)

Who may change what, from `policy.yaml` (enforced by `spine guard` when `VERIFY_BASE` is set):

| Branch | Role | May change | May not change |
|---|---|---|---|
| `build/*` | builder | `examples/*/src/`, `docs/`, `README.md` | `examples/*/spec/`, `examples/*/checks/`, `examples/*/conformance/`, `examples/*/mutation/`, `tools/`, `integrations/`, `verify`, `policy.yaml`, `.claude/`, `.github/`, `.gitlab/`, CODEOWNERS |
| `spec/*` | spec-author | `examples/*/spec/`, `examples/*/checks/`, `examples/*/conformance/` | `examples/*/src/` |
| anything else | person | everything | |

## The ref_sync example

In `examples/ref_sync/`; the paths below are relative to it. Its spine is one operation's
lifecycle, which `spine conform` checks through the client's pure `transition()`. Two clients
racing, lost replies and crashes are checked by its own explorer, and Git's assumed behavior by
checks against real Git. The adapter's docstring is the whole contract between the oracle and
`src/`.

![The ref_sync example's files. Two interpreters read spec/ref_sync/chapter.md (rules S1-S7,
C01-C12) into spec/ref_sync/readings/\*.yaml; spine diff compares them and a person ratifies
spec/ref_sync/spine.yaml, one operation's lifecycle. spine behaviors replays
spec/ref_sync/behaviors/ against it, and spine conform tries every case, with margin 1, through
conformance/adapter_ref_sync.py (new, apply, observe), which calls transition in
src/ref_sync_client.py (transition, Client, GitRemote). checks/ref_sync/test_protocol.py (C01-C08)
uses explore.py, a breadth-first search, and world.py, the simulated remote with its faults and
S1-S5 and S7. world.py reads the spine's states, steps two Clients against the simulated remote, and
with test_protocol.py cites the whole spine as covers: spine@hash, drawn dashed.
checks/ref_sync/test_git_ref.py (C09-C11, S6) publishes with GitRemote and uses the git program on
throwaway repositories, which GitRemote runs too. verify makes each of the six edits in
mutation/mutants.yaml to the client, in a copy. Colours show who owns what: the client,
src/ref_sync_client.py, is pink, the builder's; the git program is white; every other file is
purple, yours: the builder can't change it.](architecture/4-ref-sync.svg)

The explorer is generic: `explore.py` knows nothing of Git, and `world.py` holds the moves, the
faults and the invariants. `git` is a dependency of this example only; without it the Git
checks fail.

## The elixir_job example

In `examples/elixir_job/`; the paths below are relative to it. It is in Elixir; its adapter and
content checks are `.cmd` files, each holding one command line that `./verify`
runs from this folder. Every command loads `src/**/*.ex` with `elixir -pr`; there is no Mix
project. The spine is one job's lifecycle, which `spine conform` checks through the job's pure
`Job.Lifecycle.transition/3`, taking `attempts` as given; the content checks drive `Job`
itself, which counts its own attempts.

![The elixir_job example's files. Two interpreters read spec/job/chapter.md (rules J1-J2, J01-J07)
into spec/job/readings/\*.yaml; spine diff compares them and a person ratifies spec/job/spine.yaml,
one job's lifecycle. spine behaviors replays spec/job/behaviors/ against it, and spine conform tries
every case, with margin 1, through conformance/adapter_job.exs, which speaks the adapter protocol
from adapter_job.cmd and calls Job.Lifecycle.transition/3 in src/job.ex. checks/job/job_test.exs,
ExUnit run from checks/run.cmd, checks J01-J07, and J1 and J2 in every reachable job. verify makes
each of the five edits in mutation/mutants.yaml to src/job.ex, in a copy. Colours show who owns
what: src/job.ex is pink, the builder's; every other file is purple, yours: the builder can't change
it.](architecture/5-elixir-job.svg)

## The oban_job example

In `examples/oban_job/`; the paths below are relative to it. It is in Elixir, as elixir_job is,
but its code under test uses a library: `src/mailer.ex` sends each email from an Oban 2.24.2
job, on SQLite, and `spine conform` drives the app and Oban together. Every command loads
`src/deps.exs` first, which installs Oban's Hex packages with `Mix.install`, pinned by
`src/mix.lock` and kept per machine outside the repo, with `HEX_OFFLINE=1`; then `src/**/*.ex`.
The adapter runs one Oban, in `testing: :manual` on an in-memory database, for the whole run: a
`run` is `Oban.drain_queue/2`, a `wait` the same with `with_scheduled: true`, and `observe`
reads the job's row back as its state and attempt. Its spine declares an output (#116), what a
step sends to another system: `send_mail`, the worker's call to the mail server. The adapter's
fake mail server counts its calls, and each reply to `apply` reports one `send_mail` for each call
made during the step, which `conform` compares with the rule's.

![The oban_job example's files. Two interpreters read spec/delivery/chapter.md, written from Oban's
docs (D1-D6, D01-D14), into spec/delivery/readings/\*.yaml; spine diff compares them and a person
ratifies spec/delivery/spine.yaml, one email's job. spine behaviors replays spec/delivery/behaviors/
against it, and spine conform tries every case, with margin 1, through
conformance/adapter_delivery.exs, which speaks the adapter protocol from adapter_delivery.cmd. The
adapter sends, cancels and drains the queue through src/mailer.ex (Mailer, Mailer.Worker), which
runs on Oban 2.24.2, on SQLite in memory; it counts the mail server's calls, and reads the job's
state and attempt from Oban.
checks/delivery/delivery_test.exs, ExUnit run from checks/run.cmd, checks backoff, snooze,
max_attempts and the address. src/deps.exs and src/mix.lock, Mix.install offline, are loaded first
by every command, drawn dashed. verify makes each of the eight edits in mutation/mutants.yaml to the
mailer, in a copy. Colours show who owns what: src/mailer.ex, src/deps.exs and src/mix.lock are
pink, the builder's; Oban is white; every other file is purple, yours: the builder can't change
it.](architecture/6-oban-job.svg)

## Checking the checker

`./verify --full` makes each example's mutants, listed in `mutation/mutants.yaml`, each one edit
to a file under `src/` (#58). For each root it runs the steps that read `src/`, each machine's
`conform` and the content checks, first on the code as it is and then on each mutant, from one
temporary copy of the whole root but its `.git`, made once per run (#113), whose `src/` is a
fresh copy of the root's, with the mutant's edit made, before each run. If a
step fails on the code as it is, it reports `oracle-fails` and makes no mutant, because an
oracle that already fails would count every mutant as caught. A mutant is killed when any step
exits other than 0, survives when all pass, and is stale when its text isn't in its file exactly
once. This is `verify`'s own code, in every language alike (`mutants.go` for the decisions,
`verify.go` for the copies); a project with a mutation tool gives its command as
`mutation/mutate_<name>.cmd`, which runs as a step of its own.

![How ./verify --full checks the checker. verify copies the root, but for its .git, into a
temporary copy, once, and runs spine conform, per machine, and checks/run.cmd from it; unless every
step exits 0, it reports oracle-fails and makes no mutant. Then, for each mutant in
mutation/mutants.yaml, it gives the copy a fresh src/ with the mutant's one edit and runs the same
steps from it; the mutant is killed if any step exits other than 0. The step fails if any mutant
survived or is stale. Colours show who owns what: verify and spine conform are green, the toolkit's
programs; checks/run.cmd is purple, yours: the builder can't change it; the temporary copy is
white.](architecture/7-checking-the-checker.svg)

## Acceptance: `demo/faults.sh` and CI

`demo/faults.sh` is the toolkit's own acceptance test. Each scenario makes a fresh copy of the
repo (without `.git`) in a temp folder, seeds one fault, runs a command there, and checks the
exit code and the lines it must print. Scenarios 0 to 9 seed faults in the notes example, and
`./verify` in the copy checks every example, so ref_sync's, elixir_job's and oban_job's checks run
in each of them too. Scenarios 11 to 13 seed faults in elixir_job and check it alone. Scenario
14 breaks the code of every example, so that each example's mutants stop at `oracle-fails`.
Scenarios 15 to 17 run the Stop hook and read its reply as JSON, as Claude Code does: on a
machine without `python3`, on failing lines that hold characters JSON must escape, and with a
project folder that doesn't exist.

![One demo/faults.sh scenario. The working copy is copied with tar, without .git, into a
temporary copy; one fault is seeded there; ./verify, spine diff or the Stop hook runs on it. If
its exit code and lines are the expected ones, the scenario is ok; otherwise it fails and
faults.sh exits 1.](architecture/8-faults.svg)

| # | Fault seeded | Command | Must print |
|---|---|---|---|
| 0 | none | `./verify --full` | `all steps passed` |
| 1 | the service as first written | `./verify` | two `diverges` lines with their sequences |
| 2 | the notes spine lets editors restore | `./verify` | `behavior-allowed`, `stale-check`, `diverges` |
| 3 | `on:` instead of `event:` in the notes spine | `./verify` | `yaml-boolean-key`, and the notes steps skipped |
| 4 | a `build/*` branch edits the notes spine | `VERIFY_BASE=main ./verify` | `path-denied examples/notes/spec/notes/spine.yaml` |
| 5 | none (two readings) | `spine diff` on the notes readings | two `case-disagrees` lines |
| 6 | notes content checks replaced by one that never looks at the text | `./verify --full` | `mutant-survived` for the upper-case mutant |
| 7 | the service as first written | the Stop hook, twice | a block, then the attempt-cap warning |
| 8 | the notes adapter, content checks and mutants removed | `./verify --full` | `no-adapter`, `no-checks`, `no-mutation` |
| 9 | the notes service imports a module that doesn't exist | `./verify examples/notes` | `adapter-failed`, `check-unloadable` |
| 10 | unused since #69, when notes' adapter became a command line: scenario 1 runs it through the adapter protocol | | |
| 11 | the job retries after its last attempt fails | `./verify examples/elixir_job` | `diverges running/fail attempts=last` |
| 12 | the job counts one attempt too many | `./verify examples/elixir_job` | conformance `ok`, content checks `FAIL` with J03 |
| 13 | elixir_job's checks that count attempts deleted (J03, J04 and every reachable job) | `./verify examples/elixir_job --full` | content checks `ok`, and `mutant-survived` for a counting mutant |
| 14 | the code of all four examples, as in scenarios 1 and 11, ref_sync's first mutant and oban_job's first | `./verify --full` | `oracle-fails` for each example's mutants |
| 15 | no `python3` on the PATH, and no session id | the Stop hook, twice | a block with the failing lines, then the attempt-cap warning, each saying every session shares the count |
| 16 | `./verify` prints failing lines with quotes, backslashes, control characters and a byte that isn't UTF-8 | the Stop hook | a block whose failing lines are exactly those |
| 17 | the project folder given to the Stop hook doesn't exist | the Stop hook | a block saying `./verify` did not run |

This repo's CI (`.github/workflows/verify.yml`) runs `go vet` and `go test` in `tools/`, then
`go test` in `examples/`, which checks the conventions every example follows
(`examples/README.md`), then `./verify --full`, then `demo/faults.sh`, on every pull request and
push to `main`. It does
not set `VERIFY_BASE`, so the path guard runs here only inside scenario 4.

`go test` holds the cases test (`tools/internal/spine/cases_test.go`). It runs the Go `spine`
(`tools/cmd/spine/`, `tools/internal/spine/`, one module in `tools/`) on every input it has,
among them the repo's spines, readings, behaviors, checks, adapters and policy and the faults
scenarios 1 to 4, 9 and 10 seed. Between them they give every finding code the contract lists.

Each run must give exactly what the record holds (`tools/internal/spine/testdata/record/`,
written by `record_test.go` beside it, in the format `tools/internal/record` reads and writes):
its exit code and output, the message on standard error of a usage error, and in JSON runs the
JUnit report, with the paths that differ between machines written as `$ROOT` and `$TMP` (#44).
`go -C tools test ./internal/spine/ -update` writes the record again; its diff is what a
reviewer reads. Beside it, `conform_test.go` drives `conform`'s search with a fake adapter in Go,
with no program, and `core_test.go` checks what `spine`'s core imports.

`go test` also holds the record of `./verify` (`tools/internal/verify/record_test.go`, #68).
It installs the toolkit into fixture projects with `install.sh --from`, as a project does, and
runs `./verify` in them as its callers do. Between them the fixtures reach every path through it: misuse, a
failed build, which roots run, the guard, every skip, each piece as a command line, passing and
failing, each missing piece, each `bad-command`, and a root that gives its pieces only by
Python file names. Each run must give exactly what
`tools/internal/verify/testdata/record/` holds: its exit code, standard output, standard error
and every JUnit report, with the test's temporary folder written as `$TMP`. Every run is in the
C locale. The fixtures need no Elixir.
`go -C tools test ./internal/verify/ -update` writes the record again. It was written from the
Bash `./verify`, and is what the change to commands (#69) and the Go `verify` (#70) were checked
against. Beside it, `plan_test.go` tests each of `verify`'s decisions without starting a
program.

`go test` also runs the benchmarks (`tools/internal/benchmarks/`, #106): `spine conform` against
four Mealy machines from the Automata Wiki, with faults planted in them, each verdict checked
against what ADR 0008's argument promises, computed by code that shares none of `spine`'s. The
test binary is the adapter. Plain `go test` runs one mutant of each class, model and translation;
`-full` runs them all (`docs/benchmarks.md`).

## Per machine, per root and repo-wide

| Part | Scope today | How it finds its inputs |
|---|---|---|
| `spine check`, `behaviors`, `stale`, `conform` | per machine | `{root}/spec/{m}/`, `{root}/checks/{m}/`, `{root}/conformance/adapter_{m}.cmd` |
| content checks | per root: one run over all of `{root}/checks/` | `{root}/checks/run.cmd` |
| mutation | per root, under `--full`: `{root}/mutation/mutants.yaml`, and every `{root}/mutation/mutate_*.cmd` | the file's name, and a file name pattern; each mutant runs every machine's `conform` and all of `checks/`, in a copy |
| `src/` | per root: loaded by the commands | module name (`notes_app`, `ref_sync_client`), with `src/` on `PYTHONPATH` from the line; elixir_job's and oban_job's commands load `src/**/*.ex`, oban_job's after `src/deps.exs` |
| path guard | repo-wide | `policy.yaml` patterns over `examples/*/` and the toolkit's folders |
| deny rules, CODEOWNERS (`integrations/`) | a project's root | folder patterns for a project with `spec/` at its root |
| `demo/faults.sh` | seeds faults in notes (0-9), elixir_job (11-13) and all four examples (14) | hard-coded `examples/notes/`, `examples/elixir_job/`, `examples/ref_sync/` and `examples/oban_job/` paths |
| `demo/live-builder.sh` | notes only | hard-coded `examples/notes/` paths |
| choosing what to run | one root per call | `./verify ROOT`; without it, the discovery rule above |
