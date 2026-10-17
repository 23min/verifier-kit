# Connects the spine to the job, over the adapter protocol (docs/reference/tool-contract.md):
# one JSON request per line on stdin, one JSON reply per line on stdout.
#
# Owned by the oracle side: the builder can read it but not change it.
# It only translates. It never decides whether an event is allowed.
#
#   new      -> a fresh job, Job.new(3); its state is read from the job
#   apply    -> hands one spine event, with the value of attempts, to the job's lifecycle;
#               replies "ok" or "rejected"
#   observe  -> the job's state, as the lifecycle last returned it
#
# What src/ must offer, in .ex files under src/. The checks in checks/job/ use the same names,
# so this is the whole contract between the oracle and the job.
#
# The lifecycle, a pure function:
#
#   Job.Lifecycle.transition(state, event, attempts) -> {:ok, next_state} or :refused
#             state and event are the spine's names as atoms (spec/job/spine.yaml); attempts is
#             :last or :some_left, as the chapter's glossary defines it.
#
# The job, which counts its own attempts and asks the lifecycle what each event does:
#
#   Job.new(max_attempts) -> a job: queued, no attempts started; max_attempts is an integer, 1 or more
#   Job.start(job), Job.succeed(job), Job.fail(job), Job.retry_due(job), Job.cancel(job)
#             -> {:ok, job} or :refused. Each works out attempts from the job's own count and
#             maximum, and calls Job.Lifecycle.transition/3 for the next state.
#   Job.state(job) -> the job's state, one of the spine's names as an atom
#   Job.attempts(job) -> how many attempts the job has started
#
# Constraints, so the checks can explore every job: src/ is pure and deterministic (no
# processes, clocks, randomness or I/O), a job is plain data that can be compared with ==, and
# src/ uses only Elixir's standard library and nothing from conformance/ or checks/.

defmodule Adapter do
  @events %{
    "start" => :start,
    "succeed" => :succeed,
    "fail" => :fail,
    "retry_due" => :retry_due,
    "cancel" => :cancel
  }
  @attempts %{"last" => :last, "some_left" => :some_left}

  def serve(state) do
    case IO.read(:stdio, :line) do
      :eof ->
        :ok

      {:error, reason} ->
        raise "cannot read stdin: #{inspect(reason)}"

      line ->
        {reply, state} = handle(JSON.decode(line), state)
        IO.write(JSON.encode!(reply) <> "\n")
        serve(state)
    end
  end

  defp handle({:ok, %{"op" => "new"}}, _state), do: {%{ok: true}, Job.state(Job.new(3))}

  defp handle(
         {:ok, %{"op" => "apply", "event" => event, "env" => %{"attempts" => attempts}}},
         state
       )
       when is_map_key(@events, event) and is_map_key(@attempts, attempts) do
    # A raise is a divergence, as an exception from a Python adapter's apply() is.
    try do
      case Job.Lifecycle.transition(state, @events[event], @attempts[attempts]) do
        {:ok, next} -> {%{result: "ok"}, next}
        :refused -> {%{result: "rejected"}, state}
        other -> {%{error: "transition returned #{inspect(other)}"}, state}
      end
    rescue
      e -> {%{error: Exception.format_banner(:error, e, __STACKTRACE__)}, state}
    end
  end

  defp handle({:ok, %{"op" => "observe"}}, state) when is_atom(state) and not is_nil(state),
    do: {%{state: Atom.to_string(state)}, state}

  defp handle(request, state),
    do: {%{error: "cannot handle #{inspect(request)} in #{inspect(state)}"}, state}
end

Adapter.serve(nil)
