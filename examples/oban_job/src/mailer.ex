# The app under test (spec/delivery/chapter.md): it sends each email from an Oban job, on SQLite.
# The mail server is whatever module Mailer.start/1 is given, with deliver(to) answering
# :accepted, :failed, :bounced or :rate_limited.

defmodule Mailer.Repo do
  use Ecto.Repo, otp_app: :mailer, adapter: Ecto.Adapters.SQLite3
end

defmodule Mailer.Migration do
  use Ecto.Migration

  def up, do: Oban.Migration.up()
  def down, do: Oban.Migration.down()
end

defmodule Mailer do
  # Starts the app: its database at opts[:database], Oban's tables in it, and Oban, with
  # opts[:oban] added to Oban's options. opts[:server] is the mail server.
  def start(opts) do
    Application.put_env(:mailer, :server, Keyword.fetch!(opts, :server))

    Application.put_env(:mailer, Mailer.Repo,
      database: Keyword.fetch!(opts, :database),
      pool_size: 1
    )

    {:ok, _} = Mailer.Repo.start_link()
    Ecto.Migrator.run(Mailer.Repo, [{0, Mailer.Migration}], :up, all: true, log: false)

    {:ok, _} =
      Oban.start_link(
        [repo: Mailer.Repo, engine: Oban.Engines.Lite] ++ Keyword.get(opts, :oban, [])
      )

    :ok
  end

  def server, do: Application.fetch_env!(:mailer, :server)

  # Sends an email to `to`, from a job inserted to run now: {:ok, job}.
  def send_email(to), do: Oban.insert(Mailer.Worker.new(%{"to" => to}))

  # An operator cancels the job.
  def cancel(job_id), do: Oban.cancel_job(job_id)
end

defmodule Mailer.Worker do
  use Oban.Worker, queue: :mail, max_attempts: 3

  @impl Oban.Worker
  def perform(%Oban.Job{args: %{"to" => to}}) do
    case Mailer.server().deliver(to) do
      :accepted -> :ok
      :failed -> {:error, :failed}
      :bounced -> {:cancel, :bounced}
      :rate_limited -> {:snooze, 60}
    end
  end

  @impl Oban.Worker
  def backoff(%Oban.Job{attempt: attempt}), do: 30 * attempt
end
