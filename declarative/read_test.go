package declarative_test

import (
	"errors"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// The three outputs a reader is declared for. They are small documents of the
// reader's own tests; what real tools print is held by the recordings, and the
// tests of the adapters that read them (cmd/gragas).
func sarifOut() model.AdapterOutput { return model.AdapterOutput{Format: model.OutputSARIF} }

func jsonOut() model.AdapterOutput {
	return model.AdapterOutput{Format: model.OutputJSON, Each: "/issues", Fields: &model.Fields{
		Key: "/rule", Message: "/text", Path: "/file", Line: "/line", Column: "/col", Severity: "/level", URL: "/url"}}
}

func linesOut() model.AdapterOutput {
	return model.AdapterOutput{Format: model.OutputLines, OnlyForm: true,
		Pattern: `^(?P<path>[^:\n]+):(?P<line>\d+):(?P<column>\d+): (?P<severity>\w+) (?P<key>[A-Z]+\d+): (?P<message>.*)$`}
}

const sarifPlanted = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},"results":[` +
	`{"ruleId":"errcheck","level":"error","message":{"text":"not checked"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"fixtures/bad.go"},"region":{"startLine":5,"startColumn":3}}}]},` +
	`{"ruleId":"govet","message":{"text":"no place"}}]}]}`

func TestSARIFReadsTheRuleIdAsTheKeyAndTheFirstLocationAsThePlace(t *testing.T) {
	t.Parallel()
	got, err := declarative.ReadDiagnostics(sarifOut(), []byte(sarifPlanted))
	want := []model.Diagnostic{
		{Key: "errcheck", Message: "not checked", Path: "fixtures/bad.go", Line: 5, Column: 3, Severity: "error"},
		{Key: "govet", Message: "no place"},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("the planted log reads as %+v, %v; want %+v", got, err, want)
	}
	clean := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},"results":[]}]}`
	if got, err := declarative.ReadDiagnostics(sarifOut(), []byte(clean)); err != nil || len(got) != 0 {
		t.Errorf("the clean control reads as %+v, %v; want none", got, err)
	}
	noResults := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}}}]}`
	if got, err := declarative.ReadDiagnostics(sarifOut(), []byte(noResults)); err != nil || len(got) != 0 {
		t.Errorf("a run with no results key reads as %+v, %v; want none", got, err)
	}
}

func TestJSONReadsTheFieldsAtTheirPointers(t *testing.T) {
	t.Parallel()
	doc := `{"issues":[{"rule":"no-bad","file":"bad.txt","line":2,"col":"7","text":"bad","level":"warn","url":"https://x/y"},{"rule":"no-todo"}]}`
	got, err := declarative.ReadDiagnostics(jsonOut(), []byte(doc))
	want := []model.Diagnostic{
		{Key: "no-bad", Message: "bad", Path: "bad.txt", Line: 2, Column: 7, Severity: "warn", URL: "https://x/y"},
		{Key: "no-todo"},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("reads as %+v, %v; want %+v", got, err, want)
	}
	if got, err := declarative.ReadDiagnostics(jsonOut(), []byte(`{"issues":[]}`)); err != nil || len(got) != 0 {
		t.Errorf("the clean control reads as %+v, %v; want none", got, err)
	}
	// A pointer steps through a list by index and through ~0 and ~1.
	o := model.AdapterOutput{Format: model.OutputJSON, Each: "", Fields: &model.Fields{Key: "/ids/0", Path: "/a~1b", Line: "/t~0"}}
	got, err = declarative.ReadDiagnostics(o, []byte(`[{"ids":["k"],"a/b":"p","t~":4}]`))
	if err != nil || !slices.Equal(got, []model.Diagnostic{{Key: "k", Path: "p", Line: 4}}) {
		t.Errorf("the escaped pointers read as %+v, %v", got, err)
	}
}

func TestLinesReadEachLineThePatternMatchesAndNoOther(t *testing.T) {
	t.Parallel()
	out := "src/a.ts:3:9: error TS2322: Type 'x' is not assignable.\r\nnoise that is no finding\nsrc/b.ts:10:1: warning TS6133: unused\n"
	got, err := declarative.ReadDiagnostics(linesOut(), []byte(out))
	want := []model.Diagnostic{
		{Key: "TS2322", Message: "Type 'x' is not assignable.", Path: "src/a.ts", Line: 3, Column: 9, Severity: "error"},
		{Key: "TS6133", Message: "unused", Path: "src/b.ts", Line: 10, Column: 1, Severity: "warning"},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("reads as %+v, %v; want %+v", got, err, want)
	}
	if got, err := declarative.ReadDiagnostics(linesOut(), nil); err != nil || len(got) != 0 {
		t.Errorf("the clean control, nothing printed, reads as %+v, %v; want none", got, err)
	}
}

// TestOutputThatDoesNotFitItsDeclarationIsRefusedByName: each is refused with
// ErrPrinted and a sentence that says where, and none gives a partial read
// (gates.plants.1, the refusal case).
func TestOutputThatDoesNotFitItsDeclarationIsRefusedByName(t *testing.T) {
	t.Parallel()
	noKey := model.AdapterOutput{Format: model.OutputLines, OnlyForm: true, Pattern: `^(?P<path>\w+)$`}
	cases := map[string]struct {
		o    model.AdapterOutput
		out  string
		says string
	}{
		"sarif empty":        {sarifOut(), "", "no document"},
		"sarif not json":     {sarifOut(), "{", "yaml"},
		"sarif no runs":      {sarifOut(), `{"version":"2.1.0"}`, "/runs is not one"},
		"sarif no rule id":   {sarifOut(), `{"runs":[{"results":[{"ruleId":"a"},{"message":{"text":"x"}}]}]}`, "run 1 result 2 has no ruleId"},
		"sarif results":      {sarifOut(), `{"runs":[{"results":{}}]}`, "results is not a list"},
		"json no each":       {jsonOut(), `{"other":[]}`, `"/issues" is not in the printed document`},
		"json each not list": {jsonOut(), `{"issues":{}}`, "is not a list of findings"},
		"json no key":        {jsonOut(), `{"issues":[{"rule":"a"},{"file":"f"}]}`, `finding 2 has no key at "/rule"`},
		"json key not text":  {jsonOut(), `{"issues":[{"rule":["a"]}]}`, "finding 1 has no key"},
		"json no fields":     {model.AdapterOutput{Format: model.OutputJSON, Each: "/issues"}, `{"issues":[]}`, "names no fields"},
		"lines no key":       {noKey, "abc\n", "has no key"},
		"lines bad pattern":  {model.AdapterOutput{Format: model.OutputLines, OnlyForm: true, Pattern: "("}, "x", "does not compile"},
		"unknown format":     {model.AdapterOutput{Format: "xml"}, "<a/>", `"xml"`},
		"over the cap":       {sarifOut(), strings.Repeat(" ", declarative.MaxPrinted+1), "over the"},
	}
	for name, c := range cases {
		got, err := declarative.ReadDiagnostics(c.o, []byte(c.out))
		if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(c.says)) {
			t.Errorf("%s: %+v, %v; want a refusal by ErrPrinted saying %q and no findings", name, got, err, c.says)
		}
	}
}

func TestEntriesAreReadFromAnObjectByMemberNameAndFromAListByKey(t *testing.T) {
	t.Parallel()
	object := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/rules", Value: "/0", Options: "/1", Severity: "/0"}}
	got, err := declarative.ReadEntries(object, []byte(`{"rules":{"no-bad":["error",{"max":3}],"no-todo":["warn",{}]}}`))
	want := []model.ToolEntry{
		{Key: "no-bad", Value: "error", Options: `{"max": 3}`, Severity: "error"},
		{Key: "no-todo", Value: "warn", Options: "{}", Severity: "warn"},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("the object reads as %+v, %v; want %+v", got, err, want)
	}
	list := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/Enabled", Key: "/name"}}
	got, err = declarative.ReadEntries(list, []byte(`{"Enabled":[{"name":"errcheck","since":"v1"},{"name":"govet"}],"Disabled":[{"name":"x"}]}`))
	if err != nil || !slices.Equal(got, []model.ToolEntry{{Key: "errcheck"}, {Key: "govet"}}) {
		t.Errorf("the list reads as %+v, %v", got, err)
	}
	yaml := model.ConfigRead{Format: model.ConfigYAML, Entries: model.EntryMap{Each: "/rules", Severity: "/level"}}
	got, err = declarative.ReadEntries(yaml, []byte("rules:\n  a:\n    level: quiet\n  b:\n    level: error\n"))
	if err != nil || !slices.Equal(got, []model.ToolEntry{{Key: "a", Severity: "quiet"}, {Key: "b", Severity: "error"}}) {
		t.Errorf("the YAML reads as %+v, %v", got, err)
	}
	if got, err := declarative.ReadEntries(list, []byte(`{"Enabled":[]}`)); err != nil || len(got) != 0 {
		t.Errorf("a config with no entry reads as %+v, %v; want none", got, err)
	}
}

func TestAConfigThatDoesNotFitItsEntryMapIsRefusedByName(t *testing.T) {
	t.Parallel()
	list := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/Enabled", Key: "/name"}}
	for name, c := range map[string]struct{ out, says string }{
		"empty":        {"", "no document"},
		"no entries":   {`{}`, `"/Enabled" is not in the printed config`},
		"not a list":   {`{"Enabled":3}`, "neither an object nor a list"},
		"entry no key": {`{"Enabled":[{"name":"a"},{"id":"b"}]}`, `entry 2 has no key at "/name"`},
		"empty key":    {`{"Enabled":[{"name":""}]}`, "entry 1 has no key"},
		"blank":        {" \n\t", "no document"},
		// JSON is read by JSON's rules: YAML is no JSON.
		"yaml as json": {"Enabled: [{name: a}]\n", "a value cannot start"},
		"twice":        {`{"Enabled":[],"Enabled":[{"name":"a"}]}`, "duplicate"},
	} {
		got, err := declarative.ReadEntries(list, []byte(c.out))
		if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(err.Error(), c.says) {
			t.Errorf("%s: %+v, %v; want a refusal by ErrPrinted saying %q", name, got, err, c.says)
		}
	}
	yaml := model.ConfigRead{Format: model.ConfigYAML, Entries: list.Entries}
	got, err := declarative.ReadEntries(yaml, []byte("Enabled: &a [1]\nx: *a\n"))
	if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(err.Error(), "anchor") {
		t.Errorf("alias: %+v, %v; want a refusal by ErrPrinted saying %q", got, err, "anchor")
	}
}

// FuzzReadDiagnostics: whatever a tool prints, the reader neither panics nor
// gives a finding with no key, a negative place or more than its cap, and
// refuses with ErrPrinted alone (tests.generated.1).
func FuzzReadDiagnostics(f *testing.F) {
	for i, s := range []string{sarifPlanted, `{"issues":[{"rule":"a","line":1}]}`, "a.ts:1:2: error TS1: m\n"} {
		f.Add(byte(i), []byte(s))
	}
	outs := []model.AdapterOutput{sarifOut(), jsonOut(), linesOut()}
	f.Fuzz(func(t *testing.T, which byte, data []byte) {
		got, err := declarative.ReadDiagnostics(outs[int(which)%len(outs)], data)
		if err != nil {
			if !errors.Is(err, declarative.ErrPrinted) || got != nil {
				t.Fatalf("refused with %v and %d findings", err, len(got))
			}
			return
		}
		if len(got) > declarative.MaxFindings {
			t.Fatalf("%d findings", len(got))
		}
		for _, d := range got {
			if d.Key == "" || d.Line < 0 || d.Column < 0 {
				t.Fatalf("a finding %+v", d)
			}
		}
	})
}

// TestAnObjectMemberWithAnEmptyNameIsRefusedByName: an entry is named by its
// member, and one with no name is refused as a list entry with none is.
// Risk: an entry with no key reaches the compare, which cannot match it.
func TestAnObjectMemberWithAnEmptyNameIsRefusedByName(t *testing.T) {
	t.Parallel()
	object := model.ConfigRead{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/rules", Value: "/0", Options: "/1", Severity: "/0"}}
	yaml := model.ConfigRead{Format: model.ConfigYAML, Entries: object.Entries}
	for name, c := range map[string]struct {
		read model.ConfigRead
		out  string
	}{
		"first":  {object, `{"rules":{"":["error",{}]}}`},
		"second": {object, `{"rules":{"a":["error",{}],"":["warn",{}]}}`},
		"null":   {object, `{"rules":{"":null}}`},
		// A member with no value is no JSON, and YAML reads it as null.
		"bare": {yaml, `{"rules":{""}}`},
	} {
		got, err := declarative.ReadEntries(c.read, []byte(c.out))
		if !errors.Is(err, declarative.ErrPrinted) || got != nil || !strings.Contains(err.Error(), "has no key") {
			t.Errorf("%s: %+v, %v; want a refusal by ErrPrinted saying it has no key", name, got, err)
		}
	}
}

// FuzzReadEntries: the same for a printed config, as an object and as a list.
func FuzzReadEntries(f *testing.F) {
	f.Add(byte(0), []byte(`{"rules":{"a":["error",{}]}}`))
	f.Add(byte(1), []byte(`{"Enabled":[{"name":"errcheck"}]}`))
	f.Add(byte(0), []byte("rules:\n  a: [warn, {}]\n"))
	reads := []model.ConfigRead{
		{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/rules", Value: "/0", Options: "/1", Severity: "/0"}},
		{Format: model.ConfigJSON, Entries: model.EntryMap{Each: "/Enabled", Key: "/name"}},
	}
	f.Fuzz(func(t *testing.T, which byte, data []byte) {
		got, err := declarative.ReadEntries(reads[int(which)%len(reads)], data)
		if err != nil {
			if !errors.Is(err, declarative.ErrPrinted) || got != nil {
				t.Fatalf("refused with %v and %d entries", err, len(got))
			}
			return
		}
		for _, e := range got {
			if e.Key == "" {
				t.Fatalf("an entry with no key: %+v", e)
			}
		}
	})
}

// TestTheReadersFuzzSeedsAreTheCheckedInOnes: the planted output, the clean
// control and the refusal of each format stay checked in as seeds.
func TestTheReadersFuzzSeedsAreTheCheckedInOnes(t *testing.T) {
	t.Parallel()
	for dir, want := range map[string][]string{
		"FuzzReadDiagnostics": {"json-no-key", "json-planted", "lines-clean", "lines-planted", "sarif-clean", "sarif-empty", "sarif-no-rule-id", "sarif-planted"},
		"FuzzReadEntries":     {"anchor", "emptykey", "linters", "object", "yaml"},
	} {
		entries, err := os.ReadDir("testdata/fuzz/" + dir)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		if !slices.Equal(names, want) {
			t.Errorf("%s seeds = %v, want %v", dir, names, want)
		}
	}
}

// yamlApart are plain scalars YAML 1.1 and YAML 1.2 read as different values.
var yamlApart = []string{"off", "on", "yes", "No", "010", "1_000", "1:30", "2001-12-14"}

// TestAPlainScalarTheTwoYAMLsReadApartIsRefusedWhereTheToolsVersionIsNotNamed
// (the refusal ported from the engine's reader): a printed YAML
// config of the format "yaml" names no version, so a plain scalar the two
// versions read apart is refused with its line, whichever of them the tool
// read. Risk: the config is compared under one reading while the tool ran
// under the other.
func TestAPlainScalarTheTwoYAMLsReadApartIsRefusedWhereTheToolsVersionIsNotNamed(t *testing.T) {
	t.Parallel()
	c := model.ConfigRead{Format: model.ConfigYAML, Entries: model.EntryMap{Each: "/rules", Severity: "/level"}}
	for _, s := range yamlApart {
		_, err := declarative.ReadEntries(c, []byte("rules:\n  a:\n    level: "+s+"\n"))
		if err == nil || !strings.Contains(err.Error(), "line 3 holds the value") {
			t.Errorf("level %s: err = %v, want it refused at line 3", s, err)
		}
		if got, err := declarative.ReadEntries(c, []byte("rules:\n  a:\n    level: \""+s+"\"\n")); err != nil || len(got) != 1 || got[0].Severity != s {
			t.Errorf("level %q quoted: %+v, %v; want it read as text", s, got, err)
		}
	}
}

// TestAnUnquotedOffReadsAsOffWhereTheToolReadsYAML12: ast-grep
// writes a rule's severity as an unquoted off and reads YAML 1.2, where off is
// the word. A config of the format "yaml-1.2" is read by that version's core
// schema, so each scalar is the value the tool saw: off, yes and 010 are text,
// true is still a boolean and a quoted word is the word. A plain scalar the
// 1.2 schema reads as a number stays one. Risk: off read as false, so a
// switched-off rule is imported as a rule that is on, or the file is refused.
func TestAnUnquotedOffReadsAsOffWhereTheToolReadsYAML12(t *testing.T) {
	t.Parallel()
	c := model.ConfigRead{Format: model.ConfigYAML12, Entries: model.EntryMap{Each: "/rules", Severity: "/level"}}
	for _, s := range []string{"off", "on", "yes", "No", "1_000", "1:30", "2001-12-14"} {
		for _, form := range []string{s, `"` + s + `"`} {
			got, err := declarative.ReadEntries(c, []byte("rules:\n  a:\n    level: "+form+"\n"))
			if err != nil || len(got) != 1 || got[0].Severity != s {
				t.Errorf("level %s: %+v, %v; want a at %q", form, got, err, s)
			}
		}
	}
	if _, err := declarative.ReadEntries(c, []byte("rules:\n  a:\n    level: off\n---\nx: 1\n")); err == nil {
		t.Error("a second document was read; want it refused as under yaml")
	}
}
