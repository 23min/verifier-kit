# Examples

Each folder here is one example, laid out like a project that uses the toolkit
(`docs/adr/0001-examples-layout.md`). The examples differ in what they show and in what their
language dictates, never otherwise in how they are set up (`docs/adr/0007-no-language-built-in.md`).
These conventions hold for this repo's examples only, not for projects. `conventions_test.go`
checks the ones that are mechanical: `go -C examples test ./...`.

## What every example has

- `README.md`, with these sections, in order: What it shows; Run it; Layout and who owns what;
  What is checked, and how far; The mutants; Not verified.
- `spec/<machine>/chapter.md`: a title and a short introduction, then seven numbered sections:
  1. Glossary; 2. the process, named for it; 3. What must hold; 4. Scenarios; 5. Assumptions;
  6. Not covered; 7. What the lifecycle does not show. A section with nothing to say says "None."
- `spec/<machine>/readings/`: `prompt.md`, the instructions both readers are given, the same in
  every example but for rule 1, which names the machine and its variables; and
  `reading-a.yaml` and `reading-b.yaml`, by two different models. Each reading is the reader's
  reply, unedited, under a header naming the reader, its model, the date, and the
  `git hash-object` of the chapter, the prompt and `docs/reference/spine-format.md` it was made
  from.
- `spec/<machine>/spine.yaml`, headed with the lifecycle it is, when it was built from
  `readings/` and the owner's answers, in which issue, and "Ratified by the reviewed merge of
  this file." It has no worked examples: sample runs go in `spec/<machine>/behaviors/`.
- `conformance/adapter_<machine>.cmd`, and the program it runs, whose header states what `src/`
  must offer: the whole contract between the oracle and the code.
- `checks/run.cmd`. The content checks cite what they depend on with `covers:`, and report as
  `tools/run_checks.py` does: a `check-failed` or `check-unloadable` line per problem,
  `no-checks` when none ran, then the tally.
- `mutation/mutants.yaml`, and no mutation command.
- `src/`, the code under test.

## What may differ

- What the example shows: its subject, chapter, spine, checks, scenarios and sizes, and checks
  of its own beyond the content checks, such as ref_sync's explorer and its checks against real
  Git.
- What its language dictates: the adapter's program, the checks' runner, and the command lines
  that start them.
- notes, the acceptance fixture, alone keeps the first version of its code,
  `demo/notes_app_as_written.py`, for `demo/faults.sh` and `demo/live-builder.sh`.
