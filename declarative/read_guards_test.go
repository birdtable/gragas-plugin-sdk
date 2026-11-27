package declarative_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// many is head, then n items made by item(i) joined by commas, then tail.
func many(head string, n int, item func(i int) string, tail string) []byte {
	var b strings.Builder
	b.WriteString(head)
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(item(i))
	}
	b.WriteString(tail)
	return []byte(b.String())
}

// TestTheCapOnWhatIsReadHoldsInEveryReader: exactly MaxFindings items read,
// one more is refused by name, in each of the four places that count
// (boundary: MaxFindings).
func TestTheCapOnWhatIsReadHoldsInEveryReader(t *testing.T) {
	t.Parallel()
	sarif := func(n int) []byte {
		return many(`{"runs":[{"results":[`, n, func(int) string { return `{"ruleId":"a"}` }, `]}]}`)
	}
	jsonList := func(n int) []byte {
		return many(`{"issues":[`, n, func(int) string { return `{"rule":"a"}` }, `]}`)
	}
	lines := func(n int) []byte {
		return []byte(strings.Repeat("a.ts:1:1: error T1: m\n", n))
	}
	diagnostics := map[string]struct {
		o   model.AdapterOutput
		out func(n int) []byte
	}{
		"sarif": {sarifOut(), sarif},
		"json":  {jsonOut(), jsonList},
		"lines": {linesOut(), lines},
	}
	for name, c := range diagnostics {
		t.Run(name+" at the cap", func(t *testing.T) {
			t.Parallel()
			got, err := declarative.ReadDiagnostics(c.o, c.out(declarative.MaxFindings))
			if err != nil || len(got) != declarative.MaxFindings {
				t.Errorf("%s at the cap: %d findings, %v; want all, no error", name, len(got), err)
			}
		})
		t.Run(name+" over the cap", func(t *testing.T) {
			t.Parallel()
			got, err := declarative.ReadDiagnostics(c.o, c.out(declarative.MaxFindings+1))
			if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(err.Error(), "more than") {
				t.Errorf("%s over the cap: %d findings, %v; want a refusal by name", name, len(got), err)
			}
		})
	}

	object := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/rules", Severity: "/0"}}
	list := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/Enabled", Key: "/name"}}
	entries := map[string]struct {
		c   model.ConfigRead
		out func(n int) []byte
	}{
		"object": {object, func(n int) []byte {
			return many(`{"rules":{`, n, func(i int) string { return fmt.Sprintf(`"k%d":["e"]`, i) }, `}}`)
		}},
		"list": {list, func(n int) []byte {
			return many(`{"Enabled":[`, n, func(int) string { return `{"name":"a"}` }, `]}`)
		}},
	}
	for name, c := range entries {
		t.Run(name+" entries at the cap", func(t *testing.T) {
			t.Parallel()
			got, err := declarative.ReadEntries(c.c, c.out(declarative.MaxFindings))
			if err != nil || len(got) != declarative.MaxFindings {
				t.Errorf("%s entries at the cap: %d, %v; want all, no error", name, len(got), err)
			}
		})
		t.Run(name+" entries over the cap", func(t *testing.T) {
			t.Parallel()
			got, err := declarative.ReadEntries(c.c, c.out(declarative.MaxFindings+1))
			if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(err.Error(), "more than") {
				t.Errorf("%s entries over the cap: %d, %v; want a refusal by name", name, len(got), err)
			}
		})
	}
}

// TestALineOverTheLimitIsNoFinding: a line longer than 64 KiB is skipped, one
// of that length is read.
func TestALineOverTheLimitIsNoFinding(t *testing.T) {
	t.Parallel()
	const prefix = "a.ts:1:1: error T1: "
	at := prefix + strings.Repeat("x", 64<<10-len(prefix))
	over := at + "x"
	got, err := declarative.ReadDiagnostics(linesOut(), []byte(at+"\n"+over+"\n"))
	if err != nil || len(got) != 1 || len(at) != 64<<10 {
		t.Errorf("a line of the limit and one over read as %d findings, %v; want the first alone", len(got), err)
	}
}

// TestPointersFollowRFC6901: an index has no leading zero and is not negative,
// and ~0 and ~1 decode in that order.
func TestPointersFollowRFC6901(t *testing.T) {
	t.Parallel()
	at := func(p string) model.AdapterOutput {
		return model.AdapterOutput{Format: model.OutputJSON, Each: "", Fields: &model.Fields{Key: p}}
	}
	for name, c := range map[string]struct {
		out, key string
		want     string
	}{
		"index":           {`[{"k":["a","b"]}]`, "/k/1", "b"},
		"leading zero":    {`[{"k":["a","b"]}]`, "/k/01", ""},
		"negative":        {`[{"k":["a","b"]}]`, "/k/-1", ""},
		"past the end":    {`[{"k":["a","b"]}]`, "/k/2", ""},
		"tilde one":       {`[{"~1":"t","/":"s","~":"u"}]`, "/~01", "t"},
		"slash":           {`[{"~1":"t","/":"s"}]`, "/~1", "s"},
		"tilde zero":      {`[{"~":"u"}]`, "/~0", "u"},
		"tilde zero one":  {`[{"~/":"v"}]`, "/~0~1", "v"},
		"not a container": {`[{"k":"text"}]`, "/k/0", ""},
	} {
		got, err := declarative.ReadDiagnostics(at(c.key), []byte(c.out))
		if c.want == "" {
			if !errors.Is(err, declarative.ErrPrinted) || got != nil {
				t.Errorf("%s: %q reads as %+v, %v; want a refusal, no key", name, c.key, got, err)
			}
			continue
		}
		if err != nil || len(got) != 1 || got[0].Key != c.want {
			t.Errorf("%s: %q reads as %+v, %v; want key %q", name, c.key, got, err, c.want)
		}
	}
}

// TestAPlaceIsAWholePositiveNumberInRange: zero, negative, fractional and
// out-of-range places read as none, as a number and as text, in JSON and in
// lines.
func TestAPlaceIsAWholePositiveNumberInRange(t *testing.T) {
	t.Parallel()
	o := model.AdapterOutput{Format: model.OutputJSON, Each: "", Fields: &model.Fields{Key: "/k", Line: "/l", Column: "/col"}}
	for name, c := range map[string]struct {
		doc  string
		line int
		col  int
	}{
		"plain":             {`[{"k":"a","l":7,"col":"8"}]`, 7, 8},
		"largest":           {`[{"k":"a","l":2147483647,"col":"2147483647"}]`, 2147483647, 2147483647},
		"zero":              {`[{"k":"a","l":0,"col":"0"}]`, 0, 0},
		"negative":          {`[{"k":"a","l":-4,"col":"-4"}]`, 0, 0},
		"one past a number": {`[{"k":"a","l":2147483648,"col":"2147483648"}]`, 0, 0},
		"far past":          {`[{"k":"a","l":9223372036854775807,"col":"99999999999999999999"}]`, 0, 0},
		"fraction":          {`[{"k":"a","l":1.5,"col":"1.5"}]`, 0, 0},
	} {
		got, err := declarative.ReadDiagnostics(o, []byte(c.doc))
		if err != nil || len(got) != 1 || got[0].Line != c.line || got[0].Column != c.col {
			t.Errorf("%s: reads as %+v, %v; want line %d column %d", name, got, err, c.line, c.col)
		}
	}

	pattern := model.AdapterOutput{Format: model.OutputLines, OnlyForm: true, Pattern: `^(?P<key>\w+) (?P<line>-?\d+) (?P<column>\d+)$`}
	for name, c := range map[string]struct {
		line       string
		ln, column int
	}{
		"plain":    {"k 7 8", 7, 8},
		"largest":  {"k 2147483647 2147483647", 2147483647, 2147483647},
		"one past": {"k 2147483648 2147483648", 0, 0},
		"negative": {"k -3 5", 0, 5},
		"zero":     {"k 0 0", 0, 0},
	} {
		got, err := declarative.ReadDiagnostics(pattern, []byte(c.line))
		if err != nil || len(got) != 1 || got[0].Line != c.ln || got[0].Column != c.column {
			t.Errorf("lines %s: reads as %+v, %v; want line %d column %d", name, got, err, c.ln, c.column)
		}
	}
}

// TestALinesDeclarationWithNoPatternIsRefused: the empty pattern compiles and
// matches every line, so it is refused before any line is read.
func TestALinesDeclarationWithNoPatternIsRefused(t *testing.T) {
	t.Parallel()
	o := model.AdapterOutput{Format: model.OutputLines, OnlyForm: true}
	got, err := declarative.ReadDiagnostics(o, []byte("anything\n"))
	if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(err.Error(), "does not compile") {
		t.Errorf("no pattern reads as %+v, %v; want a refusal by name", got, err)
	}
}

// TestSARIFResultsThatAreNullAreNone: a run whose results is null has no
// finding, and one whose results is not a list is refused.
func TestSARIFResultsThatAreNullAreNone(t *testing.T) {
	t.Parallel()
	got, err := declarative.ReadDiagnostics(sarifOut(), []byte(`{"runs":[{"results":null},{"results":[{"ruleId":"a"}]}]}`))
	if err != nil || !slices.Equal(got, []model.Diagnostic{{Key: "a"}}) {
		t.Errorf("a null results reads as %+v, %v; want the second run's finding alone", got, err)
	}
	for _, results := range []string{`3`, `"x"`, `true`} {
		got, err := declarative.ReadDiagnostics(sarifOut(), []byte(`{"runs":[{"results":`+results+`}]}`))
		if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(err.Error(), "results is not a list") {
			t.Errorf("results %s reads as %+v, %v; want a refusal by name", results, got, err)
		}
	}
}

// TestAnObjectOfBareMembersGivesEachMemberItsWholeValue: with no pointer but
// each, an object's entries are valued by their members; a list entry, and an
// object with any pointer, are not.
func TestAnObjectOfBareMembersGivesEachMemberItsWholeValue(t *testing.T) {
	t.Parallel()
	bare := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/compilerOptions"}}
	got, err := declarative.ReadEntries(bare, []byte(`{"compilerOptions":{"strict":true,"target":"es2022","lib":["es2022","dom"],"x":null}}`))
	want := []model.ToolEntry{{Key: "strict", Value: "true"}, {Key: "target", Value: "es2022"}, {Key: "lib", Value: `["es2022", "dom"]`}, {Key: "x"}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("the bare object reads as %+v, %v; want %+v", got, err, want)
	}
	withSeverity := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/o", Severity: "/s"}}
	got, err = declarative.ReadEntries(withSeverity, []byte(`{"o":{"a":{"s":"warn","v":1}}}`))
	if err != nil || !slices.Equal(got, []model.ToolEntry{{Key: "a", Severity: "warn"}}) {
		t.Errorf("an object with a pointer reads as %+v, %v; want no whole value", got, err)
	}
	list := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/l", Key: "/n"}}
	got, err = declarative.ReadEntries(list, []byte(`{"l":[{"n":"a","v":1}]}`))
	if err != nil || !slices.Equal(got, []model.ToolEntry{{Key: "a"}}) {
		t.Errorf("a list reads as %+v, %v; want no whole value", got, err)
	}
}
