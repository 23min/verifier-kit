# CLAUDE.md

This repo is a standalone toolkit for checking code written by coding agents against a small,
executable oracle that people own. The tools are plain command-line programs; agent hooks, git
hooks and CI call them through one script, `./verify`.

Read before changing anything: `README.md`, `docs/design.md` (the design, normative) and
`docs/reference/` (the tool contract and spine format).

## Rules

- **Before you finish any change,** `./verify --full` and `demo/faults.sh` both pass. Show their
  last lines. Don't call something done because it looks right; show the command and its output.
- **The notes example is the acceptance fixture.** Don't edit `examples/notes/` (its spec,
  checks, conformance adapter, `src/notes_app.py`, mutation and demo) or `demo/` to make a
  failure go away. If a tool change needs the fixture to change, stop and say why.
- **Each example lives in `examples/<name>/`,** laid out like a project that uses the toolkit
  (`docs/adr/0001-examples-layout.md`); `./verify examples/<name>` runs one alone.
- **The spine format and the tool contract are fixed** (`docs/reference/`). Changing them,
  including exit codes, JSON shape or finding codes, needs a GitHub issue the owner has
  approved. New finding codes may be added; existing ones are never renamed or reused.
- **Tools stay standalone:** files in, findings out. No network, no model calls (the planned
  `interpret` tool is the one exception), no knowledge of who calls them, no aiwf or other
  workflow tool. Python standard library plus PyYAML unless an issue says otherwise.
- **Docs move with code.** A change to a tool updates `docs/reference/` in the same commit;
  `docs/design.md` changes with the code it describes.
- **Work is tracked in GitHub issues** with `gh`. One issue per piece of work, with acceptance
  criteria and open questions; reference it in commits (`Refs #12`). Ask before closing one.
- **Branches and pull requests.** Work on a branch; ask before pushing to `main`, merging or
  tagging.

## Commands

```sh
./verify             # fast checks on every example, about a second each
./verify --full      # also checks the checker (mutation)
./verify examples/notes --full   # one example only
demo/faults.sh       # acceptance test: every seeded fault must be caught; exits 1 otherwise
demo/faults.sh 4     # one scenario
python3 tools/spine.py --help
```

Setup needs Python 3 with PyYAML. The devcontainer image has both; don't create a `.venv`
there (the repo is shared with the host). Elsewhere: `uv venv && uv pip install -r
requirements.txt`; `./verify` and `demo/faults.sh` use `.venv` automatically when it exists.
`demo/live-builder.sh` starts a real Claude Code session and costs money; run it only when asked.
