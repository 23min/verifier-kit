# Introduction

This page explains verifier-kit's parts in the order they build on each other, a few sentences
each. Afterwards, [the guide](guide.md) takes your own project through them step by step, and
[the examples](../examples/README.md) show each part working.

![How verifier-kit fits together. You write a chapter in prose; two readers, on different
models, each turn it into a spine; spine diff turns their disagreements into questions, and your
answers make the ratified spine. The builder changes only the code. ./verify, run by the agent's
hook, the git hook and CI, checks the code against the spine through the adapter, runs the
content checks, and plants bugs to check that the oracle notices.](introduction/1-how-it-fits.svg)

## 1. Two sides

A coding agent writes code quickly, and it can also write the tests that pass it. verifier-kit
splits the work between two sides:

- **The oracle is yours.** It says what the system must do, precisely enough for a program to
  check. ("Oracle" is the testing term for whatever decides whether an answer is right.)
- **The code is the builder's.** An agent, or a person, changes it until it agrees with the
  oracle.

A program decides whether they agree, never the builder. In a project, each side is a set of
folders:

| Folder | Holds | Whose |
|---|---|---|
| `spec/<machine>/` | the chapter, its readings, the spine, its drawing, behaviors | yours |
| `conformance/` | the adapter, which connects the spine to the code | yours |
| `checks/` | content checks | yours |
| `mutation/` | planted bugs the oracle must catch | yours |
| `src/` | the code | the builder's |

An LLM may draft every part of the oracle for you. What makes it yours is that you review and
approve it, and the builder can't change it.

## 2. A spine

The core of the oracle is a *spine*: one small state machine for one feature. This is the spine
of the notes example, a service in which notes are drafted, published, trashed and restored:

![The notes spine, drawn by spine draw. Four states: draft, where it starts; active; trashed;
and purged, which is terminal. Arrows: publish by the owner moves a draft to active, trash by
the owner moves active to trashed, restore by the owner moves trashed back to active, and purge
by the owner moves trashed to purged. A table below lists every rule that stays in its state,
ignores the event or refuses it.](../examples/notes/spec/notes/drawing.svg)

A spine has four parts:

- **States** the thing can be in: `draft`, `active`, `trashed`, `purged`. Each has a kind: a
  *fact* records something that can't be undone in the world (a note purged for good), a
  *policy* is a business choice that may change, and a *design* is an implementation choice.
- **Events** that happen to it: `publish`, `edit`, `trash`, `restore`, `purge`.
- **Variables**, each with a few values: here `role`, which is `owner`, `editor` or `viewer`.
- **Rules.** For every state, event and value, exactly one rule says what happens: the event
  moves to another state (`to`, an arrow in the drawing), stays (`stay`), is accepted and does
  nothing (`ignore`), or is refused (`reject`). The table under the drawing lists every rule
  that isn't an arrow.

The spine itself is YAML, and `spine draw` makes the drawing from it. The start of notes' spine,
and two of its 25 rules ([the whole file](../examples/notes/spec/notes/spine.yaml)):

```yaml
machine: note
vars:
  role: [owner, editor, viewer]
states:
  draft:   {kind: policy}
  active:  {kind: policy}
  trashed: {kind: design}
  purged:  {kind: fact, terminal: true}
initial: draft
events: [publish, edit, trash, restore, purge]
rules:
  - {from: active, event: trash, when: "role == 'owner'", to: trashed}
  - {from: active, event: trash, when: "role != 'owner'", reject: true}
```

A spine covers a lifecycle and its permissions: which steps are allowed, for whom, and where
they lead. When a step sends something to another system, such as a mail or a commit, the spine
can say that too: it declares `outputs:`, and each rule that moves or stays names what its step
sends, `outputs: [send_mail]`, or `[]` for nothing. The drawing writes it after the event,
`run / send_mail`. A spine says nothing about a note's text or a mail's address; that is for
content checks (section 7). The format is in
[`reference/spine-format.md`](reference/spine-format.md).

## 3. Every case decided

States, events and values are few, so every case can be listed. Notes has three states that
aren't terminal, five events and three roles: 45 cases. `spine check` confirms that exactly one
rule decides each one. It reports a case no rule covers, two rules that disagree on a case, and
a state that nothing reaches. A gap in your thinking turns up as a finding before any code
exists, not as a bug later.

## 4. From prose to spine

You don't start with the spine. You start with a *chapter*: plain prose about the feature,
written with an LLM as your spec partner. It has a glossary, the process, what must hold,
scenarios, assumptions, and what it leaves out. For an existing system, the chapter can start
from an audit of what its docs and its history say
([step 3 of the guide](guide.md#3-write-the-chapter)). Then:

1. **Two readings.** Two readers, LLM sessions on different models that see neither the code nor
   each other, each turn the chapter into a spine.
2. **Questions.** `spine diff` compares the two readings case by case, and turns each
   disagreement into a question with a run that shows it: "after publish, trash, restore by an
   editor: A says rejected, B says active."
3. **Your answers.** You answer each question, and the answers make the ratified spine. You
   review it as a drawing as well as YAML, and approve it with a reviewed merge.

The readings find what your prose leaves open; they don't gate anything. In notes they disagreed
on 15 cases. Two readers can share a misreading, so a clean diff is evidence, not proof.

## 5. Reaching the code

The spine speaks of states and events; the code has functions and data. The *adapter* connects
them with three operations:

- **`new`:** a fresh instance of the system, in the spine's initial state.
- **`apply`:** one event with its values, such as `trash` by the owner, answered with `ok` or
  `rejected`. Notes' adapter calls `svc.trash(note, user)` and answers `rejected` when the
  service refuses.
- **`observe`:** read the system's real data and name the state it is in.

When the spine declares outputs, `apply`'s answer also says what the step sent, read from what
the system really did: a call caught on its way out, an outbox compared before and after.

The adapter only translates. It never decides whether something is allowed, since that is what
is being checked. It is a program that speaks JSON lines, so the code can be in any language; for
Python, a file with three functions is enough, and a fourth, `outputs`, when the spine declares
outputs.

## 6. Checking the code

`spine conform` runs the spine and the code side by side through the adapter. It reaches each
state by a shortest path, tries every event with every value from there, and one step further
(the *margin*). After each step it compares the code's answer and state, and what the step sent
when the spine declares outputs, with the spine's. Notes
takes 180 runs, well under a second. When they differ, conform prints the shortest run that
shows it. This is the first version of notes' service, which let a draft be trashed:

```
error   diverges           draft/trash role=owner: after trash(role=owner): spine expects rejected in draft, code gave ok in trashed
```

A case that conform couldn't reach is an error, not a pass. When every case agrees, the code and
the spine agree on runs of any length, provided the code is deterministic and what the adapter
observes is all that decides what the code does next. Section 12 says what that leaves out, and
[`design.md`, "What conform guarantees"](design.md#what-conform-guarantees) says it exactly.

## 7. What a spine can't see

A spine knows that an editor may edit a published note, but not that the edit keeps the text.
*Content checks* are ordinary tests for that: text, totals, formats. Each one cites the spine
cell it depends on, with the cell's hash:

```python
# covers: active/edit@a6899d5a7722
def test_owner_edit_of_published_note_keeps_text():
```

When the rules of a cell change, its hash changes, and `spine stale` reports every check that
cites the old one. A check can't go on testing a rule that is gone without anyone noticing.
`checks/run.cmd` holds the line that runs them, in your project's own test runner.

## 8. Example runs

Optionally, `spec/<machine>/behaviors/` holds short runs that you agree on: those in `valid/`
the spine must allow, those in `invalid/` it must refuse. They state your intent from the
outside, and catch a spine that drifts from it. One of notes':

```yaml
steps: ["publish role=owner", "edit role=editor"]
expect: active
```

## 9. Checking the checks

An oracle that passes every program is worthless, so verifier-kit tests the oracle too.
`mutation/mutants.yaml` lists planted bugs, each one small edit to the code:

```yaml
- name: 'drafts may be trashed'
  file: src/notes_app.py
  find: 'raise InvalidState("only published notes can be trashed")'
  replace: 'pass'
```

`./verify --full` makes each one in a copy of the project and fails if conform and the content
checks all miss it. A mutant that survives points at a gap in the oracle, not in the code.

## 10. One command

`./verify` runs every step, and every gate calls it and nothing else. Its checks make no network
or model calls, and give the same output every time.

| Step | Catches | Doesn't catch |
|---|---|---|
| the spine is sound (`spine check`) | YAML slips such as `on:` read as true, cases no rule decides, rules that overlap, states nothing reaches | a spine that is complete but wrong |
| behaviors match the spine | a spine that drifted from the runs you agreed on | |
| checks cite the current spine (`spine stale`) | checks citing a cell, or the whole spine, that has changed; citations written wrongly | checks that cite nothing (listed) |
| the code conforms (`spine conform`) | any case where the code allows, refuses or ends somewhere other than the spine says, with the shortest run that shows it | hidden state beyond the margin; data the adapter doesn't observe |
| the drawing matches the spine (`spine draw`) | a committed drawing that no longer shows the spine | |
| content checks | wrong text, values, formats | whatever no check asserts |
| mutants killed (`--full`) | an oracle too weak to notice a planted bug | bugs unlike the planted ones |
| the path guard (in CI) | a branch changing paths its role doesn't own | |

A step that doesn't run says so and why, and a missing piece, such as a spine without an
adapter, is an error, not a pass.

## 11. The gates

Checking only helps if the builder can't skip it. `integrations/` has example files for each
layer, and [step 11 of the guide](guide.md#11-the-gates) says how to set them up:

- **The agent's harness:** deny rules keep the agent's file tools out of the oracle, and a Stop
  hook runs `./verify` whenever the agent tries to finish, sending it back with the failing
  lines.
- **Git:** a pre-commit hook runs the fast checks.
- **CI:** `./verify --full` and the path guard on every pull request.
- **Review:** code owners approve every change to the oracle.

Only CI, as a required check on a protected branch, is a gate; the others make failures cheap to
fix early, and each can be bypassed ([`design.md`, "Gates"](design.md#gates)).

In a live run (`demo/live-builder.sh`), Claude Code, as the builder, was given notes' first
version and told about one of its bugs, with no shell to run `./verify` itself. It fixed that
bug and tried to finish. The Stop hook ran `./verify`, which failed on the other bug, a draft
that could be trashed. The builder read the spine, fixed it and tried again, and the hook let it
finish: four lines changed in `src/`. The agent never chose to run the checks; the hook did.

## 12. Limits

- **What the adapter can't see.** Conform's guarantee holds as long as what `observe` reports
  decides what the code does next. A counter the adapter can't see can hide a bug, such as one
  that only a third restore of a note sets off. Each step of `--margin` rules out one more such
  hidden state, at about four times the runs, and state that only a refusal changes, such as a
  lockout after three refused attempts, is never reached
  ([ADR 0008](adr/0008-conform-margin.md)). Making such state observable rules it out instead.
- **One lifecycle at a time.** Conform doesn't run clients against each other. Concurrency needs
  checks of its own, such as the ref_sync example's explorer, a parallel state-machine test or a
  model checker ([other tools](comparison.md)).
- **States, permissions and outputs, not content.** A spine names what a step sends, not what is
  in it. Text, totals, formats and addresses need content checks.
- **The adapter is part of the oracle.** If it decides instead of translating, it can hide a bug;
  review it like the spine.
- **One machine at a time.** Real features have machines that interact, and more variables; the
  number of cases grows fast, and nothing here handles that yet.

## 13. Where next

- [The guide](guide.md): your own project, from installing the toolkit to gated builds, saying
  at each step what an LLM drafts and what you decide.
- [The examples](../examples/README.md): four projects that use the toolkit, from the notes
  service above to Oban, an Elixir job library the toolkit didn't write.
- [Other tools](comparison.md): how verifier-kit compares with proof assistants, model
  checkers, model-based and property-based testing, and spec-driven agent tools.
- [The reference](reference/): the spine format, the tool contract and the drawing layout,
  exactly.
- [The design](design.md), for why it works this way, and [the architecture](architecture.md),
  for how this repo is built.
