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
| `vars` | no | each variable mapped to a short, finite list of values |
| `states` | yes | each state mapped to `{kind: fact|policy|design, terminal: true}` (`terminal` optional) |
| `initial` | yes | the starting state |
| `events` | yes | the list of event names |
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

Anything else (calls, attributes, arithmetic, undeclared names) is a `bad-condition`. Numeric
rules ("over 1,000") are split into named ranges at the thresholds the chapter gives, so the
cases stay enumerable. Quote values YAML would read as booleans (`"yes"`, `"no"`, `"on"`), and
write variable values as strings, since steps pass them as text.

## Steps, examples and behaviors

A step is `"<event> <var>=<value> ..."`, for example `"restore role=editor"`.

- **Examples** in the spine have `name`, `steps` and `expect`, where `expect` is the end state,
  `rejected` (some step is refused) or `undefined` (some step has no rule).
- **Behavior files** live in `spec/<machine>/behaviors/valid/` and `.../invalid/`, one YAML file
  each, with `steps` and an optional `expect` (end state). Files in `valid/` must be allowed and
  end as expected. Files in `invalid/` must not be: some step is refused or undecided, or the
  run ends somewhere other than `expect`.

## Readings

Unratified interpretations of a chapter, `spec/<machine>/readings/*.yaml`, use the same format.
`spine diff` compares two of them, or a reading against the ratified spine.

## Citing a cell

A content check states which cell it depends on, anywhere in the file:

```python
# covers: trashed/restore@95f9c8204e9f
```

`spine hash <spec>` prints the current hash of every cell. When the rules for a cell change,
its hash changes and `spine stale` reports every check that cites the old one.
