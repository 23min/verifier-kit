# A shared explorer for concurrency

- Status: proposal, not decided; waits for a second example that explores concurrency (#61 D8)
- Date: 2026-10-24, against `main` at `8ea3870`
- To adopt: #61's D8 decides when; then an issue the owner approves, since it adds a tool and
  a protocol to the contract

## What exists

`spine conform` runs one lifecycle at a time, so a protocol where clients race needs a check of
its own (`docs/design.md`, "Concurrency needs a model checker or deterministic simulation").
Two examples search a state space in their content checks today, each in its own language:

- **ref_sync**, in Python. `checks/ref_sync/explore.py` is a breadth-first search that knows
  nothing of Git: given a starting world and `moves`, `play` and `problems`, it tries every move
  on a copy of every world it reaches, up to a number of moves, and stops at the first world with
  a problem, whose moves are then a shortest witness. `checks/ref_sync/world.py` holds the world:
  two real `Client`s from `src/`, a simulated remote that can lose or delay a reply, crashes,
  and invariants S1-S5 and S7. `checks/ref_sync/test_protocol.py` runs scenarios C01-C06
  through it, and for each also asks that some run ends, that every ended run has the outcome
  the chapter names, and, for all but C06, that some ended run shows the fault the scenario is
  about. On `main`
  at `8ea3870`, its 7 runs reach 1,840 worlds and play 5,834 moves, in about 1.0 s.
- **elixir_job**, in Elixir. `checks/job/job_test.exs` walks every job reachable by accepted
  events, in 15 lines, and checks each step it takes against J1 and J2. One job, no
  concurrency.

ADR 0002 decided that the explorer drives the real client, not a model of it, so that mutants
planted in `src/` are caught, and kept the search loop free of Git "so the simulator can take it
over". Nothing has taken it over yet.

## The handoff's proposal

A handoff proposal, `SIMULATOR_PROPOSAL.md`, written on 2026-10-13 before ref_sync existed and
never committed, asks for a generic explorer in Go. In short:

- a pure Go engine, with no I/O, clocks or randomness, behind a typed interface:
  `Model[S, A]` with `Initial`, `Enabled`, `Step`, `Key`, `Violations` and `ActionName`, and
  limits on depth, states and transitions;
- breadth-first search, so a failure comes with a shortest witness, counted in moves;
- five outcomes instead of pass or fail: `violation_found`, `exhausted_finite_model`,
  `bounded_no_violation`, `budget_exceeded` (inconclusive, never a pass) and `model_error`;
- the model kept apart from the implementation, which separate tests compare with it;
- ref_sync's protocol as the first model, a second model with nothing to do with Git (two
  workers claiming one job) to show that the engine is generic, and a mutant that loses an
  update, which the engine must catch;
- phases A to F, starting as an internal Go package run by `go test`.

This note keeps the shortest witness, the honest outcomes, the second model as the test of
genericity, and the engine's ignorance of any domain. Two parts no longer fit:

- **The model apart from the code.** ADR 0002 weighed an explorer with its own model of the
  client and rejected it: it checks the design, not the code, so mutants in `src/` survive.
- **A model typed in Go.** A `Model[S, A]` must be written in Go. ADR 0007 builds no language
  into the toolkit, and examples are equal; ref_sync's client is Python, and an Elixir project
  could not plug in at all.

## The design

The adapter protocol, applied to search. A Go tool owns the search; a program the oracle owns,
in any language, owns the world, as `world.py` does today.

**The tool** is a program of its own, `tools/cmd/explore`, files in, findings out. It owns the
search order, removing worlds seen before, the bound, the witness, the outcome of each
scenario, findings in the contract's format and exit codes 0, 1 and 2. It knows nothing of
any domain, and it runs one program: the world's command line.

**The world program** speaks JSON lines over stdin and stdout, like an adapter. Unlike an
adapter it holds many worlds at once, each by a number the program gives it, so the tool never
needs to read a world, and the real client never needs to be turned into data:

| Request | Reply |
|---|---|
| `{"op": "scenarios"}` | `{"scenarios": [{"name": "C01", "title": "...", "bound": "2 clients, ...", "max_moves": 30, "needs": ["refused"]}, ...]}` |
| `{"op": "start", "scenario": "C01"}` | a world, as below |
| `{"op": "play", "world": 7, "move": ...}` | a world, as below, or `{"nothing": true}` when the move does nothing |

A world, in a reply: `{"world": 8, "key": "...", "what": "alice pushes (accepted)", "moves":
[...], "problems": [...], "ended": true, "shows": ["refused"]}`. `key` is the world's
fingerprint, by which the tool removes worlds seen before. `what` describes the move, for the
witness. `moves` are the moves possible next, as JSON the tool passes back without reading.
`problems` covers the world and the step that reached it, so a check on each step, as
elixir_job's is, fits too. `ended` says every operation has ended; `shows` names what this
world shows, for the scenario's `needs`. Each move played is one round trip, and the reply
carries everything the tool needs about the new world.

**For each scenario, the tool reports** one of:

- a problem: the first one found, with the moves that reach it (`world-problem`, error);
- every reachable world explored, with no problem (`explored`, info, with the bound and the
  count of worlds);
- the bound reached first, with no problem: `explored`, info, saying that it stopped at
  `max_moves` and how many runs it left unfinished, as `test_protocol.py` says today.

It fails a scenario in which no run ends (`nothing-ended`), or in which no ended run shows
something the scenario `needs` (`not-shown`). These are what keep an explorer from passing
because it explored nothing that matters. A program that can't be started, exits early, or
breaks the protocol is `world-failed`, as an adapter's is `adapter-failed`. The codes are
names for the issue to settle.

**In a Python project,** `tools/world_shim.py` serves a file with `scenarios()`, `start()`,
`moves()`, `play()`, `problems()` and `ended()` over the protocol, as `tools/adapter_shim.py`
serves an adapter. It copies a world before each move and computes its fingerprint, with the
`fingerprint` that is in `explore.py` today. ref_sync would then lose `explore.py` and the
search half of `test_protocol.py`; `world.py` would gain its scenarios, and the outcomes and
`needs` they check. C07 and C08, which check the checks on worlds made by hand, stay content
checks.

**In `./verify`,** an optional piece: `checks/explore_<name>.cmd`, whose line is given to the
tool, run from the root like every other piece. A root without one prints `skip`, as one
without behaviors does.

**Cost.** One round trip per move played: for ref_sync, about 5,840. At the 0.2 ms the README
measures for an adapter's round trip, that adds about 1.2 s to the 1.0 s it takes in process.
That is an estimate, not a measurement.

## What it leaves out

Each of these waits until a case needs it:

- **Random search,** "deterministic simulation" in the sense of FoundationDB: a seeded random
  walk for state spaces too big to search to the end. ref_sync's largest scenario has 1,272
  worlds.
- **Budgets** on states and transitions, and with them the handoff's `budget_exceeded`. The
  bound on moves is the only limit today.
- **Freeing worlds.** The program keeps every world it made, as `explore.py` keeps every world
  it reached.
- **Replaying the witness** from the start, to show that the program is deterministic.
- **Reduction** by partial order or symmetry, and liveness.
- **A second concurrent example.** The handoff wants one to show the engine is generic; it is
  new scope, so it is a separate choice.

## Open questions

1. **When.** #61's D8 recommends a shared explorer only when a second example needs one. On
   this design, ref_sync's search moves into Go and its Python world stays. elixir_job's
   15 lines would become an Elixir world program with its own protocol loop, as its adapter
   has; that costs more than it saves. So today there is one user, which supports D8's
   recommendation.
2. **A program of its own, or `spine explore`?** ADR 0007 says the one program `spine` runs is
   the adapter's line, and the explorer needs no spine. A program of its own keeps `spine` as
   it is.
3. **Scenarios as data or from the program?** Here the program lists them, so no format is
   added. A file of scenarios and bounds would let a reviewer see the bounds without reading
   code, as `mutants.yaml` will for mutants (#58).
4. **Trusting the fingerprint.** The program computes `key`; a fingerprint that leaves something
   out hides bugs. The world program is oracle side and reviewed like an adapter; is that
   enough, or should the tool compute it from a world the program sends as data, which the real
   client would then have to become?
