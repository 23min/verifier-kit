# 0003. An adapter protocol: JSON lines over stdin and stdout

- Status: accepted
- Date: 2026-10-14
- Issue: #24

## Context

`spine conform` imported `conformance/adapter_<machine>.py` and called `new()`, `apply()` and
`observe()` in its own process, so the code under test had to be Python. `docs/design.md`
listed "the adapter protocol for other languages" as designed but not built, and as an open
decision. The planned Go port of the toolkit (track 4) needs it too: a Go `conform` cannot
import a Python adapter, and its findings are to be compared with the Python toolkit's on the
same adapters.

## Decision

1. **JSON lines over stdin and stdout,** one request and one reply per line, flushed; stderr is
   the adapter's own and is not read. Requests are `new`, `apply` and `observe`, the three
   functions of a Python adapter.
2. **`spine conform --adapter-cmd "COMMAND"`,** which excludes `--adapter`. The string is split
   by shell quoting rules and run without a shell.
3. **No handles.** `new` replaces the current instance; `apply` and `observe` act on it.
   `conform` uses one instance at a time, and parallel runs, if they come, would start several
   programs. The reply to `new` is `{"ok": true}`.
4. **No version handshake.** A program replies `{"error": ...}` to a request it does not
   recognize, so a later protocol can open with a `hello` and recognize an older program.
5. **No timeout,** as in process: a clock would make a slow but correct adapter fail on a
   loaded machine. The program must flush every reply, which removes the usual cause of a hang.
6. **One code for every adapter failure,** `adapter-failed` from #21, on both paths: an
   `{"error": ...}` reply to `new` or `observe`, a program that can't start or exits early, and
   a reply that is not JSON or has the wrong shape. An `{"error": ...}` reply to `apply` is a
   divergence, as an exception from `apply()` is.
7. **The in-process adapter stays.** The command path is an object with the same `new`, `apply`
   and `observe`, so `conform` replays sequences one way.
8. **Only `conform` gains it.** `./verify` keeps finding `conformance/adapter_<machine>.py`;
   discovering a command waits for an adapter that is not Python.
9. **A shim,** `tools/adapter_shim.py`, standard library only, serves any Python adapter over
   the protocol unchanged, and sends the adapter's printing to stderr.

## Consequences

- An adapter can be in any language, and the protocol is tested before one exists: through the
  shim, `conform`'s JSON findings for notes (as it is and as first written) and ref_sync equal
  the in-process ones, and `demo/faults.sh` scenario 10 finds the notes bugs this way.
- Each request is a round trip between processes, about 0.2 ms in the devcontainer (a bare
  Python echo over a pipe takes 0.15 ms). Notes makes 13,412 of them: 2.3 s through the shim
  against 0.2 s in process; ref_sync 0.6 s against 0.2 s.
- A program that never replies makes `conform` wait until someone or CI's job timeout stops it.
- When the shim cannot import the adapter it exits 1 with the reason on stderr, so the finding
  says only that the program exited before replying; the reason is on stderr.
- A project whose adapter is not Python can run `conform` but not all of `./verify` yet.

## Alternatives considered

- **The command after `--`.** No string splitting, but all options must come first, and a
  command kept in a file would still need splitting.
- **`--adapter FILE` running any file that is not `.py` as a program.** No new option, but
  behavior would depend on a file's extension and executable bit.
- **Handles, with or without a `drop` request.** Several instances per program, at the cost of
  a handle table in every adapter, unknown-handle errors, and without `drop` a leak of every
  instance until the end.
- **A handshake now, or no rule for unknown requests.** The first adds a message every adapter
  must implement for a second version that does not exist; the second leaves a later change
  to cope with adapters that hang or crash on an unknown request.
- **A timeout, fixed or optional.** Either brings a clock into the tools; an optional one adds
  an option the Go port must match. It can come when a caller meets a hang.
- **Every Python adapter through the shim.** One path, but every run pays the round trips.
- **`./verify` discovering `conformance/adapter_<machine>.cmd`, or a switch to run every
  adapter through the shim.** Useful later; nothing needs either yet.
- **A separate code for protocol failures.** An import failure would then get different codes
  in process and through the shim, which breaks the comparison the Go port relies on.
