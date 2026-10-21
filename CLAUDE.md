# CLAUDE.md

This repo is a standalone toolkit for checking code written by coding agents against a small,
executable oracle that people own. The tools are plain command-line programs; agent hooks, git
hooks and CI call them through one script, `./verify`.

Read before changing anything: `README.md`, `docs/design.md` (the design, normative) and
`docs/reference/` (the tool contract and spine format).

## Rules

- **Before you finish any change,** `./verify --full`, `demo/faults.sh`, `go -C tools vet ./...` and
  `go -C tools test ./...` all pass, as they must in CI. Show their last lines. Don't call something done
  because it looks right; show the command and its output.
- **The notes example is the acceptance fixture.** Don't edit `examples/notes/` (its spec,
  checks, conformance adapter, `src/notes_app.py`, mutation and demo) or `demo/` to make a
  failure go away. If a tool change needs the fixture to change, stop and say why.
- **Each example lives in `examples/<name>/`,** laid out like a project that uses the toolkit
  (`docs/adr/0001-examples-layout.md`); `./verify examples/<name>` runs one alone.
- **Examples are equal.** They differ in what they show and in what their language dictates,
  never otherwise in how they are set up (`docs/adr/0007-no-language-built-in.md`). notes alone
  is the acceptance fixture.
- **The spine format and the tool contract are fixed** (`docs/reference/`). Changing them,
  including exit codes, JSON shape or finding codes, needs a GitHub issue the owner has
  approved. New finding codes may be added; existing ones are never renamed or reused.
- **Tools stay standalone:** files in, findings out. No network, no model calls (the planned
  `interpret` tool is the one exception), no knowledge of who calls them, no aiwf or other
  workflow tool. Python standard library plus PyYAML unless an issue says otherwise; the Go
  `spine` (`tools/cmd/spine/`, `tools/internal/spine/`) uses the Go standard library plus
  `go.yaml.in/yaml/v4`.
- **Docs move with code.** A change to a tool updates `docs/reference/` in the same commit;
  `docs/design.md` changes with the code it describes.
- **Keep it clean.** A change leaves no second copy of anything, no code or file that nothing
  uses, and no description that disagrees with the code. Each pull request says, besides the
  four commands:
  - which documents it changed, and the search that found every place the changed fact is told;
  - that `go run golang.org/x/tools/cmd/deadcode@v0.52.0 -test ./...`, run in `tools/`, reports
    nothing (a check run by hand, not a dependency of the toolkit);
  - what it removed, and that nothing it added is unused.
- **Work is tracked in GitHub issues** with `gh`. One issue per piece of work, with acceptance
  criteria and open questions; reference it in commits (`Refs #12`). Ask before closing one.
  #61 holds the plan: the steps, in order, toward the design in ADR 0007.
- **Branches and pull requests.** Work on a branch; ask before pushing to `main`, merging or
  tagging.

## Commands

```sh
./verify             # fast checks on every example, a few seconds
./verify --full      # also checks the checker (mutation)
./verify examples/notes --full   # one example only
demo/faults.sh       # acceptance test: every seeded fault must be caught; exits 1 otherwise
demo/faults.sh 4     # one scenario
go -C tools build -o bin/spine ./cmd/spine   # spine, into tools/bin/spine; ./verify does this too
tools/bin/spine --help
go -C tools vet ./...
go -C tools test ./...   # runs spine on every test input; any difference from the record
                         # (tools/internal/spine/testdata/record/) fails
go -C tools test ./internal/spine/ -update   # writes the record again from the Go spine; review its diff
```

Setup needs Python 3; PyYAML and the `git` program for the ref_sync example's checks; Elixir
1.18 or later for the elixir_job example's; and Go 1.27, which `./verify` and `demo/faults.sh`
build `spine` with. The
devcontainer image has all five; don't create a `.venv` there
(the repo is shared with the host). Elsewhere: `uv venv && uv pip install -r requirements.txt`;
`./verify` and `demo/faults.sh` use `.venv` automatically when it exists.
`demo/live-builder.sh` starts a real Claude Code session and costs money; run it only when asked.

## Handoffs

When asked for a handoff, for example before `/compact` or a container rebuild:

- Write it in the chat, ready to paste into `/compact`. Write it to a file only when asked.
- Keep it to about 25 lines, holding only what isn't recorded elsewhere. Link to issues, comments
  and ADRs instead of repeating them; CLAUDE.md and the global rules load by themselves.
- Cover:
  - the state: `main`'s commit, branches, open pull requests, and untracked files never to
    commit;
  - each open work item: what's done, what's unverified, and what waits on the owner;
  - the next steps, in order;
  - traps met in the session that the next one would hit again;
  - what a container rebuild loses.
