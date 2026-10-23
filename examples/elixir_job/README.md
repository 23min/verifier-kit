# Example: elixir_job

One background job with retries, as Oban runs them, written in Elixir. Where notes and ref_sync
are Python, this example shows the toolkit checking a project in another language. Its adapter,
content checks and mutation are `.cmd` files, as every example's are, each holding one command
line that `./verify` runs from this folder (`docs/adr/0007-no-language-built-in.md`); its lines
run Elixir where theirs call the Python helpers in `tools/`. The chapter (`spec/job/chapter.md`) gives
the rules, J1 and J2, and the scenarios J01-J07. The work is tracked in issue #35.

## What it shows

- A spine for one job's lifecycle (`queued`, `running`, `retrying`, and two final states), with
  one variable, `attempts`. It was ratified from two readings by different models, which agreed
  on every cell; the owner chose the rules before the readings and made `discarded` policy, not
  fact, after them.
- An adapter that is a program: `conformance/adapter_job.exs` speaks the adapter protocol over
  stdin and stdout, and `spine conform` drives the job's pure lifecycle through it.
- What the lifecycle can't see: it takes `attempts` as given, so whether the job counts its own
  attempts is checked by the content checks, in ExUnit, against J1 and J2 in every job
  reachable with maximum attempts 1 to 4.
- A job written by a different builder (Codex) from the oracle alone, and five mutants of it.

## Run it

From the repo root:

```sh
./verify examples/elixir_job          # fast checks
./verify examples/elixir_job --full   # also the five mutants
```

It needs Elixir 1.18 or later, for its built-in `JSON`, besides what the toolkit needs. There
is no Mix project: each command loads `src/**/*.ex` with `elixir -pr`, so nothing is built or
cached.

## Layout and who owns what

```
spec/job/chapter.md                the prose chapter: rules, J1-J2, J01-J07, assumptions      person
spec/job/readings/                 two readings, with the prompt and their models             interpreters
spec/job/spine.yaml                the ratified lifecycle of one job                          person
spec/job/behaviors/                valid/ must be allowed, invalid/ refused                   person
conformance/adapter_job.exs, .cmd  the adapter; its header is the whole contract with src/    person
checks/run.exs, run.cmd            runs the checks; exits 0 pass, 1 a check failed, 2 none ran person
checks/job/job_test.exs            J01-J07, and J1 and J2 in every reachable job              person
mutation/mutate_job.exs, .cmd      five deliberate bugs the oracle must catch                 person
src/job.ex                         the lifecycle and the job                                  builder
```

## What is checked, and how far

| What | How | Bound |
|---|---|---|
| The lifecycle matches the spine | `spine conform` through `adapter_job.cmd`, on `Job.Lifecycle.transition/3` | every sequence up to 4 events |
| Scenarios J01-J07 end as the chapter says | `checks/job/job_test.exs` | one run each |
| J1 and J2: attempts are counted, bounded and retried until the limit | the same file, over every reachable job | maximum attempts 1 to 4 |

## The mutants

`mutate_job.exs` makes one small edit at a time to `src/job.ex`, in a copy of the example, and
runs the same two commands `./verify` runs on it. It first runs them on the job as it is, and
stops if they don't pass there, since a broken command would make every mutant look caught.

| Mutant | Caught by |
|---|---|
| a last failure is retried | conformance, checks |
| cancel is refused while running | conformance, checks |
| one attempt too many | checks |
| start does not count an attempt | checks |
| a running attempt counts as the next | checks |

The last three break the job's counting, not its lifecycle, so `conform` passes on them; with
the content checks deleted they survive, as `demo/faults.sh` scenario 13 shows.

## Not verified

- **The job against the spine, step by step.** `conform` checks `Job.Lifecycle.transition/3`
  only. That `Job` passes the right value of `attempts` is checked through J1, J2 and the
  scenarios, not on every sequence.
- **Anything about time.** Backoff, time-outs and scheduling are out of the chapter's scope.
