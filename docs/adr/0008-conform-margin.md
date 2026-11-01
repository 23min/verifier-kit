# 0008. Conform tries every case: a shortest path to each state, then a margin

- Status: accepted
- Date: 2026-11-01
- Issue: #89

## Context

`spine conform` replayed every event sequence up to a depth, 4 by default, and nothing let a
project set another. A case is first tried one step after its state is first reached, so the
cases of any state four or more events from `initial` were never tried, and conform said so
only in an `info` coverage line, which `./verify` prints only with `VERIFY_VERBOSE=1`.

#89 showed it on a chain of six states, `created` to `returned`, five events, with code that
refuses `return_item` from `delivered`, four events out: `./verify` passed, and so did
`./verify --full` unless a mutant happened to sit beyond step 3. The three examples escaped only
because every case they have is reached by step 3.

Running deeper is not the fix. Setting the depth from the spine, one step past its furthest live
state, tries every case, but the cost grows with the depth exponentially, and leaves no margin at
the furthest state. On chains branching like notes (three roles; per state the owner moves on,
the owner and the editor edit, the owner makes a no-op), measured with `spine conform`:

| States | Depth needed | Runs | Time per conform |
|---|---|---|---|
| 6 | 6 | 12,286 | 24.2 s |
| 7 | 7 | 49,150 | 100 s |
| 9 | 9 | 786,430 | about 35 min, estimated |

`./verify --full` runs conform again for every mutant, and the Stop hook runs `./verify` at
every stop.

## Decision

1. **Conform tries every case from the shortest path to its state, plus a margin.** For each live
   state, it takes the first shortest path to it, found breadth first in the spine's order of
   events and variable values, then runs every sequence of 1 to K+1 further steps. A run stops at
   a terminal state, after a refusal, or where the code diverges. A sequence two states share runs
   once. K is the margin.
2. **`--margin K` replaces `--depth N` on `conform`:** a whole number, 0 or more, 1 unless given.
   `spine diff --depth` stays: its case-by-case comparison already covers every case.
3. **A case left untried is an error,** `untried-case`, one per case, naming it. Every case is
   tried one step past its state's path, so a case is untried only when the code diverged on the
   way there.
4. **Refusals still end a run.** Hidden state that a refusal changes, such as a lockout after
   three refused attempts, can't be seen at any margin. It is a known limit, written down.

## Why it is enough

This is the classic method for testing finite state machines: Vasilevskii (1973) and Chow (1978),
surveyed by Lee and Yannakakis (1996). Reach each state by one fixed path, a state cover, then try
every sequence of up to k+1 inputs from there. When the state can be read directly, as
`observe()` reads it, this catches every implementation that has up to k states the spec doesn't,
provided the code is deterministic and `new()` starts it afresh. The margin is the same at every
state, and the cost grows with the spine's size, not exponentially with its depth. The old search
ran a superset of these sequences, with its margin set by its furthest state.

| Spine | Depth 4 before | Margin 1 | Margin 0 (every case once) |
|---|---|---|---|
| notes | 1,426 runs | 180 | 45 |
| ref_sync | 352 | 81 | 27 |
| elixir_job | 91 | 60 | 30 |
| 9-state chain | 766, trying only 36 of its 81 cases | 324 | 81 |

The default margin, 1, keeps the margin the three examples had at depth 4. A larger one finds
more hidden state: on notes, code whose `restore()` fails on the third restore from the trash is
caught at margin 4 (14,130 runs, 26 s) and not at margin 3 (3,285 runs, 5 s).

## Consequences

- `./verify` fails on #89's chain, with `diverges delivered/return_item`, and passes on correct
  code with every case tried.
- The examples take fewer runs, and `./verify --full` is faster for it.
- A project that relied on `--depth` for conform passes `--margin`; no project uses the toolkit
  yet, and no example or caller passes either.

## Alternatives considered

- **Replay every sequence to a depth computed from the spine.** Every case is tried, but the cost
  is exponential in the depth (above), and the margin at the furthest state is none.
- **Keep the depth and cap the cost.** A cap leaves cases untried again, which is the gap #89
  closes; reporting them would make the cap a failure on any long spine.
- **Report untried cases as a warning.** `./verify` would still pass, which is the bug.
