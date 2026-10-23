# 0006. `tools/spine.py` retires; the Go `spine` lives in `tools/`, and `./verify` builds it

- Status: accepted; the end of decision 3 reversed by `0007-no-language-built-in.md`
  (`./verify` no longer exports `PYTHON`)
- Date: 2026-10-18
- Issue: #44

## Context

ADR 0004 ported `spine` to Go, kept `tools/spine.py` as the reference while the two were
compared, and left retiring it to its own issue. ADR 0005 let `./verify` run a project's pieces
in any language. #44 retires `spine.py`. The owner decided its three open questions on
2026-10-18, and during the work, where the Go module lives.

## Decision

1. **`./verify` builds `spine` on every run** (OQ1): `go -C tools build -trimpath -o bin/spine
   ./cmd/spine`, so the binary is `tools/bin/spine` and never lags behind its sources. Go's
   build cache makes the build quick when nothing changed: about 0.3 s, measured in the
   devcontainer. `-trimpath` lets a fresh copy of the repo, such as each one `demo/faults.sh`
   makes, use the cache too: 0.24 s there, against 0.6 s without it. Without Go, `./verify`
   fails; it never skips. There is no `SPINE` variable until a project needs one.
2. **The Go module lives in `tools/`** (`github.com/23min/verifier-kit/tools`), beside
   `run_checks.py` and `adapter_shim.py`. A project copies `verify` and `tools/` as before, and
   nothing new lands at its root. A project written in Go is unaffected: its own module leaves
   the nested `tools/go.mod` alone. This supersedes the module's place in ADR 0004's decision 1.
   The `go` commands run with `-C tools`.
3. **`VERIFY_SPINE` names the Go `spine`, for Python mutation scripts too** (OQ2). notes' and
   ref_sync's mutation scripts run whatever it names, as elixir_job's does, and find `tools/`
   by `run_checks.py`. This extends ADR 0005's decision 6. It changes
   `examples/notes/mutation/mutate_notes.py`, part of the acceptance fixture; the owner
   approved that change. `./verify` exports `PYTHON`, so `spine` serves a Python adapter with
   the Python `./verify` picked. (Reversed on 2026-10-23: `./verify` picks no Python and
   `spine` serves none; see `0007-no-language-built-in.md`.)
4. **A record takes over from `spine.py` as the reference** (OQ3).
   `tools/internal/spine/testdata/record/` holds what the Go `spine` gave on every input of the
   differential test, recorded while the test showed both implementations agree. `go test`
   compares the Go `spine` against it. After a deliberate change, `-update` writes it again,
   and its diff is reviewed.
5. **Four steps, each leaving `main` green:**
   - the record (#45);
   - the move to `tools/`;
   - the switch: `./verify`, `demo/`, the mutation scripts, the aiwf snippet and the templates
     under `integrations/`;
   - removing `tools/spine.py`, and the Python half of the differential test.

## Consequences

- Once `spine.py` is gone, the spine steps need no Python. Python stays for
  `tools/run_checks.py`, `tools/adapter_shim.py`, `./verify`'s reading of `.cmd` files, and the
  Python examples. PyYAML is then needed only by ref_sync's checks.
- Wherever `./verify` runs needs Go 1.27. This repo's CI installs it already; the CI templates
  under `integrations/ci/` add it with the switch.
- `tools/` carries the Go tests and the record, about 4.9 MB in all. A project can leave out
  `tools/internal/spine/*_test.go` and `testdata/`.
- The devcontainer runs `go mod download` only for a `go.mod` at the repo root, so in a fresh
  container the first build downloads the YAML module.
- ADR 0004's known differences become the Go `spine`'s own behavior once `spine.py` is gone.
- Some recorded text is written by Python, such as exception messages from the test adapters.
  The record was written on Python 3.12, and passes on 3.13.

## Alternatives considered

- **A prebuilt `spine`, required by `./verify`** (OQ1). No build in `./verify`, but a stale
  binary would go unnoticed: after an edit to the Go sources, `./verify` would run the old one
  while `go test` builds a new one.
- **A `SPINE` variable naming the binary** (OQ1). For a binary installed some other way, a
  case that hasn't arisen.
- **`tools/spine.py` kept as a few lines that start the Go `spine`** (OQ2). The fixture stays
  unchanged, but a file by that name stays without being the implementation.
- **A smaller record, of exit codes and finding codes only** (OQ3). It would no longer check
  the text, the JSON shape or the JUnit report.
- **Keeping `tools/spine.py` for the test alone** (OQ3). Every change would still be made
  twice, and `go test` would still need PyYAML.
- **The Go module at the project's root, beside `verify`.** A smaller change here, but a
  project written in Go couldn't adopt the toolkit (two `go.mod` files at one root), and the
  builder's deny lists would need six new paths.
