# The job against the chapter (spec/job/chapter.md): scenarios J01-J07, and J1 and J2 in every
# job reachable from Job.new/1 with maximum attempts 1 to 4. The lifecycle takes attempts as
# given; these checks are where the job's own counting is checked.
#
# covers: queued/start@4252715ad208
# covers: running/succeed@77b506c05f03
# covers: running/fail@e91c699d5c60
# covers: retrying/retry_due@49f46d690e73
# covers: queued/cancel@58794c90f1ff
# covers: running/cancel@f486dfca49fa
# covers: retrying/cancel@23d7efa55bab

defmodule JobTest do
  use ExUnit.Case, async: true

  @events [:start, :succeed, :fail, :retry_due, :cancel]
  @retry [:start, :fail, :retry_due]

  # Applies the events in order to a new job; fails naming the first one refused.
  defp run(max, events) do
    events
    |> Enum.with_index(1)
    |> Enum.reduce(Job.new(max), fn {event, i}, job ->
      case apply(Job, event, [job]) do
        {:ok, next} -> next
        :refused -> flunk("step #{i}, #{event}, is refused in #{Job.state(job)}")
      end
    end)
  end

  defp states(max, events) do
    for n <- 1..length(events), do: Job.state(run(max, Enum.take(events, n)))
  end

  defp outcome(job), do: {Job.state(job), Job.attempts(job)}

  test "J01: maximum 3; the first attempt succeeds" do
    assert outcome(run(3, [:start, :succeed])) == {:succeeded, 1}
  end

  test "J02: maximum 3; two attempts fail and the third succeeds" do
    events = @retry ++ @retry ++ [:start, :succeed]

    assert states(3, events) ==
             [:running, :retrying, :queued, :running, :retrying, :queued, :running, :succeeded]

    assert outcome(run(3, events)) == {:succeeded, 3}
  end

  test "J03: maximum 3; all three attempts fail, and there is never a fourth" do
    job = run(3, @retry ++ @retry ++ [:start, :fail])
    assert outcome(job) == {:discarded, 3}
    assert Job.start(job) == :refused
  end

  test "J04: maximum 1; the attempt fails and the job is discarded, never retrying" do
    job = run(1, [:start, :fail])
    assert outcome(job) == {:discarded, 1}
    assert Job.retry_due(job) == :refused
    assert Job.start(job) == :refused
  end

  test "J05: a queued job is cancelled before any attempt" do
    job = run(3, [:cancel])
    assert outcome(job) == {:discarded, 0}
    assert Job.start(job) == :refused
  end

  test "J06: a running job is cancelled, and its attempt then reports success" do
    job = run(3, [:start, :cancel])
    assert Job.succeed(job) == :refused
    assert outcome(job) == {:discarded, 1}
  end

  test "J07: a retrying job is cancelled" do
    job = run(3, [:start, :fail, :cancel])
    assert outcome(job) == {:discarded, 1}
    assert Job.retry_due(job) == :refused
    assert Job.start(job) == :refused
  end

  for max <- 1..4 do
    test "J1 and J2 in every job reachable with maximum attempts #{max}" do
      explore(unquote(max), [Job.new(unquote(max))], MapSet.new())
    end
  end

  # Breadth-first over every job reachable by accepted events, checking each step taken.
  defp explore(_max, [], _seen), do: :ok

  defp explore(max, [job | rest], seen) do
    if MapSet.member?(seen, job) do
      explore(max, rest, seen)
    else
      nexts =
        for event <- @events, {:ok, next} <- [apply(Job, event, [job])] do
          check_step(max, job, event, next)
          next
        end

      explore(max, rest ++ nexts, MapSet.put(seen, job))
    end
  end

  defp check_step(max, job, event, next) do
    before = Job.attempts(job)
    counted = if event == :start, do: before + 1, else: before
    where = "#{event} from #{Job.state(job)} after #{before} of #{max} attempts"

    assert Job.attempts(next) == counted,
           "#{where}: #{Job.attempts(next)} attempts, expected #{counted}"

    assert Job.attempts(next) <= max, "J1: #{where} makes #{Job.attempts(next)} attempts"

    if event == :fail and before < max do
      assert Job.state(next) == :retrying, "J2: #{where} ends #{Job.state(next)}, not retrying"
      assert outcome(again(next)) == {:running, before + 1}, "J2: #{where} is not attempted again"
    end

    if event == :fail and before == max do
      assert Job.state(next) == :discarded, "J2: #{where} ends #{Job.state(next)}, not discarded"
    end
  end

  defp again(job) do
    {:ok, job} = Job.retry_due(job)
    {:ok, job} = Job.start(job)
    job
  end
end
