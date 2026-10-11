# verifier-kit

A small, standalone toolkit for keeping an AI builder honest: people own an
oracle (a state-machine "spine" plus checks), the builder owns only the code, and one script,
`./verify`, decides whether the code matches the oracle. Every gate (agent hook, git hook,
CI) calls that script and nothing else.

"Oracle" here is the software-testing term: a *test oracle* is whatever decides whether an
output is correct.

It is a prototype for learning the process on something small. The example is a notes
service with drafts, publishing, trash and roles; it doubles as the toolkit's acceptance test.

The design is in [`docs/design.md`](docs/design.md); the exact tool contract and spine format
are in [`docs/reference/`](docs/reference/). Rules for agents working here are in
[`CLAUDE.md`](CLAUDE.md). Work is tracked in GitHub issues.

## Try it

```sh
pip install -r requirements.txt   # PyYAML, the only dependency; or: uv venv && uv pip install -r requirements.txt
./verify                    # fast checks, about a second
./verify --full             # also breaks the code on purpose to check the checks
demo/faults.sh              # acceptance test: seeds one fault at a time and checks the right part catches it
demo/live-builder.sh        # optional: a real Claude Code builder session against the oracle
```

Or open the repo in its devcontainer (`.devcontainer/`, "Dev Containers: Reopen in Container"),
which has Python 3 with PyYAML, uv, gh and Claude Code. It was made for the author's machines:
it mounts the host's `~/.claude`, `~/.config/gh` and `~/.guidance`, so on another machine you
may need to trim `.devcontainer/devcontainer.json` or just use a local Python.

## Layout and who owns what

```
spec/notes/chapter.md            the prose chapter you write with an LLM            person
spec/notes/readings/*.yaml       two independent readings of the chapter            interpreters
spec/notes/spine.yaml            the ratified state machine: the oracle's spine     person
spec/notes/behaviors/            example runs: valid/ must be allowed, invalid/ refused
checks/notes/                    content checks the spine cannot see; each cites a spine cell
conformance/adapter_notes.py     translates spine events into calls on the service
mutation/mutate_notes.py         deliberate bugs the oracle must catch
tools/spine.py, run_checks.py    the tools (no knowledge of any CI or agent)
verify                           the single entry point
policy.yaml                      which branch role may change which paths
src/notes_app.py                 the service                                        builder
demo/                            the service as first written (two bugs), faults.sh, live-builder.sh
docs/                            design (normative) and reference: tool contract, spine format
.github/workflows/verify.yml     this repo's own CI: ./verify --full and demo/faults.sh
examples/                        CI, agent and git hook files, to copy when you want them
```

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
| code conforms to spine | `spine conform` | any state/event/role case where the code allows, refuses or ends up somewhere other than the spine says, with the shortest sequence that shows it | anything beyond 4 steps; data the adapter does not observe |
| content checks | `run_checks.py` | wrong text, values, formats | whatever no check asserts |
| mutants killed (`--full`) | `mutate_notes.py` | an oracle too weak to notice a planted bug | bugs unlike the planted ones |

When the spine itself is not sound, the steps that depend on it are skipped. Output is one
line per step, with error lines only for failing steps. `VERIFY_REPORTS=dir` writes JUnit XML
for the spine steps; `spine ... --format json` gives machine-readable findings. Exit codes:
0 pass, 1 findings, 2 misuse.

## Jacking it in

Nothing here assumes a particular CI, agent or workflow tool. The files in `examples/` show
how each layer would call `./verify`. They are examples, linted but not run on a real
GitHub or GitLab instance.

| Layer | Example | What it does | Can it be bypassed? |
|---|---|---|---|
| Agent harness | `examples/claude-code/settings.json` and `hooks/stop-verify.sh` | deny rules keep Claude's file tools out of the oracle; a Stop hook sends Claude back to work while `./verify` fails, with the failing lines as feedback, and gives up after 5 attempts | yes: deny rules do not cover shell commands that write files (use the OS sandbox for that) |
| Git | `examples/git-hooks/pre-commit` | runs fast checks before each commit | yes: `--no-verify` |
| CI, GitHub | `examples/ci/github-actions.yml` | runs `./verify --full` and the path guard on pull requests | not when the job is a required check on a protected branch (private repos need a paid plan) |
| CI, GitLab | `examples/ci/gitlab-ci.yml` | the same, in merge request pipelines, with JUnit in the MR widget | not with "Pipelines must succeed" and a protected branch (both on Free) |
| Review | `examples/CODEOWNERS` | people approve any change to the oracle | GitLab Code Owner approvals need Premium; on Free the guard step stands in |
| aiwf | `examples/aiwf/README.md` | aiwf contracts can call `spine` like any other caller | |

Only CI with a required check is a gate; the agent and git layers make failures cheap to
fix early. The path guard maps a branch name to a role (`build/*` is the builder), so it is
only as strong as the rule about who may push which branch names; protect those patterns too.

## A live run

`demo/live-builder.sh` starts Claude Code (Sonnet) as the builder in a scratch copy, with
the service as first written and the example settings. The builder is told about one bug
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
rule for every state, event and variable combination.

A content check cites the cell it depends on with a comment like
`# covers: trashed/restore@95f9c8204e9f`; `spine hash spec/notes/spine.yaml` prints the
current hashes.

## Adding another machine

Everything is keyed by folder name, so for a machine called `invoice`:
`spec/invoice/spine.yaml`, optional `spec/invoice/behaviors/{valid,invalid}/`,
`checks/invoice/`, `conformance/adapter_invoice.py` (three functions: `new()`, `apply()`,
`observe()`), and optionally `mutation/mutate_invoice.py`. `./verify` picks them up.

## Limits

- **Bounded, not proven.** Conformance replays every sequence up to 4 steps. That finds most
  logic slips in small machines (the small-scope hypothesis), but it is testing, not proof.
- **The spine sees states and permissions, not content.** Text, totals and formats need
  ordinary checks; the mutation step exists to show where that gap is.
- **The adapter is Python and in-process.** Code in another language would need a process
  adapter (for example JSON lines over stdin and stdout). Not built yet.
- **The adapter is part of the oracle.** It must only translate, never decide; a person
  reviews it like the spine.
- **One machine, one variable.** Real features have several interacting machines and more
  variables; the case count grows quickly, and nothing here handles that yet.
