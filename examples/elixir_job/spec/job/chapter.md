# job: running a background job with retries

A service runs background jobs, such as sending an email or resizing an image, outside the
request that asked for them. An attempt at a job can fail, and the job is then tried again, up
to a limit. Someone can cancel a job at any point before it ends. This chapter says how one job
moves from being enqueued to its outcome.

## 1. Glossary

The rest of this chapter uses only these names for the states, events and variable of a job.

Things:

- **job**: one piece of work and its arguments, enqueued once and never changed after.
- **worker**: the code that does a job's work.
- **attempt**: one run of the worker on a job. A job's attempts are numbered from 1.
- **maximum attempts**: how many attempts a job may have, at least 1, fixed when it is enqueued.
- **backoff**: the wait after a failed attempt before the job is queued again.

States of a job:

- **queued**: the job waits for a worker to start its next attempt.
- **running**: a worker is running an attempt at the job.
- **retrying**: an attempt failed and the job may have another; it waits out its backoff.
- **succeeded**: an attempt finished without error. Final.
- **discarded**: the job will never run again, because its last allowed attempt failed or
  because it was cancelled. Final.

Events of a job:

- **start**: a worker takes the job and starts an attempt.
- **succeed**: the attempt finishes without error.
- **fail**: the attempt ends with an error, or the worker crashes during it.
- **retry_due**: the backoff after a failed attempt is over.
- **cancel**: someone cancels the job.

Variable:

- **attempts**: whether the job's current attempt is the last its maximum attempts allow:
  `last` if it is, `some_left` if not. The current attempt is the one running; for a queued or
  retrying job it is the next one the job would start.

## 2. Running one job

1. A job is enqueued as queued, before its first attempt.
2. A worker starts an attempt only at a queued job, and the job is then running. A job never has
   two attempts running at once.
3. If the attempt succeeds, the job has succeeded.
4. If the attempt fails and it was not the job's last allowed attempt, the job is retrying.
   When its backoff is over, it is queued again for its next attempt.
5. If the attempt fails and it was the job's last allowed attempt, the job is discarded.
6. A queued, running or retrying job that is cancelled is discarded at once. A running attempt
   is stopped, and nothing it reports afterwards changes the job.
7. Any other event is refused: start outside queued, succeed and fail outside running, and
   retry_due outside retrying.

## 3. What must hold

- **J1. Bounded attempts.** A job never has more attempts than its maximum attempts.
- **J2. Retried until the limit.** A job whose attempt fails, and that has attempts left, is
  attempted again unless it is cancelled. A job whose last allowed attempt fails is never
  attempted again.

## 4. Scenarios

| ID | Scenario | What must happen |
|---|---|---|
| J01 | A job with maximum attempts 3 succeeds at its first attempt | Succeeded after 1 attempt |
| J02 | Maximum 3; the first two attempts fail and the third succeeds | Retrying after each failure; succeeded after 3 attempts |
| J03 | Maximum 3; all three attempts fail | Discarded after the third attempt; never a fourth |
| J04 | Maximum 1; the attempt fails | Discarded at once, never retrying |
| J05 | A queued job is cancelled before any attempt | Discarded; no attempt runs |
| J06 | A running job is cancelled, and its attempt then reports success | Discarded; the success changes nothing |
| J07 | A retrying job is cancelled | Discarded; no further attempt |

## 5. Assumptions

- The service keeps a job's state and attempt count, and does not lose them.
- Each event reaches the job once, in the order it happened.

## 6. Not covered

- How long a backoff lasts, time-outs, and jobs scheduled for later.
- Priorities, queues and how many jobs run at once.
- A service that crashes while a job is running, and recovering such a job.
- Retrying a succeeded or discarded job by hand.

## 7. What the lifecycle does not show

The lifecycle takes `attempts` as given. Whether a job counts its attempts and compares them
with its maximum correctly is not part of it; checks of J1 and J2 against the scenarios cover
that.
