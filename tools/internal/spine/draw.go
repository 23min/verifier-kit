package spine

// The drawing (#108): a spine as an SVG, its states where a layout puts them. A move to another
// state is an edge, labelled with its event and its condition as the spine writes it; every
// other rule is an entry in a table of events by state. Terminal states are double circles, and
// kinds are grey fills. A layout that can't be drawn is an error; a label that overlaps something
// or leaves the drawing is a warning, and the drawing is made all the same.

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/23min/verifier-kit/tools/internal/yamldoc"
)

var (
	layoutKeys = names("states", "edges")
	edgeKeys   = names("bend", "label")
)

type point struct{ x, y float64 }

// Layout is a validated layout: each state's centre, and for some edges a bend or a label's
// centre.
type Layout struct {
	at    map[string]point
	edges map[[2]string]edgeLayout // by the states an edge joins, from and to
}

type edgeLayout struct {
	bend  *float64
	label *point
}

// loadLayout loads and validates a layout for a valid spine. The layout is nil when there are
// errors.
func loadLayout(f file, s *Spec) (*Layout, Findings) {
	var out Findings
	v, ok := parseYAML(f, &out)
	if !ok {
		return nil, out
	}
	path := f.path
	if !isMap(v) {
		out.Add("spec-shape", path, `a layout must be a mapping with "states" and an optional "edges"`)
		return nil, out
	}
	checkKeys(v.Map, layoutKeys, path, "layout", &out, quoteHint)
	l := &Layout{at: map[string]point{}, edges: map[[2]string]edgeLayout{}}
	textKeys := func(k, _ *yamldoc.Value) bool { return isText(k) }
	states, ok := v.Map.Get("states")
	switch {
	case !ok:
		out.Add("missing-key", path, `the layout needs "states"`)
	case !isMap(states) || !allPairs(states.Map, textKeys):
		out.Add("spec-shape", path, `"states" must map each state to its centre, [x, y]`)
	default:
		for i, k := range states.Map.Keys {
			p, ok := position(states.Map.Values[i])
			switch {
			case !s.stateSet[k.Str]:
				out.Add("bad-value", path, fmt.Sprintf("states: %q is not a state of the spine", k.Str))
			case !ok:
				out.Add("spec-shape", path, fmt.Sprintf("states: %q must be at [x, y], two whole numbers", k.Str))
			default:
				l.at[k.Str] = p
			}
		}
		for _, st := range s.States {
			if !states.Map.Has(st) {
				out.Add("missing-key", path, fmt.Sprintf("states: %q has no centre", st))
			}
		}
	}
	moves := map[[2]string]bool{}
	for _, e := range edges(s) {
		moves[e.ends] = true
	}
	given, ok := v.Map.Get("edges")
	if !ok || given.Kind == yamldoc.KNull {
		if out.Errors() > 0 {
			return nil, out
		}
		return l, out
	}
	if !isMap(given) || !allPairs(given.Map, textKeys) {
		out.Add("spec-shape", path, `"edges" must map a state to the states it moves to, each to its bend and label`)
		return nil, out
	}
	for i, k := range given.Map.Keys {
		to := orEmpty(given.Map.Values[i])
		if !isMap(to) || !allPairs(to.Map, textKeys) {
			out.Add("spec-shape", path, fmt.Sprintf("edges: %q must map the states it moves to, each to its bend and label", k.Str))
			continue
		}
		for j, t := range to.Map.Keys {
			where := fmt.Sprintf("edges: %q to %q", k.Str, t.Str)
			e := orEmpty(to.Map.Values[j])
			if !isMap(e) {
				out.Add("spec-shape", path, where+" must be a mapping with a bend, a label or both")
				continue
			}
			checkKeys(e.Map, edgeKeys, path, "edge", &out, quoteHint)
			if !moves[[2]string{k.Str, t.Str}] {
				out.Add("bad-value", path, fmt.Sprintf("edges: no rule moves from %q to %q", k.Str, t.Str))
			}
			var el edgeLayout
			if b, ok := e.Map.Get("bend"); ok {
				if b.Kind != yamldoc.KInt {
					out.Add("spec-shape", path, where+": bend must be a whole number")
				}
				bend := float64(b.Int)
				el.bend = &bend
			}
			if lb, ok := e.Map.Get("label"); ok {
				p, ok := position(lb)
				if !ok {
					out.Add("spec-shape", path, where+": label must be at [x, y], two whole numbers")
				}
				el.label = &p
			}
			l.edges[[2]string{k.Str, t.Str}] = el
		}
	}
	if out.Errors() > 0 {
		return nil, out
	}
	return l, out
}

// position is a point given as [x, y], two whole numbers.
func position(v *yamldoc.Value) (point, bool) {
	if !isList(v) || len(v.Items) != 2 || v.Items[0].Kind != yamldoc.KInt || v.Items[1].Kind != yamldoc.KInt {
		return point{}, false
	}
	return point{float64(v.Items[0].Int), float64(v.Items[1].Int)}, true
}

// edge is the moves from one state to another, drawn as one arrow.
type edge struct {
	ends  [2]string
	moves []*Rule // in the spine's order
}

// edges are the spine's moves to another state, one edge for each pair of states, in the order
// of each pair's first move.
func edges(s *Spec) []*edge {
	var out []*edge
	byEnds := map[[2]string]*edge{}
	for _, r := range s.Rules {
		if r.Outcome.Kind != "to" || r.Outcome.Target == r.From {
			continue
		}
		ends := [2]string{r.From, r.Outcome.Target}
		if byEnds[ends] == nil {
			byEnds[ends] = &edge{ends: ends}
			out = append(out, byEnds[ends])
		}
		byEnds[ends].moves = append(byEnds[ends].moves, r)
	}
	return out
}

const (
	drawWidth = 860.0
	margin    = 24.0 // around the drawing's text
	radius    = 40.0 // a state's, unless its name needs a wider ellipse
	ringGap   = 5.0  // between a terminal state and its ring
	lead      = 18.0 // the height of a line of text
	pairBend  = 22.0 // how far each edge bows when two states move to each other
)

// textWidth estimates the width of s in the drawing's serif at size px: generously, as wide as
// DejaVu Serif, the widest serif a reader is likely to have.
func textWidth(s string, size float64) float64 {
	return float64(utf8.RuneCountInString(s)) * size * 0.56
}

// shape is a state's outline: an ellipse, a circle unless the name needs more width.
type shape struct {
	c      point
	rx, ry float64
}

// boundary is where the line from the centre in direction u leaves the outline.
func (sh shape) boundary(u point) point {
	t := 1 / math.Hypot(u.x/sh.rx, u.y/sh.ry)
	return point{sh.c.x + t*u.x, sh.c.y + t*u.y}
}

// inside reports whether p is inside the outline grown by grow.
func (sh shape) inside(p point, grow float64) bool {
	return math.Hypot((p.x-sh.c.x)/(sh.rx+grow), (p.y-sh.c.y)/(sh.ry+grow)) < 1
}

func unit(from, to point) point {
	d := math.Hypot(to.x-from.x, to.y-from.y)
	if d == 0 {
		return point{1, 0}
	}
	return point{(to.x - from.x) / d, (to.y - from.y) / d}
}

// box is the space a block of text takes, and what it is, for the warnings.
type box struct {
	x0, y0, x1, y1 float64
	what           string
}

// curve is a line drawn, as points along it, and what it is.
type curve struct {
	pts   []point
	what  string
	ends  []string // the states it joins, which it may touch
	label string   // its label's what, which it may touch
}

type drawer struct {
	s      *Spec
	where  string // the layout's path, for the warnings
	body   []string
	boxes  []box
	curves []curve
	shapes map[string]shape // each state's outer outline: its ring, for a terminal state
	warn   Findings
}

// drawing is the spine as an SVG with its states where the layout puts them, and the warnings
// about what is hard to read.
func drawing(s *Spec, l *Layout, where string) (string, Findings) {
	d := &drawer{s: s, where: where, shapes: map[string]shape{}}
	for _, st := range s.States {
		sh := shape{l.at[st], max(radius, textWidth(st, 18)/2+7), radius}
		if s.terminal[st] {
			sh.rx, sh.ry = sh.rx+ringGap, sh.ry+ringGap
		}
		d.shapes[st] = sh
	}
	title := "Machine " + s.Machine
	if s.Section != "" {
		title += ", section " + s.Section
	}
	d.text([]string{title}, margin, 34, "start", "ti", 17, "the title")
	for i, v := range s.Vars {
		d.text([]string{v.Name + " ∈ {" + strings.Join(v.Values, ", ") + "}"}, margin, 58+lead*float64(i), "start", "tx", 14, "the variables")
	}
	d.legend()
	d.start()
	all := edges(s)
	joined := map[[2]string]bool{}
	for _, e := range all {
		joined[e.ends] = true
	}
	for _, e := range all {
		d.edge(e, l, joined[[2]string{e.ends[1], e.ends[0]}])
	}
	for _, st := range s.States {
		d.state(st)
	}
	height := d.table()
	d.check()
	bg := fmt.Sprintf(`<rect class="bg" x="0" y="0" width="%s" height="%s"/>`, num(drawWidth), num(height))
	svg := strings.Join(append([]string{bg}, d.body...), "\n  ")
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %s %s" width="%s" height="%s">
  <title>%s</title>
  <style>%s  </style>
  <defs>
    <marker id="arr" viewBox="0 0 10 8" refX="10" refY="4" markerWidth="10" markerHeight="8" markerUnits="userSpaceOnUse" orient="auto">
      <path class="mk" d="M0,0 L10,4 L0,8 z"/>
    </marker>
  </defs>
  %s
</svg>
`, num(drawWidth), num(height), num(drawWidth), num(height), xmlText(title), drawStyle, svg), d.warn
}

const drawStyle = `
    :root {
      --surface: #ffffff; --ink: #141414; --ink-2: #333333; --muted: #6a6a6a; --rule: #c4c4c4;
      --fact: #b4b4b4; --policy: #dadada; --design: #f2f2f2;
      --serif: "Latin Modern Roman", "CMU Serif", "Times New Roman", Times, serif;
    }
    @media (prefers-color-scheme: dark) {
      :root { --surface: #161616; --ink: #ececec; --ink-2: #cfcfcf; --muted: #9d9d9d; --rule: #444444;
              --fact: #5a5a5a; --policy: #3a3a3a; --design: #242424; }
    }
    .bg, .none { fill: var(--surface); }
    .fact { fill: var(--fact); } .policy { fill: var(--policy); } .design { fill: var(--design); }
    .st { stroke: var(--ink); stroke-width: 1.4; }
    .ring { fill: none; stroke: var(--ink); stroke-width: 1.4; }
    .e { fill: none; stroke: var(--ink-2); stroke-width: 1.15; }
    .mk { fill: var(--ink-2); }
    .rule { stroke: var(--rule); stroke-width: 1; }
    .sn { font: italic 400 18px var(--serif); fill: var(--ink); }
    .lb { font: 400 14px var(--serif); fill: var(--ink); paint-order: stroke; stroke: var(--surface); stroke-width: 5px; stroke-linejoin: round; }
    .ti { font: 400 17px var(--serif); fill: var(--ink); }
    .tx { font: 400 14px var(--serif); fill: var(--ink); }
    .it { font-style: italic; }
    .nt { font: 400 13px var(--serif); fill: var(--muted); }
`

// num is a coordinate to a tenth, without a trailing .0 or a minus zero.
func num(x float64) string {
	return strconv.FormatFloat(math.Round(x*10)/10+0, 'f', -1, 64)
}

func (d *drawer) add(format string, a ...any) {
	d.body = append(d.body, fmt.Sprintf(format, a...))
}

// text draws lines of text, the first on baseline y, and keeps the space they take.
func (d *drawer) text(lines []string, x, y float64, anchor, class string, size float64, what string) {
	w := 0.0
	for _, line := range lines {
		w = max(w, textWidth(line, size))
	}
	x0 := map[string]float64{"start": x, "middle": x - w/2}[anchor]
	d.boxes = append(d.boxes, box{x0 - 2, y - size, x0 + w + 2, y + lead*float64(len(lines)-1) + 4, what})
	var b strings.Builder
	fmt.Fprintf(&b, `<text class="%s" x="%s" y="%s" text-anchor="%s">`, class, num(x), num(y), anchor)
	for i, line := range lines {
		dy := "0"
		if i > 0 {
			dy = num(lead)
		}
		fmt.Fprintf(&b, `<tspan x="%s" dy="%s">%s</tspan>`, num(x), dy, xmlText(line))
	}
	b.WriteString("</text>")
	d.body = append(d.body, b.String())
}

// legend explains the fills of the kinds the spine uses, top right.
func (d *drawer) legend() {
	words := map[string]string{"fact": "fact: irreversible in the world", "policy": "policy: a business choice",
		"design": "design: an implementation choice", "none": "no kind"}
	var rows []string
	for _, k := range []string{"fact", "policy", "design", "none"} {
		for _, st := range d.s.States {
			if kindOf(d.s, st) == k {
				rows = append(rows, k)
				break
			}
		}
	}
	w := 0.0
	for _, k := range rows {
		w = max(w, textWidth(words[k], 13))
	}
	x := drawWidth - margin - 24 - w
	for i, k := range rows {
		y := 56 + 22*float64(i)
		d.add(`<circle class="st %s" cx="%s" cy="%s" r="8"/>`, k, num(x+8), num(y-4))
		d.text([]string{words[k]}, x+24, y, "start", "nt", 13, "the legend")
	}
}

// kindOf is a state's kind, or none.
func kindOf(s *Spec, st string) string {
	if k, ok := s.kind[st]; ok {
		return k
	}
	return "none"
}

// start draws the arrow into the initial state, from its left.
func (d *drawer) start() {
	sh := d.shapes[d.s.Initial]
	tip := sh.boundary(point{-1, 0})
	tip.x--
	tail := point{tip.x - 44, tip.y}
	d.add(`<path class="e" d="M%s,%s L%s,%s" marker-end="url(#arr)"/>`, num(tail.x), num(tail.y), num(tip.x), num(tip.y))
	var pts []point
	for k := 0; k <= 20; k++ {
		pts = append(pts, point{tail.x + (tip.x-tail.x)*float64(k)/20, tail.y})
	}
	d.curves = append(d.curves, curve{pts: pts, what: "the start arrow", ends: []string{d.s.Initial}})
}

// edge draws the moves from one state to another as one arrow, bowed to the left of travel by
// its bend, and its label, each move's event with its condition below it. When the second state
// also moves to the first, the bend is pairBend unless the layout gives one.
func (d *drawer) edge(e *edge, l *Layout, pair bool) {
	from, to := e.ends[0], e.ends[1]
	a, b := d.shapes[from], d.shapes[to]
	el := l.edges[e.ends]
	bend := 0.0
	if el.bend != nil {
		bend = *el.bend
	} else if pair {
		bend = pairBend
	}
	u := unit(a.c, b.c)
	n := point{u.y, -u.x} // to the left of travel, as y grows downward
	mid := point{(a.c.x + b.c.x) / 2, (a.c.y + b.c.y) / 2}
	c := point{mid.x + n.x*2*bend, mid.y + n.y*2*bend}
	s := a.boundary(unit(a.c, c))
	ub := unit(b.c, c)
	t := b.boundary(ub)
	t = point{t.x + ub.x, t.y + ub.y} // the arrowhead's tip, just off the outline
	var pts []point
	for k := 0; k <= 40; k++ {
		f := float64(k) / 40
		pts = append(pts, point{(1-f)*(1-f)*s.x + 2*(1-f)*f*c.x + f*f*t.x, (1-f)*(1-f)*s.y + 2*(1-f)*f*c.y + f*f*t.y})
	}
	what := from + " → " + to
	var lines []string
	for _, r := range e.moves {
		lines = append(lines, r.Event)
		if r.WhenText != "" {
			lines = append(lines, "["+strings.Join(strings.Fields(r.WhenText), " ")+"]")
		}
	}
	w := 0.0
	for _, line := range lines {
		w = max(w, textWidth(line, 14))
	}
	h := 14 + lead*float64(len(lines)-1) + 4
	centre := point{}
	if el.label != nil {
		centre = *el.label
	} else {
		side := 1.0
		if bend < 0 {
			side = -1
		}
		apex := pts[20]
		off := 6 + math.Abs(n.x)*w/2 + math.Abs(n.y)*h/2
		centre = point{apex.x + side*n.x*off, apex.y + side*n.y*off}
	}
	d.curves = append(d.curves, curve{pts: pts, what: "the edge " + what, ends: e.ends[:], label: "the label of " + what})
	d.add(`<g class="move"><title>%s</title>`, xmlText(what))
	d.add(`<path class="e" d="M%s,%s Q%s,%s %s,%s" marker-end="url(#arr)"/>`, num(s.x), num(s.y), num(c.x), num(c.y), num(t.x), num(t.y))
	d.text(lines, centre.x, centre.y-h/2+14, "middle", "lb", 14, "the label of "+what)
	d.add(`</g>`)
}

// state draws a state: filled by its kind, ringed when terminal, its name inside.
func (d *drawer) state(st string) {
	sh := d.shapes[st]
	rx, ry := sh.rx, sh.ry
	if d.s.terminal[st] {
		rx, ry = rx-ringGap, ry-ringGap
	}
	d.add(`<ellipse class="st %s" cx="%s" cy="%s" rx="%s" ry="%s"/>`, kindOf(d.s, st), num(sh.c.x), num(sh.c.y), num(rx), num(ry))
	if d.s.terminal[st] {
		d.add(`<ellipse class="ring" cx="%s" cy="%s" rx="%s" ry="%s"/>`, num(sh.c.x), num(sh.c.y), num(sh.rx), num(sh.ry))
	}
	d.add(`<text class="sn" x="%s" y="%s" text-anchor="middle">%s</text>`, num(sh.c.x), num(sh.c.y+6), xmlText(st))
}

// table draws every rule that doesn't move to another state, by event and state, below
// everything else, and gives the drawing's height.
func (d *drawer) table() float64 {
	s := d.s
	top := 0.0
	for _, b := range d.boxes {
		top = max(top, b.y1)
	}
	for _, sh := range d.shapes {
		top = max(top, sh.c.y+sh.ry)
	}
	for _, c := range d.curves {
		for _, p := range c.pts {
			top = max(top, p.y)
		}
	}
	top += 40
	stays := func(st, ev string) []*Rule {
		var out []*Rule
		for _, r := range s.Rules {
			if r.From == st && r.Event == ev && (r.Outcome.Kind != "to" || r.Outcome.Target == st) {
				out = append(out, r)
			}
		}
		return out
	}
	var cols, bare []string // the states with such rules, and the terminal states with no rule at all
	for _, st := range s.States {
		has, some := false, false
		for _, r := range s.Rules {
			if r.From == st {
				some = true
				has = has || r.Outcome.Kind != "to" || r.Outcome.Target == st
			}
		}
		switch {
		case has:
			cols = append(cols, st)
		case !some && s.terminal[st]:
			bare = append(bare, st)
		}
	}
	notes := []string{"Every rule that doesn't change the state, by event and state. The moves are the edges above."}
	switch len(bare) {
	case 0:
	case 1:
		notes = append(notes, bare[0]+" is terminal and has no rules.")
	default:
		notes = append(notes, strings.Join(bare[:len(bare)-1], ", ")+" and "+bare[len(bare)-1]+" are terminal and have no rules.")
	}
	d.text(notes, margin, top, "start", "nt", 13, "the table's note")
	y := top + lead*float64(len(notes)-1) + 30
	first := textWidth("event", 14)
	for _, ev := range s.Events {
		first = max(first, textWidth(ev, 14))
	}
	first += 24
	right := drawWidth - margin
	cw := (right - margin - first) / float64(max(len(cols), 1))
	at := func(k int) float64 { return margin + first + cw*float64(k) }
	d.add(`<text class="tx it" x="%s" y="%s">event</text>`, num(margin), num(y))
	for k, st := range cols {
		d.add(`<text class="tx it" x="%s" y="%s">%s</text>`, num(at(k)), num(y), xmlText(st))
	}
	head := y - 14
	y += 8
	d.add(`<line class="rule" x1="%s" y1="%s" x2="%s" y2="%s"/>`, num(margin), num(y), num(right), num(y))
	for i, ev := range s.Events {
		rowTop, lines := y, 1
		d.add(`<text class="tx" x="%s" y="%s">%s</text>`, num(margin), num(rowTop+lead+2), xmlText(ev))
		for k, st := range cols {
			rules := stays(st, ev)
			if len(rules) == 0 {
				continue
			}
			lines = max(lines, len(rules))
			d.add(`<g class="rules"><title>%s</title>`, xmlText(st+"/"+ev))
			for m, r := range rules {
				words := r.Outcome.Kind
				if words == "to" {
					words += " " + r.Outcome.Target
				}
				if r.WhenText != "" {
					words += " [" + strings.Join(strings.Fields(r.WhenText), " ") + "]"
				}
				if textWidth(words, 14) > cw-10 {
					d.warn.AddSeverity("layout-overlap", d.where, fmt.Sprintf("the table's entry %q for %s/%s is wider than its column", words, st, ev), "warning")
				}
				d.add(`<text class="tx" x="%s" y="%s">%s</text>`, num(at(k)), num(rowTop+lead*float64(m+1)+2), xmlText(words))
			}
			d.add(`</g>`)
		}
		y = rowTop + lead*float64(lines) + 10
		if i+1 < len(s.Events) {
			d.add(`<line class="rule" x1="%s" y1="%s" x2="%s" y2="%s"/>`, num(margin), num(y), num(right), num(y))
		}
	}
	d.boxes = append(d.boxes, box{margin, head, right, y + 4, "the table"})
	return y + 24
}

// check adds a warning for each label that overlaps a label, a state or an edge, each edge that
// runs through a state, each state that overlaps another, and anything that leaves the drawing.
func (d *drawer) check() {
	said := map[string]bool{} // a label over two rows of the legend is said once
	warn := func(code, format string, a ...any) {
		if m := fmt.Sprintf(format, a...); !said[m] {
			said[m] = true
			d.warn.AddSeverity(code, d.where, m, "warning")
		}
	}
	s := d.s
	for i, a := range s.States {
		sa := d.shapes[a]
		if sa.c.x-sa.rx < 4 || sa.c.x+sa.rx > drawWidth-4 || sa.c.y-sa.ry < 4 {
			warn("layout-outside", "state %s leaves the drawing", a)
		}
		for _, b := range s.States[i+1:] {
			for k := 0; k < 48; k++ {
				th := 2 * math.Pi * float64(k) / 48
				if d.shapes[b].inside(point{sa.c.x + sa.rx*math.Cos(th), sa.c.y + sa.ry*math.Sin(th)}, 0) {
					warn("layout-overlap", "state %s overlaps state %s", a, b)
					break
				}
			}
		}
	}
	for _, c := range d.curves {
		for _, st := range s.States {
			if slices.Contains(c.ends, st) {
				continue
			}
			for _, p := range c.pts {
				if d.shapes[st].inside(p, 0) {
					warn("layout-overlap", "%s runs through state %s", c.what, st)
					break
				}
			}
		}
	}
	for i, a := range d.boxes {
		if a.x0 < 4 || a.x1 > drawWidth-4 || a.y0 < 4 {
			warn("layout-outside", "%s leaves the drawing", a.what)
		}
		for _, b := range d.boxes[i+1:] {
			if a.what != b.what && a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1 {
				warn("layout-overlap", "%s overlaps %s", a.what, b.what)
			}
		}
		for _, st := range s.States {
			sh := d.shapes[st]
			near := point{min(max(sh.c.x, a.x0), a.x1), min(max(sh.c.y, a.y0), a.y1)}
			if sh.inside(near, 2) {
				warn("layout-overlap", "%s overlaps state %s", a.what, st)
			}
		}
		for _, c := range d.curves {
			if c.label == a.what {
				continue
			}
			for _, p := range c.pts[3 : len(c.pts)-3] {
				if a.x0+3 < p.x && p.x < a.x1-3 && a.y0+3 < p.y && p.y < a.y1-3 {
					warn("layout-overlap", "%s runs under %s", c.what, a.what)
					break
				}
			}
		}
	}
}
