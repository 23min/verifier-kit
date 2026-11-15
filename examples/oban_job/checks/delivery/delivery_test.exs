# What the lifecycle doesn't show (spec/delivery/chapter.md, section 7): how long a backoff and a
# snooze last, that a snooze leaves max_attempts alone, and that the job sends the email to its
# address. conform checks the lifecycle itself, and how often each step sends, on every sequence.
#
# covers: available_0/run@f78bee4d3801
# covers: available_0/wait@fdb15d3155e4
# covers: retryable_1/wait@3749a1391c05
# covers: retryable_2/wait@c6bca55b2b81
# covers: scheduled_0/wait@c73ba498bd73
# covers: scheduled_1/wait@1f8f1166d405
# covers: scheduled_2/wait@0a4ce4ce4cd6

defmodule Server do
  # The mail server: answers as the check says, and keeps each address it is sent to.
  def deliver(to) do
    :persistent_term.put({__MODULE__, :sent}, sent() ++ [to])
    :persistent_term.get({__MODULE__, :answer})
  end

  def sent, do: :persistent_term.get({__MODULE__, :sent}, [])
end

defmodule DeliveryTest do
  use ExUnit.Case, async: false

  setup_all do
    # Only the findings and the tally, not Ecto's queries.
    Logger.configure(level: :warning)

    Mailer.start(
      database: ":memory:",
      server: Server,
      oban: [queues: [], plugins: false, testing: :manual]
    )
  end

  setup do
    {:ok, _} = Oban.delete_all_jobs(Oban.Job)
    :persistent_term.put({Server, :sent}, [])
    :ok
  end

  # One step: the server will give `answer`, and the queue runs; with the waits over if `wait`.
  defp step(job, answer, wait) do
    :persistent_term.put({Server, :answer}, answer)
    Oban.drain_queue(queue: :mail, with_scheduled: wait)
    Mailer.Repo.get!(Oban.Job, job.id)
  end

  # How long the job waits after its last attempt, in seconds.
  defp waits(job), do: DateTime.diff(job.scheduled_at, job.attempted_at)

  test "a failed attempt waits 30 seconds times its attempt" do
    {:ok, job} = Mailer.send_email("a@example.com")
    job = step(job, :failed, false)
    assert {job.state, job.attempt} == {"retryable", 1}
    assert waits(job) in 29..31
    job = step(job, :failed, true)
    assert {job.state, job.attempt} == {"retryable", 2}
    assert waits(job) in 59..61
  end

  test "a rate-limited attempt waits 60 seconds and leaves max_attempts at 3, at every attempt" do
    {:ok, job} = Mailer.send_email("a@example.com")
    job = step(job, :rate_limited, false)
    assert {job.state, job.attempt, job.max_attempts} == {"scheduled", 0, 3}
    assert waits(job) in 59..61
    job = step(job, :failed, true)
    job = step(job, :failed, true)
    job = step(job, :rate_limited, true)
    assert {job.state, job.attempt, job.max_attempts} == {"scheduled", 2, 3}
    assert waits(job) in 59..61
  end

  test "the job sends to its address" do
    {:ok, job} = Mailer.send_email("a@example.com")
    job = step(job, :failed, false)
    job = step(job, :accepted, true)
    assert job.state == "completed"
    assert Enum.uniq(Server.sent()) == ["a@example.com"]
  end
end
