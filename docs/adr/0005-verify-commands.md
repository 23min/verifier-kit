# 0005. `./verify` runs adapters, content checks and mutation given as commands

- Status: accepted; decision 6 extended by `0006-retire-spine-py.md` (a Python mutation script
  gets `VERIFY_SPINE` too)
- Date: 2026-10-17
- Issue: #35

## Context

`spine conform --adapter-cmd` runs an adapter in any language (`0003-adapter-protocol.md`), but
`./verify` found each piece by a Python file name and ran it with `$PYTHON`:
`conformance/adapter_<machine>.py`, `test_*.py` under `checks/`, and `mutation/mutate_*.py`.
ADR 0003's decision 8 left discovering a command until there was an adapter that isn't Python.
#35 adds one, an example in Elixir, and the owner decided its six open questions on 2026-10-17.

## Decision

1. **A `.cmd` file per piece,** next to where the Python one would be:
   `conformance/adapter_<machine>.cmd`, `checks/run.cmd`, `mutation/mutate_<name>.cmd` (OQ1).
2. **One command line per file,** read as `--adapter-cmd` reads one: split by shell quoting
   rules and run without a shell. One line ending, `\n` or `\r\n`, may follow it. A file that
   can't be read, is empty, holds a second line, or can't be split is a new error,
   `bad-command`. `./verify` checks the adapter's line itself before handing it to `conform`, so
   all three pieces report a bad file the same way.
3. **Content checks and mutation are judged by exit status only** (OQ3, OQ4). 0 means at least
   one check ran and every one passed, or at least one mutant ran and every one was caught; a
   command that finds none must fail. The output is shown when the step fails. `no-checks` and
   `no-mutation` stay for a root with neither form.
4. **A mutation command runs the oracle itself,** as `mutate_notes.py` does (OQ4).
5. **The Python names keep working** beside `.cmd` files, and a piece given both ways is a new
   error, `ambiguous-piece`, not a silent choice (OQ5). For content checks, "both" means
   `checks/run.cmd` and any `test_*.py` file under `checks/`; for mutation, a `.py` and a
   `.cmd` of the same name.
6. **Each command runs from its root,** with `./verify`'s environment plus `VERIFY_SPINE`, the
   command line that runs the toolkit's `spine`, with absolute paths and quoted by shell rules
   (OQ6). `conform` also runs from the root for a `.cmd` adapter, so the adapter's line resolves
   there. `PYTHONPATH` is set for the Python pieces only.
7. **The messages only grow.** `no-adapter` and `no-mutation` now name the `.cmd` form too, by
   adding to the text `demo/faults.sh` expects rather than changing it.

## Consequences

- A project in any language can pass `./verify --full`. The Elixir example, `examples/elixir_job/`,
  is the first, and CI installs OTP and Elixir with `erlef/setup-beam` for it.
- `./verify` reads `.cmd` files with Python's `shlex`, as `tools/spine.py` splits
  `--adapter-cmd`, so a line means the same to both.
- A content-check or mutation command reports nothing `./verify` can parse. Its findings are its
  own output, and only its exit status counts.
- The `.cmd` paths are tested by the Elixir example, which lands in `main` together with this
  change, and by `demo/faults.sh` scenarios seeded in it.

## Alternatives considered

- **Executables found by name,** such as `conformance/adapter_<machine>` with no extension. No
  file format to define, but an executable bit is easy to lose in a checkout, and a script needs
  a shebang for every language.
- **One file per root naming the three commands.** One place to look, but one more format, and
  a piece's command no longer sits where its Python form would.
- **Moving the Python examples to commands too,** so there is one rule. It would change the
  notes example, which is the acceptance fixture, for no gain in what is checked.
- **Parsing a check command's output** to count checks and name failures. It would need a
  format every test runner can produce; the exit status is the one thing they all share.
