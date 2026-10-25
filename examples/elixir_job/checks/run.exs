# Runs the content checks in checks/job/ with ExUnit, and reports them as tools/run_checks.py
# does: a check-failed line for each check that fails, with the first line of why; a
# check-unloadable line for each file that can't be loaded; no-checks when no check ran; then
# the tally. Exits 0 when at least one check ran and every one passed, 1 otherwise.

defmodule Findings do
  # An ExUnit formatter that prints a finding line for each failing check, sorted by file and
  # check as run_checks.py sorts them, once every check has run; and nothing else.
  use GenServer

  def line(code, where, message),
    do:
      IO.puts(
        String.pad_trailing("error", 7) <>
          " " <> String.pad_trailing(code, 18) <> " " <> "#{where}: #{message}"
      )

  def first_line(text), do: text |> String.split("\n") |> Enum.find("", &(String.trim(&1) != ""))

  @impl true
  def init(_), do: {:ok, []}

  @impl true
  def handle_cast(
        {:test_finished, %ExUnit.Test{state: {:failed, [{kind, reason, stack} | _]}} = test},
        state
      ) do
    why =
      if kind == :error and is_exception(reason),
        do: "#{inspect(reason.__struct__)}: #{first_line(Exception.message(reason))}",
        else: first_line(Exception.format_banner(kind, reason, stack))

    {:noreply, [{Path.relative_to_cwd(test.tags.file), "#{test.name}", why} | state]}
  end

  def handle_cast({:suite_finished, _times}, state) do
    for {file, name, why} <- Enum.sort(state), do: line("check-failed", "#{file}::#{name}", why)
    {:noreply, []}
  end

  def handle_cast(_event, state), do: {:noreply, state}
end

ExUnit.start(autorun: false, formatters: [Findings])

unloadable =
  __DIR__
  |> Path.join("job/*_test.exs")
  |> Path.wildcard()
  |> Enum.sort()
  |> Enum.count(fn file ->
    try do
      Code.require_file(file)
      false
    rescue
      e ->
        Findings.line(
          "check-unloadable",
          Path.relative_to_cwd(file),
          "cannot be loaded: #{inspect(e.__struct__)}: #{Findings.first_line(Exception.message(e))}"
        )

        true
    end
  end)

%{total: total, failures: failures, excluded: excluded, skipped: skipped} = ExUnit.run()
ran = total - excluded - skipped
none = if ran == 0, do: 1, else: 0
if none == 1, do: Findings.line("no-checks", "checks/job", "no checks found")
errors = failures + unloadable + none
IO.puts("-- #{errors} errors, #{ran} checks run")
System.halt(if errors > 0, do: 1, else: 0)
