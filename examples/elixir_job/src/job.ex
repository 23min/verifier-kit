defmodule Job.Lifecycle do
  def transition(:queued, :start, _attempts), do: {:ok, :running}
  def transition(:running, :succeed, _attempts), do: {:ok, :succeeded}
  def transition(:running, :fail, :some_left), do: {:ok, :retrying}
  def transition(:running, :fail, :last), do: {:ok, :discarded}
  def transition(:retrying, :retry_due, _attempts), do: {:ok, :queued}

  def transition(state, :cancel, _attempts) when state in [:queued, :running, :retrying],
    do: {:ok, :discarded}

  def transition(_state, _event, _attempts), do: :refused
end

defmodule Job do
  defstruct state: :queued, attempts: 0, max_attempts: 1

  def new(max_attempts) when is_integer(max_attempts) and max_attempts >= 1,
    do: %__MODULE__{max_attempts: max_attempts}

  def state(%__MODULE__{state: state}), do: state
  def attempts(%__MODULE__{attempts: attempts}), do: attempts

  def start(job), do: transition(job, :start)
  def succeed(job), do: transition(job, :succeed)
  def fail(job), do: transition(job, :fail)
  def retry_due(job), do: transition(job, :retry_due)
  def cancel(job), do: transition(job, :cancel)

  defp transition(%__MODULE__{} = job, event) do
    # Queued and retrying jobs refer to their next attempt, not the previous one.
    current = if job.state in [:queued, :retrying], do: job.attempts + 1, else: job.attempts
    remaining = if current == job.max_attempts, do: :last, else: :some_left

    case Job.Lifecycle.transition(job.state, event, remaining) do
      {:ok, state} ->
        attempts = if event == :start, do: job.attempts + 1, else: job.attempts
        {:ok, %{job | state: state, attempts: attempts}}

      :refused ->
        :refused
    end
  end
end
