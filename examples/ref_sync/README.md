# Example: ref-sync

Two clients, Alice and Bob, publish workflow events to one shared Git ref,
`refs/workflow/state`, whose history has its own root and is never merged into `main`. Where
the notes example is one lifecycle with permissions, ref-sync is a protocol: what goes wrong
is lost replies, crashes, retries and two clients racing on one ref. The chapter
(`spec/ref_sync/chapter.md`) gives the rules, the invariants S1-S7, the scenarios C01-C12 and
what is assumed of Git. The work is tracked in issue #18; the decisions are in
`docs/adr/0002-ref-sync-example.md`.

## What it shows

- A spine for one operation's lifecycle (`queued`, `prepared`, `uncertain`, and three final
  states), ratified from two readings by different models. `spine diff` found one question;
  the owner also chose the state kinds, where both readings shared a misreading `diff` cannot
  see.
- What a spine cannot do: `spine conform` checks one operation at a time, so two clients
  racing need something else. Here that is an explorer that runs two real clients from `src/`
  through every interleaving of their steps, lost replies and crashes, up to a stated bound,
  and checks S1-S5 and S7 in every world it reaches.
- Checking a simulation's assumptions: the simulated remote assumes Git refuses a push that is
  not a fast-forward; `test_git_ref.py` checks that, and the client's Git remote, against real
  Git.
- A client written by a different builder (Codex) from the oracle alone, and six mutants of
  it, each caught by a named check.

## Run it

From the repo root:

```sh
./verify examples/ref_sync          # fast checks, including the explorer and the Git checks
./verify examples/ref_sync --full   # also the six mutants
VERIFY_VERBOSE=1 ./verify examples/ref_sync   # also the explorer's bounds and world counts
VERIFY_VERBOSE=1 ./verify examples/ref_sync --full   # also each mutant's first problem found
```

It needs the `git` program besides Python 3 and PyYAML. The client uses it to publish, and the
Git checks build throwaway repositories in a temporary folder with git's global and system
configuration switched off; nothing contacts another host. Without `git` the Git checks fail;
they never skip.

## Layout and who owns what

```
spec/ref_sync/chapter.md             the prose chapter: rules, S1-S7, C01-C12, assumptions   person
spec/ref_sync/readings/              two readings, with the prompt and their models           interpreters
spec/ref_sync/spine.yaml             the ratified lifecycle of one operation                  person
spec/ref_sync/behaviors/             valid/ must be allowed, invalid/ refused                 person
conformance/adapter_ref_sync.py      the lifecycle adapter; its docstring is the whole        person
                                     contract with src/
conformance/adapter_ref_sync.cmd     the line that runs it, through adapter_shim.py           person
checks/run.cmd                       the line that runs the checks, through run_checks.py     person
checks/ref_sync/explore.py           breadth-first search over moves; knows nothing of Git    person
checks/ref_sync/world.py             the simulated remote, faults, crashes, S1-S5 and S7      person
checks/ref_sync/test_protocol.py     scenarios C01-C08                                        person
checks/ref_sync/test_git_ref.py      C09-C11 and S6 against real Git                          person
mutation/mutants.yaml                six deliberate bugs the oracle must catch                person
src/ref_sync_client.py               the lifecycle, the client and its Git remote             builder
```

The client has a pure core and a shell. `transition(state, event)` is the lifecycle, and
`spine conform` drives it one event at a time. `Client` keeps a journal and turns fetches and
pushes into the spine's events; the explorer and the Git checks drive it.

## What is checked, and how far

| What | How | Bound |
|---|---|---|
| The lifecycle matches the spine | `spine conform` on `transition()` | every sequence up to 4 events |
| S1-S5, S7 in every reachable world; each scenario's outcome | `test_protocol.py`, C01-C06 | below |
| The checks catch a broken tip (S4) and a false confirmation (S5) | C07, C08 on worlds made by hand | |
| Git refuses a push that is not a fast-forward; the client's Git remote reports it | `test_git_ref.py`, C09 | one repository |
| Publishing leaves `main` and code branches alone (S6) | `test_git_ref.py`, C10 | one repository |
| A fresh clone has the workflow ref only after fetching it | `test_git_ref.py`, C11 | one repository |

`world.py` and `test_protocol.py` depend on the whole spine rather than on single cells, so each
cites it as `covers: spine@<hash>`: when what the machine does changes, `spine stale` asks for
them to be read again. The Git checks and `explore.py` don't depend on the spine and cite nothing.

The explorer runs two clients with one intent each. A fault is a push whose reply is lost
after the remote applied it, or a push held back and delivered later; a crash replaces a client
with a new one built from the same journal. Every scenario explored every world reachable
within its bound, within 30 moves:

| Scenario | Faults | Worlds | Every operation ended |
|---|---|---|---|
| C01 different entities | none | 19 | 4 |
| C02 same entity | none | 15 | 4 |
| C03 reply lost after publication | 1 lost reply | 69 | 16 |
| C04 lost or delayed push, different entities | 1 lost reply or delayed push | 275 | 46 |
| C04 lost or delayed push, same entity | 1 lost reply or delayed push | 175 | 32 |
| C05 crashes | 1 lost reply or delayed push, 1 crash per client | 1272 | 184 |
| C06 same event ID, other digest | none | 15 | 4 |

A green explorer means "no problem in any of these worlds", not a proof. A failure prints the
shortest sequence of moves that shows it, for example (the client forces its push):

```
C01: S1: commit 4ca3f40f24d0, once the tip, is no longer in the workflow ref's history;
after: alice fetches [e-alice: prepared] -> bob fetches [e-bob: prepared]
  -> alice pushes (accepted, forced) [e-alice: confirmed]
  -> bob pushes (accepted, forced) [e-bob: confirmed]
```

## The mutants

`mutation/mutants.yaml` lists six small edits to `src/ref_sync_client.py`. `./verify --full`
makes each in a copy of the example and runs conformance and the content checks on it, as for
every example; it first runs them on the client as it is, and stops with `oracle-fails` if they
don't pass there, since every mutant would then look caught. Each mutant's line names the first
problem found. The checks that fail are those this example's own mutation script listed before
#58 moved the mutants to data; the checks haven't changed since.

| Mutant | First problem found | Checks that fail |
|---|---|---|
| the client forces its push | S1 in C01 | C01-C06 |
| event IDs are not deduplicated | C03 ends conflicted, not confirmed | C03-C06 |
| a push without an answer counts as confirmed | S5 in C03 | C03-C05 |
| a retry does not check the expected version again | S3 in C02 | C02, C04 |
| the event is published without its entity change | S4 in C01 | C01-C06 |
| the journal drops an operation while its push is under way | S7 in C01 | C01-C06 |

No mutant is caught by `spine conform`: all six break the shell, not `transition()`. The
deduplication mutant is caught as S2 only in C06, where Bob's commit rewrites the record of
Alice's event. Elsewhere the version check stops it first: a replayed intent ends conflicted
rather than published twice.

## Assumed of Git, and not checked

From chapter section 5, the simulated remote and the client rely on these; only the first is
checked against real Git, and only for one push at a time:

- the remote refuses a push without force that is not a fast-forward, and applies each update
  of the ref atomically;
- nobody force-pushes to the workflow ref or deletes it, and published commits stay reachable;
- a fetch returns the remote's current tip;
- connections fail and replies get lost, but work again after a while;
- the journal survives a crash (here it is a dict the caller keeps; durable storage is not
  part of the example);
- the Git host accepts a custom ref. C12 is documented, not run: the fallback is a protected
  branch with its own root, such as `refs/heads/workflow-state`, chosen on purpose.

## Not verified

- **The shell against the spine.** `conform` checks `transition()` only. That the client
  turns each fetch and push into the right event, and records every state change in its
  journal, is checked through the invariants and outcomes, not step by step.
- **The Git remote under faults.** Lost replies and crashes are simulated; `GitRemote` meets
  real Git only in the four Git checks, where nothing fails. Its mapping of other push
  failures to "no definite answer" is not exercised.
- **Beyond the bound:** more clients, more intents per client, more lost replies or crashes.
  One more lost reply takes C05 from 1,272 worlds in 0.8 s to 7,516 in 5.7 s (no problem
  found); the bound stays at one so `./verify` stays fast.
- **Liveness.** Nothing promises that every operation is eventually published.
- Authorization, malicious clients and anything about a hosted Git service (chapter section 6).

## Measured

In the devcontainer (16 CPUs, Python 3.12.3, git 2.51.1), median of three runs:

| Command | Time |
|---|---|
| `./verify examples/ref_sync` | 1.9 s |
| `./verify examples/ref_sync --full` | 8.2 s |
| `./verify` (both examples) | 2.4 s |
| `./verify --full` | 9.7 s |
| `demo/faults.sh` | 42 s |

`demo/faults.sh` takes 5.6 s on `main` without this example. It runs `./verify` on a copy of
the whole repo in every scenario, so each one also runs ref-sync's checks, and the three
`--full` scenarios its mutants, though every seeded fault is in the notes example.

## Future work

- A host that refuses custom refs: the fallback branch of C12, and checking which hosts allow
  protecting a custom ref.
- Trust in the server: a remote or a client that does not follow the rules.
- Authorization: who may publish which events, and permissions revoked mid-operation.
- Binding code changes to the workflow state that authorized them.
- Compaction and retention of the workflow ref's history.
- A comparison with a database built for this, such as Dolt.
- The explorer's search loop, `explore.py`, is meant to be taken over by a deterministic
  simulator.
