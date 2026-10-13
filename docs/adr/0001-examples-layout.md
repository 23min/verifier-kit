# 0001. One folder per example, shaped like a project

- Status: accepted
- Date: 2026-10-13
- Issue: #15

## Context

The repo holds one worked example, notes, spread over the top-level `spec/`, `checks/`,
`conformance/`, `src/`, `mutation/` and `demo/`. A second example (ref-sync) is planned.
`./verify` runs the content checks once over all of `checks/` and every mutation script under
`mutation/`, so one example's checks hide another's missing pieces: adding any
`checks/ref_sync/test_*.py` stops `demo/faults.sh` scenario 8 from seeing `no-checks` when the
notes checks are removed. Examples could also not be run on their own.

## Decision

1. Each example lives in `examples/<name>/`, laid out like an adopting project: its own
   `spec/`, `checks/`, `conformance/`, `src/`, `mutation/`, `demo/` and a `README.md`. The
   toolkit (`verify`, `tools/`) stays at the repo root and is not made a Python package.
2. `./verify [ROOT] [--full]` checks one root. Without ROOT it checks the repo root when it has
   `spec/`, and otherwise every `examples/*/` in turn, failing if any fails. An example folder
   without `spec/` is a `no-machines` error. Callers keep calling plain `./verify`.
3. Today's `examples/` (CI, Claude Code, git hooks, aiwf, CODEOWNERS) becomes `integrations/`.
4. `demo/` stays at the top as the toolkit's demos; only the notes code it uses
   (`notes_app_as_written.py`) moves into `examples/notes/demo/`. `demo/faults.sh` keeps
   checking what it checks today.
5. One `policy.yaml` at the repo root covers every example; the path guard runs once per
   `./verify` call, whatever the roots are.

## Consequences

- An example runs alone (`./verify examples/notes`) and its missing pieces are reported for it
  alone. Adding an example means adding a folder; the toolkit does not change.
- A project with `spec/` at its root sees no change.
- `verify` has two ways to find roots, and the tool contract states both.
- Every example gets the same roles in the path guard. An example copied out into its own repo
  needs its own `policy.yaml`.
- The files in `integrations/` are written for a project with `spec/` at its root. So
  `demo/live-builder.sh` builds its scratch project from `examples/notes/` plus the toolkit,
  shaped like an adopting project, and the shipped Claude Code settings apply unchanged.

## Alternatives considered

- **Keep the shared layout and add `./verify --machine NAME`.** Smallest diff, but an example
  stays spread over five folders and cannot be copied out as a unit.
- **Make the toolkit a Python package** (`src/verifier_kit/`, `pyproject.toml`). Adds an install
  step and commits early on the open question of Python or a Go binary.
- **Make callers name every root** (`./verify examples/notes examples/ref_sync`). Explicit, but
  every caller must know the layout and change when an example is added.
- **Move all of `demo/` into the notes example, or split `faults.sh`.** Either changes the
  acceptance test in the same change that moves the files, which weakens the before-and-after
  evidence that nothing broke.
- **A `policy.yaml` per example.** The toolkit's own paths belong to no example, so a
  repo-level policy would be needed as well.
