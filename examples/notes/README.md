# Example: notes

A notes service with drafts, publishing, trash and roles (owner, editor, viewer). It is the
toolkit's acceptance fixture: `demo/faults.sh` seeds faults into it and checks that the right
part of the oracle catches each one.

## What it shows

- A spine for one machine with one variable (`role`), ratified from two readings of the
  chapter that disagree in four cases (`spine diff` on `spec/notes/readings/`).
- Conformance of a small Python service to the spine through an adapter, up to 4 events.
- Content checks for what the spine cannot see (a note's text), each citing its spine cell.
- Five hand-picked mutants, one of which only the content checks can catch.

## Run it

From the repo root:

```sh
./verify examples/notes          # fast checks
./verify examples/notes --full   # also the mutants
demo/faults.sh                   # every seeded fault must be caught
demo/live-builder.sh             # a paid Claude Code builder session; run only when you mean to
```

## Layout and who owns what

```
spec/notes/chapter.md            the prose chapter                                  person
spec/notes/readings/*.yaml       two independent readings of the chapter            interpreters
spec/notes/spine.yaml            the ratified spine                                 person
spec/notes/behaviors/            valid/ must be allowed, invalid/ refused           person
checks/notes/test_content.py     content checks                                     person
conformance/adapter_notes.py     translates spine events into service calls         person
mutation/mutate_notes.py         deliberate bugs the oracle must catch              person
src/notes_app.py                 the service                                        builder
demo/notes_app_as_written.py     the service as first written, with two bugs
```

The files are the acceptance fixture: change them only when a toolkit change needs it, and say
why (see `CLAUDE.md`).
