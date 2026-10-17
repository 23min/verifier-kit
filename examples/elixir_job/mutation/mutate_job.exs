# Checks the checker: break the job on purpose and confirm the oracle notices.
#
# Each mutant is one small edit to src/job.ex, in a copy of the example with that edit made. A
# mutant is killed when conformance or the content checks fail on it: each exits 1, through the
# same commands ./verify runs, conformance/adapter_job.cmd and checks/run.cmd. A surviving
# mutant marks behavior nothing in the oracle describes; a content-check run that finds no
# checks exits 2 and notices nothing.
#
# Run from the example's root, with VERIFY_SPINE set to the command that runs spine, as
# ./verify runs mutation/mutate_job.cmd.

defmodule Mutate do
  @mutants [
    {"a last failure is retried", "def transition(:running, :fail, :last), do: {:ok, :discarded}",
     "def transition(:running, :fail, :last), do: {:ok, :retrying}"},
    {"cancel is refused while running", "when state in [:queued, :running, :retrying]",
     "when state in [:queued, :retrying]"},
    {"one attempt too many", "if current == job.max_attempts", "if current > job.max_attempts"},
    {"start does not count an attempt",
     "attempts = if event == :start, do: job.attempts + 1, else: job.attempts",
     "attempts = job.attempts"},
    {"a running attempt counts as the next",
     "current = if job.state in [:queued, :retrying], do: job.attempts + 1, else: job.attempts",
     "current = job.attempts + 1"}
  ]

  def main do
    [spine | spine_args] = OptionParser.split(System.fetch_env!("VERIFY_SPINE"))
    adapter = File.read!("conformance/adapter_job.cmd") |> String.trim_trailing()

    [checks | checks_args] =
      File.read!("checks/run.cmd") |> String.trim_trailing() |> OptionParser.split()

    source = File.read!("src/job.ex")

    oracle = [
      {"conformance",
       {spine, spine_args ++ ["conform", "spec/job/spine.yaml", "--adapter-cmd", adapter]}},
      {"checks", {checks, checks_args}}
    ]

    # On the job as it is, conformance must pass and the checks pass or find none, or every
    # mutant would look caught.
    case run(oracle, copy_with(source)) do
      [{"conformance", 0}, {"checks", checks}] when checks in [0, 2] ->
        survivors = Enum.count(@mutants, &(not killed?(&1, source, oracle)))
        IO.puts("-- #{survivors} errors, #{length(@mutants)} mutants")
        System.halt(if survivors > 0, do: 1, else: 0)

      statuses ->
        line("error", "oracle-fails", "on the job as it is: #{inspect(statuses)}")
        IO.puts("-- 1 errors, 0 mutants")
        System.halt(1)
    end
  end

  # Each oracle command's exit status in a copy, which is then removed.
  defp run(oracle, tmp) do
    statuses =
      for {label, {cmd, args}} <- oracle,
          do: {label, elem(System.cmd(cmd, args, cd: tmp, stderr_to_stdout: true), 1)}

    File.rm_rf!(tmp)
    statuses
  end

  defp killed?({name, find, replace}, source, oracle) do
    case String.split(source, find) do
      [before, rest] ->
        statuses = run(oracle, copy_with(before <> replace <> rest))
        report(name, for({label, 1} <- statuses, do: label))

      _ ->
        line("error", "mutant-stale", "#{name}: anchor not found once in src/job.ex")
        false
    end
  end

  # A copy of the example's spec/, conformance/, checks/ and src/, with src/job.ex as given.
  defp copy_with(job) do
    tmp = Path.join(System.tmp_dir!(), "mutate_job_#{System.unique_integer([:positive])}")
    File.mkdir_p!(tmp)
    for dir <- ["spec", "conformance", "checks", "src"], do: File.cp_r!(dir, Path.join(tmp, dir))
    File.write!(Path.join(tmp, "src/job.ex"), job)
    tmp
  end

  defp report(name, []) do
    line("error", "mutant-survived", "#{name}: no part of the oracle noticed")
    false
  end

  defp report(name, killed_by) do
    line("info", "mutant-killed", "#{name}: caught by #{Enum.join(killed_by, ", ")}")
    true
  end

  defp line(severity, code, text),
    do:
      IO.puts(
        String.pad_trailing(severity, 7) <> " " <> String.pad_trailing(code, 18) <> " " <> text
      )
end

Mutate.main()
