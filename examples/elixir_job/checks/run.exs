# Runs the content checks in checks/job/ with ExUnit. Exits 0 only when at least one check ran
# and every check passed; a run that finds no checks fails.
ExUnit.start(autorun: false)

__DIR__
|> Path.join("job/*_test.exs")
|> Path.wildcard()
|> Enum.sort()
|> Enum.each(&Code.require_file/1)

%{total: total, failures: failures, excluded: excluded, skipped: skipped} = ExUnit.run()

cond do
  total - excluded - skipped == 0 ->
    IO.puts(:stderr, "no checks ran from checks/job/")
    System.halt(1)

  failures > 0 ->
    System.halt(1)

  true ->
    System.halt(0)
end
