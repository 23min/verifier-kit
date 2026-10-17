# verifier-kit

A small, standalone toolkit for keeping an AI builder honest: people own an
oracle (a state-machine "spine" plus checks), the builder owns only the code, and one script,
`./verify`, decides whether the code matches the oracle. Every gate (agent hook, git hook,
CI) calls that script and nothing else.

"Oracle" here is the software-testing term: a *test oracle* is whatever decides whether an
output is correct.

It is a prototype for learning the process on something small. Each worked example lives in
`examples/<name>/`, laid out like a project that uses the toolkit. The first,
[`examples/notes/`](examples/notes/README.md), is a notes service with drafts, publishing, trash
and roles; it doubles as the toolkit's acceptance test. The second,
[`examples/ref_sync/`](examples/ref_sync/README.md), is a protocol: two clients publishing
workflow events to one Git ref, checked through every interleaving of their steps, lost
replies and crashes up to a bound, and against real Git. The third,
[`examples/elixir_job/`](examples/elixir_job/README.md), is a background job with retries,
written in Elixir: its adapter, content checks and mutation are commands, so it shows the
toolkit checking a project in another language.

The design is in [`docs/design.md`](docs/design.md); the exact tool contract and spine format
are in [`docs/reference/`](docs/reference/). Rules for agents working here are in
[`CLAUDE.md`](CLAUDE.md). Work is tracked in GitHub issues.

## Try it

```sh
pip install -r requirements.txt   # PyYAML, the only Python dependency; or: uv venv && uv pip install -r requirements.txt
./verify                    # fast checks on every example, a few seconds
./verify --full             # also breaks the code on purpose to check the checks
./verify examples/notes     # one example only
./verify examples/ref_sync  # the protocol example; it also needs the git program
./verify examples/elixir_job  # the Elixir example; it also needs Elixir 1.18 or later
demo/faults.sh              # acceptance test: seeds one fault at a time and checks the right part catches it
demo/live-builder.sh        # optional: a real Claude Code builder session against the oracle
```

`spine` is being ported to Go, so that checking a spine needs neither Python nor PyYAML
([`docs/adr/0004-go-spine.md`](docs/adr/0004-go-spine.md)). The Go `spine` has every command;
`./verify` still runs `tools/spine.py`. With Go 1.27:

```sh
go build -o bin/spine ./cmd/spine                    # bin/ is ignored by git
bin/spine check examples/notes/spec/notes/spine.yaml  # same commands, options and output as tools/spine.py
go test ./...                                        # runs both on the same inputs; any difference fails
```

Or open the repo in its devcontainer (`.devcontainer/`, "Dev Containers: Reopen in Container"),
which has Python 3 with PyYAML, Go, uv, gh and Claude Code. It needs Docker and VS Code with the Dev
Containers extension, on macOS or Linux (Windows is untested; open the repo from WSL). Before
each start it mounts these folders from your home into the container, read-write, and creates
any that are missing: `~/.claude`, `~/.claude-linux/plugins`, `~/.codex-linux`, `~/.config/gh`,
`~/.guidance`. Whatever is in them, such as your Claude Code or gh login, is visible inside the
container. None of them are needed for `./verify`; sign in to gh or Claude Code inside the
container only if you want those tools there. Each start also prints a note about personal
guidance (`~/.guidance`): those are the author's own rules for AI assistants, public at
[23min/guidance](https://github.com/23min/guidance) for reference. Don't install them; they
would apply to every project on your machine. If you'd rather not share these folders, use a
local Python.

## Layout and who owns what

```
verify                           the single entry point
tools/spine.py, run_checks.py    the tools (no knowledge of any CI or agent)
cmd/spine/, internal/spine/      the Go port of spine.py, and its test against spine.py
tools/adapter_shim.py            serves a Python adapter over the adapter protocol
policy.yaml                      which branch role may change which paths
examples/<name>/                 one worked example each, laid out like a project (below)
demo/                            the toolkit's demos on the notes example: faults.sh, live-builder.sh
integrations/                    CI, agent and git hook files, to copy when you want them
docs/                            design (normative), architecture, decisions (adr/), reference
.github/workflows/verify.yml     this repo's own CI: go vet, go test, ./verify --full, demo/faults.sh
```

Inside an example, for instance `examples/notes/`:

```
spec/notes/chapter.md            the prose chapter you write with an LLM            person
spec/notes/readings/*.yaml       two independent readings of the chapter            interpreters
spec/notes/spine.yaml            the ratified state machine: the oracle's spine     person
spec/notes/behaviors/            example runs: valid/ must be allowed, invalid/ refused
checks/notes/                    content checks the spine cannot see; each cites a spine cell
conformance/adapter_notes.py     translates spine events into calls on the service
mutation/mutate_notes.py         deliberate bugs the oracle must catch
src/notes_app.py                 the service                                        builder
demo/notes_app_as_written.py     the service as first written (two bugs)
README.md                        what the example shows and how to run it
```

A project that uses the toolkit has the same folders at its own root, next to `verify` and
`tools/`. In a project not written in Python, the adapter, the content checks and each mutation
script are `.cmd` files instead, each holding one command line that `./verify` runs from the
root: `conformance/adapter_notes.cmd`, `checks/run.cmd`, `mutation/mutate_notes.cmd`.

## The three loops

1. **Intent.** You and an LLM write the chapter (`chapter.md`): what to build, what not to
   build, the behavior. It can be long and it will have gaps.
2. **Interpretation.** Two agents, ideally on different models, each turn the chapter into a
   spine without seeing the other's. `spine diff` compares the two readings and turns every
   disagreement into a question with a concrete witness ("after publish, trash, restore by an
   editor: A says rejected, B says active"). You answer, and the answers become the ratified
   `spine.yaml`. This loop finds ambiguity; it is not a gate. Two readings can share a
   misreading, so a clean diff is evidence, not proof.
3. **Build.** The builder changes `src/` until `./verify` passes. It can read the oracle but
   not change it.

## What `./verify` runs

| Step | Command | Catches | Does not catch |
|---|---|---|---|
| guard (when `VERIFY_BASE` is set) | `spine guard` | a role changing paths it does not own | anything about behavior |
| spine is sound | `spine check` | unknown keys, YAML slips (`on:` read as `true`), cases no rule covers, overlapping rules, unreachable states, failing worked examples | a spine that is complete but wrong |
| behaviors match spine | `spine behaviors` | the spine drifting from the example runs you agreed on | |
| checks cite current spine | `spine stale` | content checks that cite a spine cell which has since changed | checks that never cite anything (listed as info) |
| code conforms to spine | `spine conform`, with `adapter_<machine>.py` or the command in `adapter_<machine>.cmd` | any state/event/role case where the code allows, refuses or ends up somewhere other than the spine says, with the shortest sequence that shows it | anything beyond 4 steps; data the adapter does not observe |
| content checks | `run_checks.py`, or the command in `checks/run.cmd` | wrong text, values, formats | whatever no check asserts |
| mutants killed (`--full`) | `mutation/mutate_*.py`, or the command in each `mutate_*.cmd` | an oracle too weak to notice a planted bug | bugs unlike the planted ones |

Nothing is skipped silently. A step that does not run prints `skip` with the reason (no
`VERIFY_BASE`, no behaviors folder, no `--full`, or a spine that is not sound), and a missing
piece is an error: no spine at all, no adapter, no content checks, or no mutation script under
`--full`. So is a piece given both as Python and as a `.cmd` file. Output is one line per step, with error lines only for failing steps. `VERIFY_REPORTS=dir` writes JUnit XML
for the spine steps; `spine ... --format json` gives machine-readable findings. Exit codes:
0 pass, 1 findings, 2 misuse.

## Jacking it in

Nothing here assumes a particular CI, agent or workflow tool. The files in `integrations/`
show how each layer would call `./verify` in a project with `spec/` at its root. They are
examples, linted but not run on a real GitHub or GitLab instance.

| Layer | Example | What it does | Can it be bypassed? |
|---|---|---|---|
| Agent harness | `integrations/claude-code/settings.json` and `hooks/stop-verify.sh` | deny rules keep Claude's file tools out of the oracle; a Stop hook sends Claude back to work while `./verify` fails, with the failing lines as feedback, and gives up after 5 attempts | yes: deny rules do not cover shell commands that write files (use the OS sandbox for that) |
| Git | `integrations/git-hooks/pre-commit` | runs fast checks before each commit | yes: `--no-verify` |
| CI, GitHub | `integrations/ci/github-actions.yml` | runs `./verify --full` and the path guard on pull requests | not when the job is a required check on a protected branch (private repos need a paid plan) |
| CI, GitLab | `integrations/ci/gitlab-ci.yml` | the same, in merge request pipelines, with JUnit in the MR widget | not with "Pipelines must succeed" and a protected branch (both on Free) |
| Review | `integrations/CODEOWNERS` | people approve any change to the oracle | GitLab Code Owner approvals need Premium; on Free the guard step stands in |
| aiwf | `integrations/aiwf/README.md` | aiwf contracts can call `spine` like any other caller | |

Only CI with a required check is a gate; the agent and git layers make failures cheap to
fix early. The path guard maps a branch name to a role (`build/*` is the builder), so it is
only as strong as the rule about who may push which branch names; protect those patterns too.

## A live run

`demo/live-builder.sh` starts Claude Code (Sonnet) as the builder in a scratch project made
from the notes example and the toolkit, with the service as first written and the settings from
`integrations/claude-code/`. The builder is told about one bug
("users can edit notes in the trash") and has no shell, so it cannot run `./verify` itself.
What happened, in the runs we tried:

1. The builder fixed the reported bug and tried to finish.
2. The Stop hook ran `./verify` and blocked:
   `draft/trash role=owner: after trash(role=owner): spine expects rejected in draft, code gave ok in trashed`.
3. The builder read the spine, fixed `trash()`, and tried to finish again.
4. The hook passed; `./verify` is green; the diff is 4 lines in `src/`.

The agent never chose to run the oracle; the hook did. In a separate run, an attempt to
edit `spec/notes/spine.yaml` with the Edit tool was refused by the deny rule. Agent runs
vary from run to run; the checks do not.

## The spine format (excerpt)

```yaml
machine: note
vars:
  role: [owner, editor, viewer]       # every variable has a small, finite set of values
states:
  draft:   {kind: policy}             # fact: true of the world; policy: a business choice;
  trashed: {kind: design}             # design: our own choice, the easiest to change
  purged:  {kind: fact, terminal: true}
initial: draft
events: [publish, edit, trash, restore, purge]
rules:                                # from + event + optional when -> exactly one outcome
  - {from: draft, event: publish, when: "role == 'owner'", to: active}
  - {from: draft, event: publish, when: "role != 'owner'", reject: true}
  - {from: draft, event: edit, when: "role != 'viewer'", stay: true}   # allowed, same state
  - {from: active, event: restore, when: "role == 'owner'", ignore: true}  # no-op, not an error
examples:
  - name: owner trashes and restores
    steps: ["publish role=owner", "trash role=owner", "restore role=owner"]
    expect: active
```

Conditions use a tiny language (declared variables, literals, `==`, `!=`, `in`, `not in`,
`and`, `or`, `not`), so every case can be enumerated. `spine check` requires exactly one
rule for every state, event and variable combination. Every name and value is text: quote
anything YAML would read as something else, such as `"yes"`, `"012"` or `"2026-10-14"`.

A content check cites the cell it depends on with a comment like
`# covers: trashed/restore@95f9c8204e9f`; `spine hash examples/notes/spec/notes/spine.yaml`
prints the current hashes.

## Adding an example or a machine

An example is a folder `examples/<name>/` with the layout above and a `README.md`. Plain
`./verify` picks it up; `./verify examples/<name>` runs it alone. Each example needs at least
one content check and, for `--full`, one mutation script; a missing piece is an error for that
example only.

Within an example (or a project), everything is keyed by folder name, so for a machine called
`invoice`: `spec/invoice/spine.yaml`, optional `spec/invoice/behaviors/{valid,invalid}/`,
`checks/invoice/`, `conformance/adapter_invoice.py` (three functions: `new()`, `apply()`,
`observe()`), and optionally `mutation/mutate_invoice.py`. `./verify` picks them up, and fails
with `no-adapter` until the adapter exists. For code in another language, the adapter is a
program speaking the adapter protocol (`docs/reference/tool-contract.md`), named in
`conformance/adapter_invoice.cmd`, and the content checks and mutation are commands too.

## Limits

- **Bounded, not proven.** Conformance replays every sequence up to 4 steps. That finds most
  logic slips in small machines (the small-scope hypothesis), but it is testing, not proof.
- **Concurrency needs checks of its own.** `spine conform` runs one lifecycle at a time. The
  ref_sync example adds an explorer for two clients in its own `checks/`; the toolkit has no
  general one.
- **The spine sees states and permissions, not content.** Text, totals and formats need
  ordinary checks; the mutation step exists to show where that gap is.
- **Every adapter is a program.** `spine conform` serves a Python adapter through
  `tools/adapter_shim.py`, as a program speaking JSON lines over stdin and stdout, and runs an
  adapter in any other language the same way, named in `adapter_<machine>.cmd`. Each request is
  a round trip between processes, about 0.2 ms here, so notes' 1,426 sequences take about 2 s.
- **The adapter is part of the oracle.** It must only translate, never decide; a person
  reviews it like the spine.
- **One machine, one variable.** Real features have several interacting machines and more
  variables; the case count grows quickly, and nothing here handles that yet.
