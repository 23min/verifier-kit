# Example: notes

A notes service with drafts, publishing, trash and roles (owner, editor, viewer). It is the
toolkit's acceptance fixture: `demo/faults.sh` seeds faults into it and checks that the right
part of the oracle catches each one. Its files change only when a toolkit change needs it, and
the change says why (see `CLAUDE.md`).

## What it shows

- A short chapter that leaves who may do what open, and two readings of it, by different
  models, that disagree in 15 cases (`spine diff` on `spec/notes/readings/`). Each is a
  question for the chapter's owner; the ratified spine answers them.
- A spine for one machine with one variable (`role`).
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
spec/notes/readings/             two readings, with the prompt and their models     interpreters
spec/notes/spine.yaml            the ratified spine                                 person
spec/notes/behaviors/            valid/ must be allowed, invalid/ refused           person
checks/notes/test_content.py     content checks                                     person
checks/run.cmd                   the line that runs them, through run_checks.py     person
conformance/adapter_notes.py     translates spine events into service calls; its    person
                                 docstring is the whole contract with src/
conformance/adapter_notes.cmd    the line that runs it, through adapter_shim.py     person
mutation/mutants.yaml            deliberate bugs the oracle must catch              person
src/notes_app.py                 the service                                        builder
demo/notes_app_as_written.py     the service as first written, with two bugs
```

## What is checked, and how far

| What | How | Bound |
|---|---|---|
| The service matches the spine | `spine conform` through `adapter_notes.cmd` | every sequence up to 4 events |
| The runs that must be allowed or refused | `spine behaviors` on `spec/notes/behaviors/` | 2 valid, 3 invalid |
| A note's text is kept through edit, trash and restore | `checks/notes/test_content.py` | one run each |

## The mutants

`mutation/mutants.yaml` lists five small edits to `src/notes_app.py`. `./verify --full` makes
each in a copy of the example and runs conformance and the content checks on it, as for every
example; it first runs them on the service as it is, and stops with `oracle-fails` if they
don't pass there, since every mutant would then look caught (`docs/reference/tool-contract.md`).

| Mutant | Caught by |
|---|---|
| editors may restore | notes: code conforms to spine |
| drafts may be trashed | notes: code conforms to spine |
| active notes may be deleted forever | notes: code conforms to spine |
| trashed notes may be published | notes: code conforms to spine |
| edits are stored in upper case | content checks |

The last changes a note's text, which the spine doesn't see. With checks that never look at the
text it survives, as `demo/faults.sh` scenario 6 shows.

## Not verified

- **More than one note.** The adapter drives one note, shared with one editor; how notes and
  sharing affect each other is not checked.
- **Who may share.** The service lets only the owner share; the spine has no `share` event,
  and no check covers it.
- **Beyond the bound:** sequences longer than 4 events.
