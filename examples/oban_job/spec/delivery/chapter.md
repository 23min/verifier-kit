# delivery: sending one email as an Oban job

An app sends each email from a background job, run by the Oban job library, version 2.24.2, on
SQLite (`Oban.Engines.Lite`). The job's worker calls a mail server once per attempt, and the
server's answer decides what the worker returns to Oban. A failed attempt is tried again, up to
a limit, and an operator can cancel the job before it ends. This chapter says how one job moves
from being inserted to its outcome. It describes Oban as Oban's own documentation for 2.24.2
describes it. Where that documentation is silent or contradicts itself, section 5 says what the
chapter assumes, and why.

## 1. Glossary

The rest of this chapter uses only these names for the states, events and variable of a job.

Things:

- **job**: one email to send, inserted once into Oban to run now, never scheduled for later. Its
  row in the database holds, among other fields, Oban's `state` for the job and its `attempt`.
- **worker**: the app's `Oban.Worker` module for the job. Oban calls its `perform/1` once per
  attempt, and `perform/1` calls the mail server once.
- **mail server**: answers each call with `accepted`; `failed`, a temporary failure; `bounced`,
  the address doesn't exist; or `rate_limited`.
- **attempt**: one run of the worker on the job. The job's `attempt` field counts its attempts,
  less any that a snooze gave back (section 5, A1 and A3).
- **max_attempts**: how many attempts the job may have. The app's worker sets 3.
- **backoff**: the time a job waits after a failed attempt before Oban makes it available again.
  The app's `backoff/1` returns 30 times the job's `attempt`, in seconds.
- **snooze**: the worker returns `{:snooze, 60}`. Oban puts the job off for at least 60 seconds
  and gives the attempt back, so that it doesn't count toward max_attempts.
- **queue**: the Oban queue that runs the job's attempts. It runs only jobs that are available.
- **operator**: a person who can cancel the job with `Oban.cancel_job/2`.

States of a job. A state that is not final is named for Oban's `state` and the job's `attempt`
together, as `<state>_<attempt>`; a final state is named for Oban's `state` alone. These are the
only states a job here can be in after a step:

- **available_0**: inserted and not yet run. Oban inserts a job meant to run now as `available`,
  with `attempt` 0 (A1). Every job starts here.
- **retryable_1**, **retryable_2**: the job's last attempt failed, and it has attempts left. It
  waits out its backoff. The number is how many of its attempts have failed.
- **scheduled_0**, **scheduled_1**, **scheduled_2**: the job's last attempt was rate limited, and
  it waits out its snooze. The number is how many of its attempts have failed.
- **completed**: the mail server accepted the email. Final.
- **cancelled**: the mail server bounced the email, or an operator cancelled the job. Final.
- **discarded**: the job's third attempt failed, and Oban will not run it again. Final.

Oban has other states. A job here is `executing`, or `available` with a count above 0, only for a
moment inside a step (section 2), and is never `suspended` (section 6).

Events of a job:

- **run**: the queue runs every job that is available now.
- **wait**: time passes until every backoff and every snooze is over; then the queue runs every
  job that is available.
- **cancel**: an operator cancels the job.

Variable:

- **answer**: what the mail server answers if the job's worker runs in this step: `accepted`,
  `failed`, `bounced` or `rate_limited`. In a step in which the worker doesn't run, the answer
  changes nothing.

## 2. Delivering one email

Each event is one step. When the queue runs the job, the job goes from available, through its
attempt, to the attempt's outcome within the step.

1. The app inserts the job to run now, so it starts as available_0. Oban: "`available` - The
   default state for new jobs that should be executed immediately"
   (https://oban.hexdocs.pm/2.24.2/job_lifecycle.html#initial-states).
2. **An attempt.** When the queue runs the job, Oban adds one to its `attempt` and calls the
   worker, which calls the mail server once (A1). The answer decides what the worker returns,
   and the return decides the outcome
   (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#t:result/0):
   - `accepted`: the worker returns `:ok`, and the job is completed.
   - `failed`: the worker returns `{:error, reason}`. If the count is now 1 or 2, the job is
     retryable with that count: retryable_1 or retryable_2. If it is 3, the job is discarded
     (A2).
   - `bounced`: the worker returns `{:cancel, reason}`, and the job is cancelled, whatever its
     count.
   - `rate_limited`: the worker returns `{:snooze, 60}`. The job is scheduled, and Oban takes
     the attempt back, so the count is what it was before the attempt (A3). The job is then
     scheduled_0 if it was available_0 or scheduled_0, scheduled_1 if it was retryable_1 or
     scheduled_1, and scheduled_2 if it was retryable_2 or scheduled_2.
3. **run.** The queue runs an available_0 job: one attempt, as in rule 2. A retryable or
   scheduled job is not available, because its backoff or snooze isn't over, so the queue
   doesn't run it. Nothing happens to the job, and that is not an error.
4. **wait.** Once every backoff and snooze is over, Oban makes a retryable or scheduled job
   available again with the same count (A1), and the queue runs it: one attempt, as in rule 2.
   An available_0 job is run the same way. Oban: "Once the backoff has elapsed Oban's job
   staging will transition them back to `:available`", and a scheduled job likewise "After the
   `scheduled_at` time has elapsed" (https://oban.hexdocs.pm/2.24.2/Oban.Job.html#states/0).
5. **Once per step.** In one step the worker runs at most once. A job that fails or snoozes in
   a step is not available again until its backoff, at least 30 seconds, or its snooze, 60
   seconds, is over. The step doesn't include that time; a later wait does. Oban: a failed job
   is "rescheduled until after a backoff period"
   (https://oban.hexdocs.pm/2.24.2/Oban.Job.html#states/0), and a snoozed job "is postponed
   for at least the specified period"
   (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-snoozing-jobs).
6. **cancel.** An operator's cancel makes the job cancelled at once, from any state that is not
   final, and the worker doesn't run (A4).
7. completed, cancelled and discarded are final. A job in one of them never runs again.

The outcome of a step in which the worker runs, by the job's state before the step and the
answer (rules 2 to 4). "stays" means that the worker ran and the job ends the step in the state
it started in.

| Before | Step | accepted | failed | bounced | rate_limited |
|---|---|---|---|---|---|
| available_0 | run or wait | completed | retryable_1 | cancelled | scheduled_0 |
| scheduled_0 | wait | completed | retryable_1 | cancelled | stays scheduled_0 |
| retryable_1 | wait | completed | retryable_2 | cancelled | scheduled_1 |
| scheduled_1 | wait | completed | retryable_2 | cancelled | stays scheduled_1 |
| retryable_2 | wait | completed | discarded | cancelled | scheduled_2 |
| scheduled_2 | wait | completed | discarded | cancelled | stays scheduled_2 |

In every other step the answer doesn't matter: run leaves a retryable or scheduled job as it is
(rule 3), and cancel makes any job that is not final cancelled (rule 6).

Oban's part and the app's. Oban decides what each state means, and the moves between them: a
job inserted to run now is available; a failed attempt makes it retryable, or discarded at its
limit; a `{:cancel, reason}` return or an operator's cancel makes it cancelled; a snooze makes
it scheduled and gives the attempt back; and a backoff or snooze that is over makes it available
again. The app decides which of these moves a job makes, and when: the worker's return for each
answer, the limit of 3, the backoff and snooze lengths, and that an operator may cancel. So it is
the app's choice that a bounce cancels the job, that a rate limit snoozes it, and that its third
failure, not a later one, discards it. completed records that the server accepted the email,
which can't be undone.

## 3. What must hold

- **D1. Bounded attempts.** A job's worker gets the answer `failed` at most three times. After
  the third, the job is discarded and never runs again.
- **D2. A snooze is not an attempt.** An attempt answered `rate_limited` doesn't count toward
  max_attempts. In every state that is not final, the job's `attempt` is the number of its
  attempts that failed.
- **D3. Retried until the limit.** A job whose attempt fails with attempts left, or is rate
  limited, runs again after its backoff or snooze, unless it is cancelled first.
- **D4. A bounce ends the job.** A job whose email bounces is cancelled at once, at any attempt,
  even with attempts left, and never runs again.
- **D5. Final is final.** A completed, cancelled or discarded job never runs again, so an email
  the server accepted is never sent again by its job.
- **D6. Never early.** The worker runs at most once per step, and a retryable or scheduled job
  never runs before its backoff or snooze is over.

## 4. Scenarios

Each scenario starts from a new job, available_0. A step is written as its event, followed by
the answer when the worker runs in it.

| ID | Steps | The job after each step |
|---|---|---|
| D01 | run accepted | completed |
| D02 | run failed, wait failed, wait accepted | retryable_1, retryable_2, completed |
| D03 | run failed, wait failed, wait failed | retryable_1, retryable_2, discarded |
| D04 | run bounced | cancelled |
| D05 | run failed, wait bounced | retryable_1, cancelled |
| D06 | run rate_limited, wait accepted | scheduled_0, completed |
| D07 | run rate_limited, wait failed, wait failed | scheduled_0, retryable_1, retryable_2 |
| D08 | run failed, wait rate_limited, wait failed | retryable_1, scheduled_1, retryable_2 |
| D09 | run failed, wait failed, wait rate_limited | retryable_1, retryable_2, scheduled_2 |
| D10 | run rate_limited, wait rate_limited | scheduled_0, scheduled_0 |
| D11 | run failed, run | retryable_1, retryable_1 |
| D12 | cancel | cancelled |
| D13 | run failed, cancel | retryable_1, cancelled |
| D14 | run rate_limited, cancel | scheduled_0, cancelled |

What they show:

- D01 to D03: an email is sent, retried, or given up after three failed attempts; there is
  never a fourth.
- D04 and D05: a bounce cancels the job at once, though attempts were left.
- D06 to D09: a snooze doesn't use up an attempt. D07 and D08 run the worker three times and the
  job is still retryable; D09 snoozes at the limit and is not discarded.
- D10: the worker runs in both steps, and the job is scheduled_0 after each.
- D11: the second run doesn't run the worker, because the backoff isn't over.
- D12 to D14: cancel ends the job at once, before any attempt, during a backoff or during a
  snooze, and the worker doesn't run again.

## 5. Assumptions

Oban's documentation for 2.24.2 is this chapter's only source on Oban. Where it doesn't settle a
point, the chapter assumes the following, and says what in the documentation led to it.

- **A1. How `attempt` counts.** The docs don't say what `attempt` is when a job is inserted, nor
  when it changes. The chapter assumes it is 0 at insertion and goes up by one as each attempt
  starts, and by nothing else, so it is 1 during and after the first attempt, and making a job
  available again leaves it alone. The docs point that way without saying it:
  - the job's `attempt` is a `non_neg_integer()`, but a recorded error's `attempt` is a
    `pos_integer()` (https://oban.hexdocs.pm/2.24.2/Oban.Job.html#t:t/0,
    https://oban.hexdocs.pm/2.24.2/Oban.Job.html#t:errors/0), "The attempt number when the
    error occurred" (https://oban.hexdocs.pm/2.24.2/error_handling.html#error-details);
  - the linear backoff, which delays "for the same number of seconds as the current attempt",
    starts its example at attempt 1, so `backoff/1` sees 1 after the first failure
    (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-customizing-backoff);
  - `perform/1` can act on the job's "number of execution attempts"
    (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-defining-workers).

  Only the count after a step shows here, so where in a step it goes up, whether when the job is
  made available, when the attempt starts or when it ends, makes no difference to the states.
- **A2. What max_attempts limits.** The docs contradict themselves twice.
  - Attempts or retries. "Integer specifying how many times the job will be retried"
    (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-defining-workers), "the maximum
    number of times a job can be retried if there are errors during execution"
    (https://oban.hexdocs.pm/2.24.2/Oban.Job.html#new/2) and "By default, jobs are retried up
    to 20 times" (https://oban.hexdocs.pm/2.24.2/error_handling.html#limiting-retries) would
    allow a first attempt and 3 retries. But "to instruct a worker to discard jobs after three
    failures", the same guide sets `max_attempts: 3`; and with the default of 20, "the 20th
    attempt will occur 6 days and 16 hours after the 19th attempt"
    (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-customizing-backoff). These allow
    3 attempts in all.
  - At the limit. "Jobs that fail and haven't exceeded their max attempts are transitioned to
    `:retryable`" (https://oban.hexdocs.pm/2.24.2/Oban.Job.html#states/0) would retry a job
    whose third attempt fails. But "If `max_attempts` has not been reached already, the job is
    marked as `retryable` [...] Otherwise, the job is marked as `discarded`"
    (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-defining-workers), "The job failed
    and reached its maximum retry attempts"
    (https://oban.hexdocs.pm/2.24.2/job_lifecycle.html#final-states) and "If the number of
    failures reaches `max_attempts`, the job gets discarded"
    (https://oban.hexdocs.pm/2.24.2/error_handling.html#retries) discard it.

  The chapter assumes 3 attempts in all: a failure at attempt 1 or 2 makes the job retryable,
  and a failure at attempt 3 discards it.
- **A3. What a snooze does.** Two points.
  - The state. `{:snooze, period}` will "mark the job as `snoozed` and schedule it to run again
    after the specified period"
    (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-defining-workers), but the same
    module's `result()` type says "mark the job as `scheduled` to run again after the specified
    period" (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#t:result/0), and `snoozed` is
    none of the states: "Jobs exist in one of eight possible states"
    (https://oban.hexdocs.pm/2.24.2/job_lifecycle.html#job-states,
    https://oban.hexdocs.pm/2.24.2/Oban.Job.html#states/0). The Job Lifecycle guide doesn't
    mention snoozing, and gives no move from `executing` to `scheduled`. The chapter assumes
    the job is `scheduled`.
  - The count. "Snoozing schedules execution for the specified period in the future and rolls
    back the job's `attempt`. The original `max_attempts` is preserved, so a snooze never
    consumes an attempt" (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-snoozing-jobs;
    likewise https://oban.hexdocs.pm/2.24.2/changelog.html). The docs don't say how far it
    rolls back. The chapter assumes by one, to the count before the attempt began, so that a
    snooze at the third attempt leaves the job scheduled_2, not discarded.
- **A4. What cancel does.** `Oban.cancel_job/2` will "Cancel an `executing`, `available`,
  `scheduled` or `retryable` job and mark it as `cancelled` to prevent it from running. If the
  job is currently `executing` it will be killed and otherwise it is ignored"
  (https://oban.hexdocs.pm/2.24.2/Oban.html#cancel_job/2). The last sentence can be read as
  "a job in any other state is left alone" or as "nothing is killed". The chapter reads it the
  first way. Either way, every state here that is not final is available, scheduled or
  retryable, so cancel always cancels; the reading matters only for final states, which take no
  events here.
- **A5. The SQLite engine.** The page for `Oban.Engines.Lite` says only how to start it
  (https://oban.hexdocs.pm/2.24.2/Oban.Engines.Lite.html). The chapter assumes it behaves as the
  rest of the docs describe Oban: "Each engine supports the same core functionality, though they
  have differing levels of maturity and suitability for production"
  (https://oban.hexdocs.pm/2.24.2/Oban.html#module-engines).

## 6. Not covered

- Retrying a job by hand, with `Oban.retry_job/2`.
- A job seen while `executing`, and cancelling a running job.
- The `suspended` state, and jobs inserted as scheduled.
- Several nodes or queues, unique jobs, and rescuing jobs from a crashed node.
- Postgres and Oban Pro.
- Time-outs.
- A worker that raises, exits or throws instead of returning.
- Pruning or deleting the job's row.

## 7. What the lifecycle does not show

The lifecycle shows the job's state and count after each step, and nothing else.

- How long a backoff or a snooze lasts. A wait waits out any length. The 30 times `attempt`
  seconds of `backoff/1` and the 60 seconds of the snooze are checked separately, against the
  worker's `backoff/1` and its snooze.
- How often the worker ran. A snooze gives its attempt back, so the count doesn't show it. Oban
  keeps a `snoozed` count in the job's `meta` instead
  (https://oban.hexdocs.pm/2.24.2/Oban.Worker.html#module-snoozing-jobs), outside the states.
- That a job may never end. Nothing in the app limits snoozes, so a job rate limited at every
  wait stays scheduled for ever (D10 shows one round of it). The lifecycle allows this, and no
  rule here forbids it.
- The rest of the row: the recorded errors, the timestamps, `meta`, and the reasons given with
  `{:error, reason}` and `{:cancel, reason}`.
- What happens inside a step: the job's moment as `executing`, or as `available` with a count
  above 0 during a wait.
- What a cancel does to a final job. The docs, as A4 reads them, leave it alone, but final
  states take no events here.
