# Drawing layout

A spine's drawing shows it as a machine, for whoever reads it to ratify it: `spine draw`
(`tool-contract.md`) makes it from the spine and a layout file that says where each state sits.
Whoever keeps the spine keeps the layout, a person or an LLM; the drawing is committed beside
them, and `./verify` checks that it is what `spine draw` makes. Changing this format needs an
issue and the owner's approval first.

## Files

`spec/<machine>/drawing.yaml`, the layout, and `spec/<machine>/drawing.svg`, the drawing, beside
the spine and owned with it. From that folder, `spine draw spine.yaml drawing.yaml > drawing.svg`
writes the drawing again; no path is drawn, so it is the same from any folder.

## The layout

One YAML document, read as the spine format reads every file (`spine-format.md`, "Text only"),
except that a coordinate is a whole number in decimal: digits, a `-` in front of a negative one,
and no leading zero (YAML 1.1 reads `012` as octal). Top-level keys, and nothing else:

| Key | Required | Holds |
|---|---|---|
| `states` | yes | every state of the spine, and no other, mapped to its centre, `[x, y]` |
| `edges` | no | a state mapped to states it moves to, each to `{bend: N, label: [x, y]}`, both optional |

```yaml
states:
  draft:   [100, 230]
  active:  [330, 230]
  trashed: [550, 230]
  purged:  [785, 230]
```

The drawing is 860 units wide, x to the right and y downward from its top left corner; its height
follows from what it holds. The title, the variables and the legend take the top 110 or so.

One edge carries every move from one state to another. It is straight, unless the second state
also moves to the first: then each of the two bows 22 to the left of its travel, so they don't
meet. `bend` sets how far the edge's middle bows, to the left of travel when positive and to the
right when negative; 0 is straight. The label sits beside the edge's middle, on the side it bows
to, or to the left of travel when it is straight; `label` sets its centre instead.

## The drawing

- **The title,** `Machine <machine>, section <section>`, or without the section when the spine
  gives none; then each variable with its values,
  `role ∈ {owner, editor, viewer}`, and in the top right a legend of the kinds the spine uses.
- **Each state** a circle with its name inside, widened into an ellipse when the name needs it;
  filled by its kind, fact darkest, then policy, then design, and unfilled when it has none; a
  terminal state with a second outline. An arrow from the left points at the initial state.
- **Each move to another state** an arrow, labelled with its event and, on the line below, its
  `when` in brackets, as the spine writes it with each run of spaces and line breaks as one space:
  `publish` above `[role == 'owner']`. Moves from one state to another share their arrow, one
  label line or pair for each, in the spine's order.
- **Every other rule,** a stay, an ignore, a refusal or a move to its own state, in a table below:
  a row for each event, a column for each state with such a rule, and in the cell each rule as
  the spine names its outcome (`stay`, `ignore`, `reject`, `to <state>`) with its `when` in
  brackets, in the spine's order. A note above the table names the terminal states with no rules.

So each rule is drawn exactly once. In the SVG, each arrow is a `<g class="move">` titled
`<from> → <to>`, and each table cell a `<g class="rules">` titled `<state>/<event>`. The colours
switch with the reader's light or dark setting. Text is in the reader's serif, Latin Modern,
Times or another, at 14 px for labels and the table.

## Warnings

`spine draw` estimates the width of text as generously as DejaVu Serif, the widest serif a reader
is likely to have, and warns where the drawing is hard to read: `layout-overlap` when a label
overlaps a label, a state or an edge, an edge runs through a state, a state overlaps another, or
a table entry is wider than its column; `layout-outside` when a label or a state leaves the
drawing. A warning names the label or state, so the layout can move it; the drawing is made all
the same. A layout that can't be drawn, a state with no centre, a state or edge the spine doesn't
have, or a coordinate or bend that isn't a whole number, is an error, and nothing is drawn.
