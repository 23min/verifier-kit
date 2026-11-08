# Example: oban_job

One email sent from an Oban job, checked against Oban itself: version 2.24.2, on SQLite. Where
elixir_job checks ten lines of our own that model a job with retries, this example checks a
spine written from Oban's documentation against what the app's worker and the library do
together, so the toolkit is tried on behavior it doesn't control. The chapter
(`spec/delivery/chapter.md`) was written from Oban's documentation alone, and its section 5 says
what it assumes where the documentation is silent or contradicts itself. The work is tracked in
issue #110.

## What it shows

![The delivery spine drawn: the waiting states run down the middle, available_0, scheduled_0,
retryable_1, scheduled_1, retryable_2 and scheduled_2, with discarded below them. Each waiting
state moves to completed, on the left, when the server accepts, and to cancelled, on the right,
on a bounce or a cancel. A failure moves available_0 or a waiting state to the next retryable
state, and retryable_2 or scheduled_2 to discarded; a rate limit moves available_0 or a
retryable state to the scheduled state with the same count. completed, cancelled and discarded
are terminal. A table below gives every other rule by event and state.](spec/delivery/drawing.svg)

The ratified spine, drawn by `spine draw` from `spec/delivery/drawing.yaml`.

- A spine with the attempt count in its states, as Oban keeps it: `available_0`, `retryable_1`
  and `retryable_2`, `scheduled_0` to `scheduled_2`, and three final states. Its one variable,
  `answer`, is what the mail server answers. It was ratified from two readings by different
  models, which agreed on every cell; the owner made `retryable_1` and `retryable_2` policy,
  where the readings differed.
- `conform` driving code the toolkit didn't write: the adapter runs real Oban, on SQLite, in one
  process for the whole run. A `run` is one `Oban.drain_queue/2`; a `wait` is one with
  `with_scheduled: true`, which makes every waiting job available and runs it.
- A chapter written by an agent that read Oban's documentation and not its source, so that
  `conform` would show any assumption that is wrong.
- Mutants caught by `conform` alone, by the content checks alone, and by both.

## Run it

From the repo root:

```sh
elixir examples/oban_job/src/deps.exs   # once, online: Oban and its packages
./verify examples/oban_job              # fast checks
./verify examples/oban_job --full       # also the seven mutants
```

It needs Elixir 1.18 or later, for its built-in `JSON`, besides what the toolkit needs; setting
up also fetches exqlite's prebuilt SQLite library. There is no Mix project. `src/deps.exs` calls
`Mix.install`, pinned by `src/mix.lock`, and Mix keeps the packages in `~/.cache/mix`, outside
the repo. Every command line loads `src/deps.exs`, then `src/**/*.ex`, with `HEX_OFFLINE=1`, so
`./verify` fetches nothing; on a machine that hasn't set up, they fail and print the line above.
`deps.exs` sits in `src/` because `./verify --full` copies only `spec/`, `conformance/`,
`checks/` and `src/` (#113).

## Layout and who owns what

```
spec/delivery/chapter.md        the prose chapter, from Oban's docs: D1-D6, D01-D14     person
spec/delivery/readings/         two readings, with the prompt and their models          interpreters
spec/delivery/spine.yaml        the ratified lifecycle of one email's job               person
spec/delivery/drawing.yaml      where each state sits in the spine's drawing            person
spec/delivery/drawing.svg       the drawing, made by spine draw                         person
spec/delivery/behaviors/        valid/ must be allowed, invalid/ refused                person
conformance/adapter_delivery.*  the adapter; its header is the contract with src/       person
checks/run.exs, run.cmd         runs the checks; reports as tools/run_checks.py does    person
checks/delivery/                backoff and snooze lengths, max_attempts, the address   person
mutation/mutants.yaml           seven deliberate bugs the oracle must catch             person
src/mailer.ex                   the app: its worker, sending and cancelling             builder
src/deps.exs, mix.lock          the Hex packages the app uses, pinned                   builder
```

## What is checked, and how far

| What | How | Bound |
|---|---|---|
| The app and Oban match the spine | `spine conform` through `adapter_delivery.cmd`, on Oban 2.24.2 with SQLite | every case, margin 1: 396 sequences |
| The drawing shows the spine | `spine draw` on `spine.yaml` and `drawing.yaml`, compared with `drawing.svg` | byte for byte |
| Scenarios D01-D14 end as the chapter says, and D1, D2, D4, D5 and D6 rule out what they should | `spine behaviors` on `spec/delivery/behaviors/` | one run each, against the spine |
| A backoff lasts 30 seconds times the attempt, a snooze 60 seconds; a snooze leaves `max_attempts` at 3; the email goes to its address once per attempt | `checks/delivery/delivery_test.exs` | one run each |

**What `conform` found:** nothing. On every sequence, Oban did what the ratified spine says, so
the owner had no disagreement to resolve. The chapter's assumptions where the documentation is
silent or contradicts itself hold for 2.24.2: `attempt` starts at 0 and goes up as an attempt
starts (A1); `max_attempts: 3` means three attempts in all (A2); a snoozed job is `scheduled`,
and the snooze gives back exactly one attempt (A3). The documentation's contradictions stand as
section 5 records them. One more gap, which the adapter relies on: the documentation says
`with_scheduled: true` makes scheduled jobs available, and it makes retryable ones available
too.

## The mutants

`mutation/mutants.yaml` lists seven small edits to `src/mailer.ex`, which `./verify --full`
makes and judges (`docs/reference/tool-contract.md`).

| Mutant | Caught by |
|---|---|
| a bounce is snoozed | delivery: code conforms to spine |
| a rate limit uses an attempt | delivery: code conforms to spine, content checks |
| one attempt too many | delivery: code conforms to spine, content checks |
| cancel does nothing | delivery: code conforms to spine |
| the backoff does not grow | content checks |
| a short snooze | content checks |
| the email goes elsewhere | content checks |

The last three change how long the job waits or where the email goes, which the lifecycle
doesn't show, so `conform` passes on them.

## Not verified

- **A running job.** Each `run` or `wait` is one `drain_queue`, so a job goes from available to
  its attempt's outcome in one step: `executing` is never seen, and a cancel while the worker
  runs is never tried (#110, OQ1).
- **A job available again after its backoff.** A `wait` makes it available and runs it in one
  step (#110, OQ4), so a cancel at that moment is never tried.
- **Real time.** Oban runs in `testing: :manual`, so its stager, its queues' timers and
  time-outs don't run; the lengths of a backoff and a snooze are checked as the times Oban
  records.
- **What the chapter leaves out** (its section 6): retrying a job by hand, `suspended`, jobs
  inserted as scheduled, several nodes or queues, unique jobs, Postgres and Oban Pro.
- **Other versions.** The spine is checked against Oban 2.24.2, as `src/mix.lock` pins it.
