# 0002. The ref-sync example: a protocol checked against real code

- Status: accepted
- Date: 2026-10-14
- Issue: #18

## Context

The notes example checks one lifecycle with permissions. ref-sync is a protocol: two clients
publish workflow events to one Git ref with its own root, and lost replies, crashes, retries
and two clients racing on one ref are where it can go wrong. `spine conform` runs one
lifecycle at a time and cannot see two clients at once; `docs/design.md` says concurrency
needs a model checker or deterministic simulation. The example started from a handoff
proposal and a seed spine, neither ratified.

## Decision

1. **Scope** is the handoff's invariants S1-S7 and scenarios C01-C12, with its IDs. The
   chapter says a client may crash, and a fetch or push may fail, at any step, so the
   explorer reaches those cases without naming them. Authorization is out.
2. **The lifecycle** changes the seed: a queued operation that finds its own event is
   confirmed; the same event ID with another digest ends in `protocol_error`; a `restart`
   takes every unfinished operation back to `queued`, keeping only the intent. The chapter
   says what the client can observe in each state, and any other event is refused.
3. **Two readings** came from subagents on two Claude models other than the session that
   wrote the oracle, each given only the chapter, the spine format and a fixed prompt. They
   are committed unedited with their model, date and input hashes. `spine diff` found one
   question; the owner also chose the state kinds, which `diff` does not compare.
4. **The oracle drives the real client.** `src/` offers a pure lifecycle function, which
   `conform` drives one event at a time, and a client shell, which the explorer drives step
   by step against a simulated remote with lost and delayed replies and crashes. The
   environment and the invariants are the oracle's; the search loop (`explore.py`) knows
   nothing about Git, so the simulator can take it over.
5. **`git` is a dependency of this example's checks and client**, not of the toolkit. The
   real-Git checks run on every `./verify`, in throwaway repositories with git's global and
   system configuration off. A missing `git` fails; it never skips.
6. **Codex writes the client** from the ratified oracle, on a `build/*` branch; Claude writes
   the oracle. The path guard checks the builder's diff before it merges. Codex's sandbox
   cannot start in the devcontainer (it needs user namespaces), so Codex runs without it,
   told to write only in its worktree and the temporary folder and not to use the network,
   and every file changed outside its worktree during the run is listed afterwards.
7. **One merge to `main`**: the oracle, the client and the rest (mutants, README, docs) are
   reviewed as three pull requests into `example/ref-sync`, which then merges once.

## Consequences

- `conform` checks only the client's lifecycle function. Whether the shell turns each fetch
  and push into the right observations, and keeps its journal, is checked by the explorer
  and the Git checks, through the invariants.
- A green explorer means "no problem in every world reachable within the stated faults",
  not a proof. The output says how far it went.
- The adapter's docstring is the whole contract with the client, so a builder can work from
  it alone, and the client's shape (pure core, ports for the remote and the journal) is set
  by the oracle.
- An example may now need a program besides Python; its README says so.
- `demo/faults.sh` runs `./verify` on the whole repo in every scenario, so it now also runs
  ref-sync's checks and mutants: 42 s instead of 5.6 s on `main`.

## Alternatives considered

- **An explorer with its own model of the client.** It checks the design, not the code, so
  mutants planted in `src/` would survive.
- **`conform` driving the shell against a simulated remote.** One fetch can produce two spine
  events, so the adapter would have to know how the client splits it, which is deciding.
- **Real-Git checks only with `--full`, or not at all.** The first needs a change to
  `./verify`; the second leaves the simulated remote's assumptions unchecked.
- **Readings by two vendors, or by a person.** Different vendors were closer to the design;
  subagents were chosen for cost, with independence resting on instructions.
- **Codex in its sandbox**, by allowing user namespaces in the devcontainer or by running it
  on the host. The first loosens the container for every tool in it; the second makes every
  run a manual step.
- **Merging the lifecycle first and concurrency later.** Smaller reviews, but a partial
  example on `main` and a builder reworking its client.
