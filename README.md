# verifier-kit

Keep a coding agent honest. You own a small, executable description of what your code must do;
the agent owns only the code; one command, `./verify`, decides whether they agree, and your
hooks and CI run it.

## The problem

A coding agent writes code quickly, and it also writes the tests that pass it. When a test
fails, it can change the test instead of the code, and agents have been seen doing exactly that
(ImpossibleBench). Even when it doesn't, tests written from the same reading as the code show
that the two agree with each other, not with what you meant. And no one can review every line
at the speed an agent writes.

## How verifier-kit solves it

It splits the work. You own the *oracle*: what the system must do, precisely enough for a
program to check. Its core is a *spine*, one small state machine per feature, with exactly one
rule for every state, event and combination of values. The builder owns only the code. A program decides whether
the two agree, and your hooks and CI run it, so the builder can neither skip the check nor change
what it checks against.

![How verifier-kit fits together. You write a chapter in prose; two readers, on different
models, each turn it into a spine; spine diff turns their disagreements into questions, and your
answers make the ratified spine. The builder changes only the code. ./verify, run by the agent's
hook, the git hook and CI, checks the code against the spine through the adapter, runs the
content checks, and plants bugs to check that the oracle notices.](docs/introduction/1-how-it-fits.svg)

- **From prose to spine.** You describe the feature in prose. Two LLM readers, on different
  models, each turn it into a spine, and wherever they disagree you get a question with a run
  that shows it, before any code exists.
- **Every case tried against the real code.** `spine conform` drives your code through a small
  adapter and tries every case of the spine, from a shortest path to each state: the classic
  method for testing state machines (Chow, 1978). A case it couldn't try is an error, not a pass.
- **The checks are checked.** Content checks cover what a spine can't see, such as text and
  totals, and planted bugs make sure the oracle notices when the code is wrong.

[The introduction](docs/introduction.md) explains each part, in order.

## How it compares

verifier-kit tests your real code, in any language, against a small model you own: it chooses
runs that reach every case, and says exactly what it can miss. A proof assistant such as Lean
proves more, for code written in or translated into its language, at far greater cost. A model
checker such as TLA+ explores a design exhaustively, and reaches code only through the runs it
happens to see. Property-based tests sample inputs where conform tries every case. Spec-driven
agent tools let the agent write, and change, the checks that judge it.

Use something else when your logic is mostly computation or content, where property-based tests
fit better, or when clients race each other, which needs a model checker or parallel
state-machine tests. [`docs/comparison.md`](docs/comparison.md) goes tool by tool, for readers
who know them.

## Why verifier-kit

- **The builder can't grade its own work.** The oracle is yours, and deny rules, a Stop hook
  and a required CI check keep the agent out of it. The agent never has to choose to check its
  work: the hook does it.
- **Every case is decided before the code exists.** `spine check` names any case no rule
  decides, and two readings of your prose turn each ambiguity into a question.
- **A guarantee you can read, not a coverage percentage.** Conform's bound is stated, and so is
  what it can miss. Against models of a TLS server, an MQTT broker and a bank card, with faults
  planted in them, it caught every fault its guarantee covers and raised no false alarm
  ([benchmarks](docs/benchmarks.md)).
- **It checks its own checks.** `./verify --full` plants bugs and fails if the oracle misses one,
  and a check is reported stale when the rule it cites changes.
- **Any language, any agent, any CI.** The code is reached through an adapter that speaks JSON
  lines; the examples are in Python and Elixir. The checks make no network or model calls and
  give the same output every time.
- **An LLM does nearly all the writing.** [The guide](docs/guide.md) says which parts it drafts
  and what stays with you.

## Quickstart

See it catch a bug, in the notes example: a service where notes are drafted, published, trashed
and restored. It needs Go 1.27, Bash, `git` and `python3`.

```sh
git clone https://github.com/23min/verifier-kit && cd verifier-kit
./verify examples/notes          # passes: the code agrees with its oracle
cp examples/notes/demo/notes_app_as_written.py examples/notes/src/notes_app.py
./verify examples/notes          # fails: the first version's two bugs, each with the run that shows it
git checkout examples/notes/src  # put it back
```

The second run fails with lines such as:

```
FAIL  notes: code conforms to spine
      error   diverges           draft/trash role=owner: after trash(role=owner): spine expects rejected in draft, code gave ok in trashed
```

In your own project, one line installs the toolkit, and [the guide](docs/guide.md) takes it from
there:

```sh
curl -fsSL https://raw.githubusercontent.com/23min/verifier-kit/main/install.sh | bash
```

The repo also opens in a devcontainer with everything installed;
[`.devcontainer/README.md`](.devcontainer/README.md) says what it mounts from your home.

## Documentation

- [Introduction](docs/introduction.md): what each part is, in order. Start here.
- [Guide](docs/guide.md): your own project, step by step, from installing to gated builds.
- [Examples](examples/README.md): four projects that use the toolkit.
- [Comparison](docs/comparison.md): verifier-kit beside proof assistants, model checkers,
  model-based and property-based testing, and spec-driven agent tools.
- [Reference](docs/reference/): the spine format, the tool contract and the drawing layout.
- [Design](docs/design.md), [architecture](docs/architecture.md),
  [decisions](docs/adr/) and [benchmarks](docs/benchmarks.md).

## Contributing

Ideas, questions and bug reports are welcome as issues. Please open one before a pull request:
I review when I can, and may decline changes that don't fit the design. verifier-kit is under
the [MIT license](LICENSE), so you are free to fork it.
