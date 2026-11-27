package declarative_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// listLike is an adapter and a config read of the common shape:
// an object of rules, each a list of its severity word then its options, with
// the word none for off. No test below names a tool.
func listLike(off ...string) (model.Adapter, model.ConfigRead) {
	a := model.Adapter{Tool: "lint-a", Severities: model.SeverityScale{
		Form: model.SeverityLevel,
		Levels: []model.SeverityLevelName{
			{Name: "error", Is: model.SeverityError}, {Name: "warn", Is: model.SeverityWarning},
			{Name: "hi", Is: model.SeverityError}, {Name: "lo", Is: model.SeverityWarning},
		},
		Off: off,
	}}
	r := model.ConfigRead{Target: model.TargetSource, Format: model.ConfigJSON,
		Entries: model.EntryMap{Each: "/rules", Value: "/0", Options: "/1", Severity: "/0"}}
	return a, r
}

func entry(key string, sev model.Severity, opts map[string]any) model.PresetEntry {
	return model.PresetEntry{Binding: model.Binding{Tool: "lint-a", Target: model.TargetSource, Key: key, Severity: sev, Options: opts},
		Layer: "base", Decision: "d-" + key}
}

// words is the word of each observation by key, or "?" where it has none.
func words(o model.Observed) map[string]string {
	out := map[string]string{}
	for _, e := range o.Entries {
		w := string(e.Word)
		if w == "" {
			w = "?"
		}
		out[e.Key] = w
	}
	return out
}

// TestObserveGivesEachOfTheSixWordsFromAdapterDataAlone: each word, with the
// severity order read through the adapter's levels, an off word below every
// level, and options compared as sets of leaves, a list item by its place:
// a list in another order has no word.
func TestObserveGivesEachOfTheSixWordsFromAdapterDataAlone(t *testing.T) {
	t.Parallel()
	a, r := listLike("none")
	excluded := entry("dropped", model.SeverityError, nil)
	excluded.Excluded = &model.ExcludedBy{Overlay: "team", Decision: "why"}
	keptOut := entry("dropped-but-set", model.SeverityError, nil)
	keptOut.Excluded = &model.ExcludedBy{Overlay: "team", Decision: "why"}
	preset := []model.PresetEntry{
		entry("as-set", model.SeverityError, map[string]any{"a": []any{"x", "y"}}),
		entry("reordered", model.SeverityError, map[string]any{"a": []any{"x", "y"}}),
		entry("tighter-severity", model.SeverityWarning, nil),
		entry("tighter-options", model.SeverityError, map[string]any{"a": []any{"x", "y"}}),
		entry("weaker-severity", model.SeverityError, nil),
		entry("weaker-off", model.SeverityError, nil),
		entry("weaker-options", model.SeverityError, map[string]any{"a": []any{"x"}}),
		entry("absent", model.SeverityError, nil),
		excluded, keptOut,
	}
	config := `{"rules":{"as-set":["error",{"a":["x","y"]}],"reordered":["error",{"a":["y","x"]}],"tighter-severity":["error"],"tighter-options":["error",{"a":["x"]}],
		"weaker-severity":["warn"],"weaker-off":["none"],"weaker-options":["error",{"a":["x","z"]}],"dropped-but-set":["error"],"unmapped":["error"]}}`
	got := words(declarative.Observe(a, r, []byte(config), preset))
	want := map[string]string{"as-set": "as-set", "reordered": "?", "tighter-severity": "tighter", "tighter-options": "tighter",
		"weaker-severity": "weaker", "weaker-off": "weaker", "weaker-options": "weaker", "absent": "absent",
		"dropped": "excluded", "dropped-but-set": "unmapped", "unmapped": "unmapped"}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %q, want %q", k, got[k], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d observations, want %d: %v", len(got), len(want), got)
	}
}

// TestAnExcludedRuleNamesBothAndAStillSetEntryIsUnmappedWasExcludedBy: the
// provenance of an excluded entry names the overlay and the decision, whether
// the config lacks it (excluded) or still holds it (unmapped).
func TestAnExcludedRuleNamesBothAndAStillSetEntryIsUnmappedWasExcludedBy(t *testing.T) {
	t.Parallel()
	a, r := listLike()
	p := entry("k", model.SeverityError, nil)
	p.Excluded = &model.ExcludedBy{Overlay: "team", Decision: "legacy"}
	for config, want := range map[string]model.ObservationWord{`{"rules":{}}`: model.ObservedExcluded, `{"rules":{"k":["error"]}}`: model.ObservedUnmapped} {
		o := declarative.Observe(a, r, []byte(config), []model.PresetEntry{p})
		if len(o.Entries) != 1 || o.Entries[0].Word != want || o.Entries[0].Provenance.ExcludedBy == nil ||
			*o.Entries[0].Provenance.ExcludedBy != *p.Excluded {
			t.Errorf("%s: %+v, want %s naming team and legacy", config, o, want)
		}
	}
	// One live rule among the dropped holds the entry: it is observed, not excluded.
	live := entry("k", model.SeverityError, nil)
	o := declarative.Observe(a, r, []byte(`{"rules":{"k":["error"]}}`), []model.PresetEntry{p, live})
	if len(o.Entries) != 1 || o.Entries[0].Word != model.ObservedAsSet {
		t.Errorf("an entry held by a live rule and a dropped one: %+v, want as-set", o)
	}
}

// TestObserveNeverGuessesAWordItCannotEstablish: a config that does not read
// is Unknown with a reason and no entries; an entry whose standing cannot be
// established carries a reason and no word; and an adapter with no off list
// reads nothing as off.
func TestObserveNeverGuessesAWordItCannotEstablish(t *testing.T) {
	t.Parallel()
	a, r := listLike()
	for name, config := range map[string]string{"not json": `{"rules":`, "no rules": `{"x":1}`, "empty": ``} {
		o := declarative.Observe(a, r, []byte(config), []model.PresetEntry{entry("k", model.SeverityError, nil)})
		if o.Unknown == "" || len(o.Entries) != 0 {
			t.Errorf("%s: %+v, want Unknown with a reason and no entries", name, o)
		}
	}
	if o := declarative.Observe(a, model.ConfigRead{}, []byte(`{}`), nil); !strings.Contains(o.Unknown, "reads no effective config") {
		t.Errorf("an adapter with no config read: %+v", o)
	}
	cases := map[string]struct {
		config, says string
		p            model.PresetEntry
	}{
		"a word that is no level":               {`{"rules":{"k":["loud"]}}`, `no level and no off word`, entry("k", model.SeverityError, nil)},
		"the off word of a tool with none":      {`{"rules":{"k":["none"]}}`, `no level and no off word`, entry("k", model.SeverityError, nil)},
		"options on one side only":              {`{"rules":{"k":["error",{"a":1}]}}`, "on one side only", entry("k", model.SeverityError, nil)},
		"options neither inside nor containing": {`{"rules":{"k":["error",{"a":1}]}}`, "neither contain", entry("k", model.SeverityError, map[string]any{"a": 2})},
		"a value that differs":                  {`{"rules":{"k":["error"]}}`, "gives no order for values", withValue(entry("k", model.SeverityError, nil), "off")},
		"an entry held twice":                   {`{"rules":{"k":["error"],"k":["warn"]}}`, "", entry("k", model.SeverityError, nil)},
	}
	for name, c := range cases {
		o := declarative.Observe(a, r, []byte(c.config), []model.PresetEntry{c.p})
		if name == "an entry held twice" {
			// A JSON object with one name twice is refused by the reader whole.
			if o.Unknown == "" {
				t.Errorf("%s: %+v, want Unknown", name, o)
			}
			continue
		}
		if len(o.Entries) != 1 || o.Entries[0].Word != "" || !strings.Contains(o.Entries[0].Reason, c.says) {
			t.Errorf("%s: %+v, want no word and a reason saying %q", name, o, c.says)
		}
	}
	// A looser aspect settles the word though another aspect cannot be read.
	p := entry("k", model.SeverityError, map[string]any{"a": 2})
	if o := declarative.Observe(a, r, []byte(`{"rules":{"k":["warn",{"a":1}]}}`), []model.PresetEntry{p}); o.Entries[0].Word != model.ObservedWeaker {
		t.Errorf("a lowered severity with options that cannot be compared: %+v, want weaker", o)
	}
}

func withValue(p model.PresetEntry, v any) model.PresetEntry {
	p.Binding.Value = v
	return p
}

// TestObserveOnlyReadsPresetEntriesOfItsToolAndTarget: an entry of another
// tool or another target is not this config's.
func TestObserveOnlyReadsPresetEntriesOfItsToolAndTarget(t *testing.T) {
	t.Parallel()
	a, r := listLike()
	other := entry("k", model.SeverityError, nil)
	other.Binding.Tool = "lint-b"
	elsewhere := entry("j", model.SeverityError, nil)
	elsewhere.Binding.Target = model.TargetConfig
	o := declarative.Observe(a, r, []byte(`{"rules":{}}`), []model.PresetEntry{other, elsewhere})
	if len(o.Entries) != 0 || o.Unknown != "" {
		t.Errorf("entries of another tool and target: %+v, want none", o)
	}
}

// rng is a small deterministic generator, so a failing property names its seed.
type rng uint64

func (r *rng) next() uint64 {
	*r ^= *r << 13
	*r ^= *r >> 7
	*r ^= *r << 17
	return uint64(*r)
}

func (r *rng) n(n int) int { return int(r.next() % uint64(n)) }

// TestObservingThePresetAgainstItselfIsAllAsSet (tests.generated.2): for
// generated presets, the config the preset would print reads, entry for
// entry, as-set. The config is made from the adapter's own levels, so it is
// the preset as the tool states it.
func TestObservingThePresetAgainstItselfIsAllAsSet(t *testing.T) {
	t.Parallel()
	a, r := listLike("none")
	for seed := 1; seed <= 200; seed++ {
		g := rng(uint64(seed) * 2654435761)
		var preset []model.PresetEntry
		rules := map[string]any{}
		for i := range 1 + g.n(12) {
			key := fmt.Sprintf("rule-%d", i)
			sev := []model.Severity{model.SeverityError, model.SeverityWarning}[g.n(2)]
			var opts map[string]any
			if g.n(2) == 0 {
				opts = map[string]any{}
				for j := range 1 + g.n(3) {
					list := []any{}
					for k := range 1 + g.n(3) {
						list = append(list, fmt.Sprintf("v%d", k+g.n(2)))
					}
					opts[fmt.Sprintf("o%d", j)] = list
				}
			}
			preset = append(preset, entry(key, sev, opts))
			word := slices.IndexFunc(a.Severities.Levels, func(l model.SeverityLevelName) bool { return l.Is == sev })
			member := []any{a.Severities.Levels[word].Name}
			if opts != nil {
				member = append(member, opts)
			}
			rules[key] = member
		}
		config, err := json.Marshal(map[string]any{"rules": rules})
		if err != nil {
			t.Fatal(err)
		}
		o := declarative.Observe(a, r, config, preset)
		if o.Unknown != "" || len(o.Entries) != len(preset) {
			t.Fatalf("seed %d: %d entries, unknown %q, want %d entries\n%s", seed, len(o.Entries), o.Unknown, len(preset), config)
		}
		for _, e := range o.Entries {
			if e.Word != model.ObservedAsSet {
				t.Fatalf("seed %d: %s is %q (%s), want as-set\n%s", seed, e.Key, e.Word, e.Reason, config)
			}
		}
	}
}

// FuzzObserve: whatever a tool printed, Observe neither panics nor answers an
// entry that has both a word and a reason, or neither, or a word outside the
// six, and an Unknown answer holds no entry (tests.generated.1).
func FuzzObserve(f *testing.F) {
	f.Add([]byte(`{"rules":{"a":["error",{"x":["y"]}],"b":["none"]}}`))
	f.Add([]byte(`{"rules":{"a":["loud",{}]}}`))
	f.Add([]byte(`{"rules":{"a":["warn"],"c":["error",{"x":["z"]}]}}`))
	a, r := listLike("none")
	preset := []model.PresetEntry{
		entry("a", model.SeverityError, map[string]any{"x": []any{"y"}}),
		entry("b", model.SeverityWarning, nil),
		entry("d", model.SeverityError, nil),
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		o := declarative.Observe(a, r, data, preset)
		if o.Unknown != "" {
			if len(o.Entries) != 0 {
				t.Fatalf("Unknown %q with %d entries", o.Unknown, len(o.Entries))
			}
			return
		}
		for _, e := range o.Entries {
			known := slices.Contains(model.ObservationWords(), e.Word)
			if known == (e.Reason != "") || (!known && e.Word != "") {
				t.Fatalf("an observation %+v has a word and a reason, neither, or a word outside the six", e)
			}
		}
	})
}

// TestTheObserveFuzzSeedsAreTheCheckedInOnes: the seeds stay checked in.
func TestTheObserveFuzzSeedsAreTheCheckedInOnes(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("testdata/fuzz/FuzzObserve")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	if want := []string{"loud", "mixed", "off"}; !slices.Equal(names, want) {
		t.Errorf("FuzzObserve seeds = %v, want %v", names, want)
	}
}

// displayOnlyNames is each field name the model's observation types mark
// display only and that no field there decides on, so a selection of the name
// is a read of a field that decides nothing (arch.ports.19).
func displayOnlyNames(t *testing.T) map[string]bool {
	t.Helper()
	shows, decides := map[string]bool{}, map[string]bool{}
	for _, name := range []string{"observation.go", "observed.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), "internal/model/"+name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, fl := range st.Fields.List {
				doc := strings.ToLower(fl.Doc.Text())
				for _, id := range fl.Names {
					if strings.Contains(doc, "decides:") {
						decides[id.Name] = true
					} else if strings.Contains(doc, "display only") {
						shows[id.Name] = true
					}
				}
			}
			return true
		})
	}
	for n := range decides {
		delete(shows, n)
	}
	return shows
}

// displayReads is each place in src where a condition, a switch, a case, a
// comparison or a logical operator reads a field named in names, as
// "line: reads Name".
func displayReads(t *testing.T, src string, names map[string]bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("the source does not parse: %v", err)
	}
	var out []string
	flag := func(e ast.Node) {
		ast.Inspect(e, func(n ast.Node) bool {
			if s, ok := n.(*ast.SelectorExpr); ok && names[s.Sel.Name] {
				out = append(out, fmt.Sprintf("%d: reads %s", fset.Position(s.Pos()).Line, s.Sel.Name))
			}
			return true
		})
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt:
			flag(x.Cond)
		case *ast.ForStmt:
			if x.Cond != nil {
				flag(x.Cond)
			}
		case *ast.SwitchStmt:
			if x.Tag != nil {
				flag(x.Tag)
			}
		case *ast.CaseClause:
			for _, e := range x.List {
				flag(e)
			}
		case *ast.BinaryExpr:
			flag(x)
		}
		return true
	})
	slices.Sort(out)
	return slices.Compact(out)
}

// TestObserveDecidesOnNoFieldThatIsDisplayOnly (arch.ports.19): nothing
// in observe.go reads a display-only field in a condition, a switch, a case
// or a comparison; it only copies them into the answer. The same reading,
// over a planted source, finds each form of read, and a copy is clean.
func TestObserveDecidesOnNoFieldThatIsDisplayOnly(t *testing.T) {
	t.Parallel()
	names := displayOnlyNames(t)
	for _, want := range []string{"Layer", "Decision", "Reason", "Overlay"} {
		if !names[want] {
			t.Fatalf("%s is not read as a display-only field: %v", want, names)
		}
	}
	for _, deciding := range []string{"Key", "Word", "Severity", "Excluded"} {
		if names[deciding] {
			t.Errorf("%s decides somewhere and must not be taken as display only", deciding)
		}
	}
	if got := displayReads(t, string(read(t, "observe.go")), names); len(got) > 0 {
		t.Errorf("observe.go reads display-only fields: %v", got)
	}
	for name, c := range map[string]struct {
		body string
		want []string
	}{
		"an if":              {"func f(p model.PresetEntry) bool {\n\tif p.Layer == \"x\" {\n\t\treturn true\n\t}\n\treturn false\n}\n", []string{"6: reads Layer"}},
		"a switch":           {"func f(p model.PresetEntry) int {\n\tswitch p.Decision {\n\tcase \"a\":\n\t\treturn 1\n\t}\n\treturn 0\n}\n", []string{"6: reads Decision"}},
		"a comparison alone": {"func f(o model.Observation) bool {\n\tok := o.Reason != \"\"\n\treturn ok\n}\n", []string{"6: reads Reason"}},
		"a copy is clean":    {"func f(p model.PresetEntry) model.Provenance {\n\treturn model.Provenance{Layer: p.Layer, Decision: p.Decision}\n}\n", nil},
		"a deciding field":   {"func f(p model.PresetEntry) bool {\n\tif p.Excluded != nil {\n\t\treturn true\n\t}\n\treturn false\n}\n", nil},
	} {
		src := "package planted\n\nimport \"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model\"\n\n" + c.body
		if got := displayReads(t, src, names); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}
