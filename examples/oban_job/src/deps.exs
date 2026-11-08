# The Hex packages src/, the adapter and the checks use, pinned by mix.lock (#110, AC8). Every
# command line loads this file first, with HEX_OFFLINE=1, so ./verify fetches nothing. Setting
# up runs it once, online, from the repository's root: elixir examples/oban_job/src/deps.exs
try do
  Mix.install([{:oban, "2.24.2"}, {:ecto_sqlite3, "0.26.0"}],
    lockfile: Path.join(__DIR__, "mix.lock"),
    config: [ecto_sqlite3: [json_library: JSON]]
  )
rescue
  e in Mix.Error ->
    reraise Mix.Error,
            [
              message:
                Exception.message(e) <>
                  "; set up once, online: elixir examples/oban_job/src/deps.exs"
            ],
            __STACKTRACE__
end
