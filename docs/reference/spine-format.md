# Spine format

A spine is one state machine for one feature: which states exist, which events can happen,
and what each event does in each state for each combination of variable values. It covers
lifecycle and permissions only. Computations, content and timing belong in other views (see
`docs/design.md`). Changing this format needs an issue and the owner's approval first.

## File

`spec/<machine>/spine.yaml`. Top-level keys, and nothing else:

| Key | Required | Holds |
|---|---|---|
| `machine` | yes | the machine's name |
| `section` | no | the chapter section it was ratified from |
| `vars` | no | each variable mapped to a short, finite list of values, each listed once (a repeat is a `listed-twice`) |
| `states` | yes | each state mapped to `{kind: fact|policy|design, terminal: true}` (`terminal` optional) |
| `initial` | yes | the starting state |
| `events` | yes | the list of event names, each listed once (a repeat is a `listed-twice`) |
| `rules` | yes | the list of rules |
| `examples` | no | worked examples that `check` replays |

```yaml
machine: note
section: chapter-4-trash
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
  - {from: draft, event: publish, when: "role == 'owner'", to: active}
  - {from: draft, event: publish, when: "role != 'owner'", reject: true}
  - {from: draft, event: edit, when: "role != 'viewer'", stay: true}
  - {from: draft, event: trash, reject: true}
  - {from: active, event: restore, when: "role == 'owner'", ignore: true}
examples:
  - name: owner trashes and restores
    steps: ["publish role=owner", "trash role=owner", "restore role=owner"]
    expect: active
```

## State kinds

- **fact:** records something irreversible in the world (a note purged for good). Changes
  should be rare and need a decision.
- **policy:** a business choice, expected to change, with a decision behind each change.
- **design:** an implementation choice; it may change freely while facts and policy hold.

## Rules

Each rule has `from`, `event`, an optional `when`, and exactly one outcome:

| Outcome | Meaning |
|---|---|
| `to: <state>` | the event is allowed and moves to that state |
| `stay: true` | allowed, same state |
| `ignore: true` | accepted as a no-op, not an error |
| `reject: true` | refused |

For every non-terminal state, every event and every combination of variable values, exactly
one rule must match. `check` reports `undecided-cell`, `missing-case` and `overlap` otherwise.

## Conditions

`when` is a small expression over declared variables only:

- literals (strings, numbers) and lists of literals;
- `==`, `!=`, `in`, `not in`;
- `and`, `or`, `not`.

Anything else (calls, attributes, arithmetic, undeclared names) is a `bad-condition`, and so is
an `in` that can't be evaluated: its right side must be a list, or text or a variable with text
or a variable on its left (`'own' in role` tests for a substring). Numeric rules ("over 1,000")
are split into named ranges at the thresholds the chapter gives, so the cases stay enumerable.

## Text only

Every key, and every name and value the format defines, is text: the machine and section,
state names, events, `initial`, a rule's `from`, `event`, `to` and `when`, variable names and
values, and in worked examples and behavior files `name`, `expect` and each step. Steps pass
values as text, so a number could never match one. Quote anything YAML would read as something
else: `"yes"`, `"no"`, `"on"`, `"off"`, `"012"`, `"1.5"`, `"2026-10-14"`, `"null"`. A key read
as true or false is a `yaml-boolean-key` and such a value a `yaml-boolean-value`; any other key
that isn't text is an `unknown-key`, and any other value a `spec-shape`.

A file is UTF-8 text. An escape for a surrogate, U+D800 to U+DFFF, such as `"\ud800"` or the
JSON-style pair `"\ud83d\ude00"`, isn't a character: the file is `spec-unreadable`. Write the
character itself, or `\U` and its eight hex digits.

Each mapping gives a key once. A key given twice, such as `role` twice under `vars`, makes the
file `spec-unreadable`, and the finding names the key and its lines.

## Steps, examples and behaviors

A step is `"<event> <var>=<value> ..."`, for example `"restore role=editor"`. It names a
declared event and gives every declared variable exactly once, with one of that variable's
values, and no other variable, so it names one whole case; anything else is a `bad-step`.
`steps` is always a list of steps, possibly empty. Every name in an example or behavior must be
declared, so a misspelling is reported rather than refused, which in `invalid/` would pass.

- **Examples** in the spine have `name`, `steps` and `expect`, all required and nothing else,
  where `expect` is a declared state (the end state), `rejected` (some step is refused) or
  `undefined` (some step has no rule); anything else is a `bad-value`.
- **Behavior files** live in `spec/<machine>/behaviors/valid/` and `.../invalid/`, one YAML file
  each: a mapping with `steps` and an optional `expect`, and nothing else. `expect` is a
  declared state, the end state, or else a `bad-value`. Files in
  `valid/` must be allowed and end as expected. Files in `invalid/` must not be: some step is
  refused or undecided, or the run ends somewhere other than `expect`.

## Readings

Unratified interpretations of a chapter, `spec/<machine>/readings/*.yaml`, use the same format.
`spine diff` compares two of them, or a reading against the ratified spine.

## Citing a cell, or the whole spine

A content check states which cell it depends on, anywhere in the file:

```python
# covers: trashed/restore@95f9c8204e9f
```

`spine hash <spec>` prints the current hash of every cell. When the rules for a cell change,
its hash changes and `spine stale` reports every check that cites the old one.

A check that depends on the whole machine rather than on single cells, such as one that
explores every interleaving, cites the spine instead (#59):

```python
# covers: spine@23fc1b40ab2e
```

That hash covers what the machine does: the states and which are terminal, `initial`, the
events, the variables and their values, and the rules. The machine's name, its section, a
state's kind, the worked examples, comments and the order anything is written in leave it
alone. `spine hash` prints it first, as `spine@<hash>`. When it changes, `spine stale` reports
every check that cites the old one; while it is current, `stale` counts every transition as
cited.
