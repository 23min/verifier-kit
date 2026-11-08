package spine

// The drawing test (#108): every rule of a spine is in its drawing exactly once, read back from
// the SVG, as a line of an edge's label or an entry of the table; and a layout drawn twice gives
// the same drawing. It runs on every example's layout and on the cases' layouts that draw.

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type svgText struct {
	Chars string   `xml:",chardata"`
	Spans []string `xml:"tspan"`
}

type svgGroup struct {
	Class string    `xml:"class,attr"`
	Title string    `xml:"title"`
	Texts []svgText `xml:"text"`
}

// drawnRules reads an SVG's rules back: from, event, condition and outcome, one string each.
func drawnRules(t *testing.T, svg string) []string {
	t.Helper()
	var doc struct {
		Groups []svgGroup `xml:"g"`
	}
	if err := xml.Unmarshal([]byte(svg), &doc); err != nil {
		t.Fatalf("the drawing is not XML: %v", err)
	}
	var out []string
	for _, g := range doc.Groups {
		switch g.Class {
		case "move":
			from, to, ok := strings.Cut(g.Title, " → ")
			if !ok || len(g.Texts) != 1 {
				t.Fatalf("a move is drawn as %+v", g)
			}
			lines := g.Texts[0].Spans
			for i := 0; i < len(lines); i++ {
				when := ""
				if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "[") {
					when = strings.TrimSuffix(strings.TrimPrefix(lines[i+1], "["), "]")
				}
				out = append(out, strings.Join([]string{from, lines[i], when, "to " + to}, " | "))
				if when != "" {
					i++
				}
			}
		case "rules":
			from, event, ok := strings.Cut(g.Title, "/")
			if !ok {
				t.Fatalf("a table cell is drawn as %+v", g)
			}
			for _, x := range g.Texts {
				outcome, when, _ := strings.Cut(x.Chars, " [")
				out = append(out, strings.Join([]string{from, event, strings.TrimSuffix(when, "]"), outcome}, " | "))
			}
		}
	}
	slices.Sort(out)
	return out
}

// specRules is the spine's rules in the same form.
func specRules(s *Spec) []string {
	var out []string
	for _, r := range s.Rules {
		outcome := r.Outcome.Kind
		if outcome == "to" {
			outcome += " " + r.Outcome.Target
		}
		out = append(out, strings.Join([]string{r.From, r.Event, strings.Join(strings.Fields(r.WhenText), " "), outcome}, " | "))
	}
	slices.Sort(out)
	return out
}

func TestDrawings(t *testing.T) {
	layouts, _ := filepath.Glob(filepath.Join(repoRoot, "examples", "*", "spec", "*", "drawing.yaml"))
	if len(layouts) < 3 {
		t.Fatalf("found %d example layouts", len(layouts))
	}
	td := testdata(t)
	for _, name := range []string{"door", "bent", "crowded", "through", "wrapped/drawing"} {
		layouts = append(layouts, filepath.Join(td, "drawings", name+".yaml"))
	}
	for _, path := range layouts {
		spinePath := filepath.Join(filepath.Dir(path), "spine.yaml")
		if _, err := os.Stat(spinePath); err != nil {
			spinePath = filepath.Join(td, "seeds", "spine.yaml")
		}
		spec, problems := loadSpec(readFile(spinePath))
		if spec == nil {
			t.Fatalf("%s: %v", spinePath, problems)
		}
		layout, problems := loadLayout(readFile(path), spec)
		if layout == nil {
			t.Fatalf("%s: %v", path, problems)
		}
		svg, _ := drawing(spec, layout, path)
		if again, _ := drawing(spec, layout, path); again != svg {
			t.Errorf("%s: drawn twice, the drawings differ", path)
		}
		if got, want := drawnRules(t, svg), specRules(spec); !slices.Equal(got, want) {
			t.Errorf("%s: the drawing shows\n  %s\nwhere the spine has\n  %s", path, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
		}
	}
}
