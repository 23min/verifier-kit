# Connects the spine to the app and to real Oban, over the adapter protocol
# (docs/reference/tool-contract.md): one JSON request per line on stdin, one JSON reply per line
# on stdout. One process, one database and one Oban serve the whole conform run.
#
# Owned by the oracle side: the builder can read it but not change it.
# It only translates. It never decides whether an event is allowed.
#
#   new      -> removes every job, then sends an email, so one job is available_0
#   apply    -> run:    the mail server will give `answer`; the queue runs its available jobs
#               wait:   the same, after every backoff and snooze is over
#               cancel: an operator cancels the job
#               Oban refuses none of them, so each replies "ok"
#   observe  -> the job's row: <state>_<attempt>, or the state alone when it is final
#
# What src/ must offer, in .ex files under src/. The checks in checks/delivery/ use the same
# names, so this is the whole contract between the oracle and the app.
#
#   Mailer.start(opts) -> :ok, once: starts the app with a SQLite database at opts[:database],
#             the mail server opts[:server], a module whose deliver(to) answers :accepted,
#             :failed, :bounced or :rate_limited, and opts[:oban] added to Oban's options
#   Mailer.send_email(to) -> {:ok, job}: inserts the job that sends it, to run now, in queue :mail
#   Mailer.cancel(job_id) -> :ok: an operator's cancel
#   Mailer.Repo -> the app's Ecto repo, which holds Oban's jobs
#   src/deps.exs -> installs the Hex packages src/ uses, pinned by src/mix.lock; the command
#             lines here and in checks/ load it first
#
# Time doesn't pass here: Oban runs in testing: :manual, and the queue runs only when the
# adapter drains it, with with_scheduled: true for a wait.

defmodule FakeServer do
  def deliver(_to), do: :persistent_term.get(__MODULE__)
end

defmodule Adapter do
  @answers %{
    "accepted" => :accepted,
    "failed" => :failed,
    "bounced" => :bounced,
    "rate_limited" => :rate_limited
  }
  @final ["completed", "cancelled", "discarded"]

  def serve(id) do
    case IO.read(:stdio, :line) do
      :eof ->
        :ok

      {:error, reason} ->
        raise "cannot read stdin: #{inspect(reason)}"

      line ->
        {reply, id} = handle(JSON.decode(line), id)
        IO.write(JSON.encode!(reply) <> "\n")
        serve(id)
    end
  end

  defp handle({:ok, %{"op" => "new"}}, _id) do
    {:ok, _} = Oban.delete_all_jobs(Oban.Job)
    {:ok, job} = Mailer.send_email("someone@example.com")
    {%{ok: true}, job.id}
  end

  defp handle({:ok, %{"op" => "apply", "event" => event, "env" => %{"answer" => answer}}}, id)
       when is_map_key(@answers, answer) do
    # A raise is a divergence, as an exception from a Python adapter's apply() is.
    try do
      :persistent_term.put(FakeServer, @answers[answer])

      case event do
        "run" -> Oban.drain_queue(queue: :mail)
        "wait" -> Oban.drain_queue(queue: :mail, with_scheduled: true)
        "cancel" -> :ok = Mailer.cancel(id)
      end

      {%{result: "ok"}, id}
    rescue
      e -> {%{error: Exception.format_banner(:error, e, __STACKTRACE__)}, id}
    end
  end

  defp handle({:ok, %{"op" => "observe"}}, id) when is_integer(id) do
    job = Mailer.Repo.get!(Oban.Job, id)
    {%{state: if(job.state in @final, do: job.state, else: "#{job.state}_#{job.attempt}")}, id}
  end

  defp handle(request, id),
    do: {%{error: "cannot handle #{inspect(request)} with job #{inspect(id)}"}, id}
end

# Logs go to stderr, so that stdout holds only the protocol.
:logger.update_handler_config(:default, :config, %{type: :standard_error})
Logger.configure(level: :warning)

:ok =
  Mailer.start(
    database: ":memory:",
    server: FakeServer,
    oban: [queues: [], plugins: false, testing: :manual]
  )

Adapter.serve(nil)
