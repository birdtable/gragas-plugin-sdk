package declarative_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// at is where the test documents stand.
const at = "adapters/lint-a/1.0.0.yaml"

func strs(rs []declarative.Refusal) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.String()
	}
	return out
}

// TestTheCleanAdapterReadsToItsTypedValue is the clean control: every field
// of the document arrives in the adapter, the range parsed and the origin
// named.
func TestTheCleanAdapterReadsToItsTypedValue(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	a, refused := declarative.Read(s, model.LayerRulebook, at, read(t, "testdata/clean.yaml"))
	if len(refused) > 0 {
		t.Fatalf("refused: %v", strs(refused))
	}
	loadsCode := true
	want := model.Adapter{
		Format: 3, Tool: "lint-a", Version: "1.0.0", ToolVersions: ">=2.1.0 <3.0.0",
		Targets: []model.TargetType{model.TargetSource, model.TargetConfig}, Extends: true,
		Config: []model.ConfigRead{{Target: model.TargetSource, Argv: []string{"print-config", "{fixtures}"}, Format: model.ConfigJSON,
			Entries: model.EntryMap{Each: "/rules", Options: "/1", Severity: "/0", As: "true",
				Also: []model.EntrySource{{Each: "/off", Key: "/name", As: "false"}}}}},
		ConfigLoadsCode: &loadsCode,
		Run: model.Invocation{Argv: []string{"run", "--config", "{config}", "--out", "{output}", "{fixtures}"},
			Output: model.AdapterOutput{Format: model.OutputJSON, Each: "/issues", Fields: &model.Fields{Key: "/rule", Path: "/file", Line: "/line"}}},
		VersionArgv: []string{"--version"},
		Severities: model.SeverityScale{Form: model.SeverityLevel, Warning: "warn",
			Levels: []model.SeverityLevelName{{Name: "error", Is: model.SeverityError}, {Name: "warn", Is: model.SeverityWarning}}},
		DocsURL:     &model.DocsOption{Option: "docs-base"},
		ResolveFrom: "node_modules",
		Preset: model.Preset{Path: "lint-a.yml", Format: model.PresetYAML, Comment: model.CommentHash,
			Lines: []model.FixedLine{{Key: "version", Value: "2"}, {Key: "ruleDirs", Items: []string{"rules"}}},
			Placement: model.Placement{Shape: model.ShapeList, Each: "/rules", Key: "/name", SeverityTable: "/severity",
				SeverityItem: model.SeverityItem{Shape: model.ShapeList, Key: "/linters", KeyForm: model.KeyList, Level: "/level"}},
			Settings: &model.PresetSettings{File: "settings.yml", Begin: "# lint:begin", End: "# lint:end"},
			Wiring: &model.Wiring{Import: &model.ImportAt{At: "/extends", Form: model.ImportList},
				Install: []string{"lint-a-install", "@lint-a/cli@1.0.0"}}},
		Paths: []model.ToolPath{{Ecosystem: model.EcosystemNode, Path: "node_modules/.bin/lint-a"}},
		Range: model.VersionRange{Lower: model.VersionBound{Version: "2.1.0", Included: true},
			Upper: model.VersionBound{Version: "3.0.0"}},
		Origin: model.AdapterOrigin{Layer: model.LayerRulebook, Path: at},
	}
	if !reflect.DeepEqual(a, want) {
		t.Errorf("read:\n%#v\nwant:\n%#v", a, want)
	}
}

// TestTheRefusedDocumentsAreRefusedWithTheirLineAsTheGoldenSays: an
// expression, a conditional, a reference (a key, an alias, a tag) and an
// unknown key, and the other refusals, each with its line and its check.
// testdata/refused.golden is written by a person who reads it and never by
// this test.
func TestTheRefusedDocumentsAreRefusedWithTheirLineAsTheGoldenSays(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	entries, err := os.ReadDir("testdata/refused")
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for _, e := range entries {
		a, refused := declarative.Read(s, model.LayerProject, at, read(t, path.Join("testdata/refused", e.Name())))
		if len(refused) == 0 {
			t.Errorf("%s was read: %+v", e.Name(), a)
		}
		fmt.Fprintf(&got, "%s\n", e.Name())
		for _, r := range refused {
			fmt.Fprintf(&got, "  %s\n", r)
		}
	}
	if want := string(read(t, "testdata/refused.golden")); got.String() != want {
		t.Errorf("the refusals differ from testdata/refused.golden; got:\n%s", got.String())
	}
}

// TestEachPlantedAdapterTurnsItsNamedCheckRed: each plant under
// testdata/plants is refused by the one check its file is named for, once,
// and the clean control beside them is read.
func TestEachPlantedAdapterTurnsItsNamedCheckRed(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	plants := map[string]declarative.Check{
		"expression.yaml":                    declarative.CheckExpression,
		"placeholder.yaml":                   declarative.CheckPlaceholder,
		"version-range.yaml":                 declarative.CheckRange,
		"preset-comment.yaml":                declarative.CheckShape,
		"preset-path.yaml":                   declarative.CheckShape,
		"preset-lines.yaml":                  declarative.CheckShape,
		"preset-severity-item.yaml":          declarative.CheckShape,
		"paths-both.yaml":                    declarative.CheckShape,
		"config-loads-code-not-boolean.yaml": declarative.CheckShape,
		"config-loads-code-zero.yaml":        declarative.CheckShape,
		"paths-neither.yaml":                 declarative.CheckShape,
		"paths-bin-key-with-a-path.yaml":     declarative.CheckShape,
		"paths-package-outside-node.yaml":    declarative.CheckShape,
	}
	for name, check := range plants {
		t.Run(name, func(t *testing.T) {
			_, refused := declarative.Read(s, model.LayerRulebook, at, read(t, "testdata/plants/"+name))
			if len(refused) != 1 || refused[0].Check != check || refused[0].Line == 0 {
				t.Errorf("refusals = %v, want one of the check %s with its line", strs(refused), check)
			}
		})
	}
	if _, refused := declarative.Read(s, model.LayerRulebook, at, read(t, "testdata/plants/clean.yaml")); len(refused) > 0 {
		t.Errorf("the clean control was refused: %v", strs(refused))
	}
	files, err := os.ReadDir("testdata/plants")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if _, ok := plants[f.Name()]; !ok && f.Name() != "clean.yaml" {
			t.Errorf("testdata/plants/%s names no check", f.Name())
		}
	}
}

// examples is the adapter's hand-written examples beside the other format
// schemas', held to the schema by the Schema adapter's own tests.
const examples = "testdata/format-v3/"

// TestTheSchemasValidExamplesReadAndItsInvalidOnesAreRefusedByTheReaderAlone:
// the Go reader, with a Schema that finds nothing, reads every document the
// schema holds valid and refuses every planted break the schema refuses, so
// neither half of the check leans on the other.
func TestTheSchemasValidExamplesReadAndItsInvalidOnesAreRefusedByTheReaderAlone(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	var docs []any
	if err := json.Unmarshal(read(t, examples+"adapter.valid.json"), &docs); err != nil {
		t.Fatal(err)
	}
	for i, d := range docs {
		if _, refused := declarative.Read(s, model.LayerEmbedded, placeOf(d), encode(t, d)); len(refused) > 0 {
			t.Errorf("valid document %d refused: %v", i, strs(refused))
		}
	}
	var breaks []struct {
		Name  string
		Patch []patchOp
	}
	if err := json.Unmarshal(read(t, examples+"adapter.invalid.json"), &breaks); err != nil {
		t.Fatal(err)
	}
	for _, b := range breaks {
		t.Run(b.Name, func(t *testing.T) {
			var fresh []any
			if err := json.Unmarshal(read(t, examples+"adapter.valid.json"), &fresh); err != nil {
				t.Fatal(err)
			}
			patched := apply(t, fresh, b.Patch)
			hit := false
			for i, d := range patched.([]any) {
				if _, refused := declarative.Read(s, model.LayerEmbedded, placeOf(docs[i]), encode(t, d)); len(refused) > 0 {
					hit = true
				}
			}
			if !hit {
				t.Errorf("the reader read every document of the break %q", b.Name)
			}
		})
	}
}

func placeOf(d any) string {
	m := d.(map[string]any)
	return fmt.Sprintf("adapters/%s/%s.yaml", m["tool"], m["version"])
}

func encode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// patchOp is one step of an RFC 6902 patch: add, remove or replace.
type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// apply applies ops to doc, a decoded JSON value, in place where it can.
func apply(t *testing.T, doc any, ops []patchOp) any {
	t.Helper()
	for _, op := range ops {
		steps := strings.Split(strings.TrimPrefix(op.Path, "/"), "/")
		for i, s := range steps {
			steps[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
		}
		parent := doc
		for _, s := range steps[:len(steps)-1] {
			switch p := parent.(type) {
			case map[string]any:
				parent = p[s]
			case []any:
				n, err := strconv.Atoi(s)
				if err != nil {
					t.Fatal(err)
				}
				parent = p[n]
			}
		}
		last := steps[len(steps)-1]
		switch p := parent.(type) {
		case map[string]any:
			if op.Op == "remove" {
				delete(p, last)
			} else {
				p[last] = op.Value
			}
		case []any:
			n, err := strconv.Atoi(last)
			if err != nil || op.Op != "replace" {
				t.Fatalf("the patch %s %s is not one this test applies", op.Op, op.Path)
			}
			p[n] = op.Value
		}
	}
	return doc
}

// TestARulebooksOverrideOfAnEmbeddedAdapterWinsAndTheLayerIsNamed: the more
// specific layer's adapter of the same tool and version replaces the less
// specific one's, and says it was set there; another version of the tool
// stays from the layer that has it.
func TestARulebooksOverrideOfAnEmbeddedAdapterWinsAndTheLayerIsNamed(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := read(t, "testdata/clean.yaml")
	// An override changes only the paths.
	override := []byte(strings.Replace(string(clean), "path: node_modules/.bin/lint-a", "path: tools/bin/lint-a", 1))
	moved := []byte(strings.Replace(string(clean), "path: node_modules/.bin/lint-a", "path: vendor/bin/lint-a", 1))
	older := []byte(strings.Replace(string(clean), "version: 1.0.0", "version: 0.9.0", 1))
	table, refused := declarative.Load(s, stand,
		declarative.Layer{Layer: model.LayerEmbedded, Files: []model.File{{Path: at, Data: clean}, {Path: "adapters/lint-a/0.9.0.yaml", Data: older}}},
		declarative.Layer{Layer: model.LayerRulebook, Files: []model.File{{Path: at, Data: override}}},
	)
	if len(refused) > 0 {
		t.Fatalf("refused: %v", strs(refused))
	}
	a, ok := table.Get("lint-a", "1.0.0")
	if !ok || a.Paths[0].Path != "tools/bin/lint-a" || a.Origin.Layer != model.LayerRulebook || a.Origin.Path != at {
		t.Errorf("lint-a 1.0.0 = %+v, %v; want the rulebook's, at its path, named as the rulebook's", a.Origin, a.Paths)
	}
	old, ok := table.Get("lint-a", "0.9.0")
	if !ok || old.Origin.Layer != model.LayerEmbedded {
		t.Errorf("lint-a 0.9.0 = %+v, %v; want the embedded one", old.Origin, ok)
	}
	var order []string
	for _, a := range table.All() {
		order = append(order, a.Tool+"@"+a.Version+"/"+string(a.Origin.Layer))
	}
	if want := []string{"lint-a@0.9.0/embedded", "lint-a@1.0.0/rulebook"}; !slices.Equal(order, want) {
		t.Errorf("All = %v, want %v", order, want)
	}
	project, refused := declarative.Load(s, stand,
		declarative.Layer{Layer: model.LayerEmbedded, Files: []model.File{{Path: at, Data: clean}}},
		declarative.Layer{Layer: model.LayerRulebook, Files: []model.File{{Path: at, Data: override}}},
		declarative.Layer{Layer: model.LayerProject, Files: []model.File{{Path: at, Data: moved}}},
	)
	if a, _ := project.Get("lint-a", "1.0.0"); len(refused) > 0 || a.Origin.Layer != model.LayerProject || a.Paths[0].Path != "vendor/bin/lint-a" {
		t.Errorf("with a project's: %+v, %v; want the project's", a.Origin, strs(refused))
	}
}

// TestOneRefusedDocumentRefusesTheWholeTable: a layer that meant to override
// an adapter and wrote it wrong never falls back to the layer below unseen.
func TestOneRefusedDocumentRefusesTheWholeTable(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	table, refused := declarative.Load(s, stand,
		declarative.Layer{Layer: model.LayerEmbedded, Files: []model.File{{Path: at, Data: read(t, "testdata/clean.yaml")}}},
		declarative.Layer{Layer: model.LayerProject, Files: []model.File{{Path: at, Data: read(t, "testdata/plants/expression.yaml")}}},
	)
	if len(refused) != 1 || refused[0].Check != declarative.CheckExpression || len(table.All()) != 0 {
		t.Errorf("table = %d adapters, refusals %v; want none and the project's one refusal", len(table.All()), strs(refused))
	}
	if _, ok := table.Get("lint-a", "1.0.0"); ok {
		t.Error("the embedded adapter stood in for the refused override")
	}
}

// TestLayersOutOfOrderTwiceOrUnknownAndAFileTwiceAreRefused.
func TestLayersOutOfOrderTwiceOrUnknownAndAFileTwiceAreRefused(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := model.File{Path: at, Data: read(t, "testdata/clean.yaml")}
	cases := map[string]struct {
		layers []declarative.Layer
		says   string
	}{
		"out of order": {[]declarative.Layer{{Layer: model.LayerProject}, {Layer: model.LayerRulebook}}, "comes after a more specific one"},
		"twice":        {[]declarative.Layer{{Layer: model.LayerRulebook}, {Layer: model.LayerRulebook}}, "comes after a more specific one"},
		"unknown":      {[]declarative.Layer{{Layer: "remote"}}, "no such layer"},
		"a file twice": {[]declarative.Layer{{Layer: model.LayerEmbedded, Files: []model.File{clean, clean}}}, "handed twice"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, refused := declarative.Load(s, stand, c.layers...)
			if len(refused) != 1 || refused[0].Check != declarative.CheckLayer || !strings.Contains(refused[0].Msg, c.says) {
				t.Errorf("refusals = %v, want one of the check layer saying %q", strs(refused), c.says)
			}
		})
	}
	if _, refused := declarative.Load(s, stand, declarative.Layer{Layer: model.LayerEmbedded, Files: []model.File{clean}}, declarative.Layer{Layer: model.LayerProject, Files: []model.File{clean}}); len(refused) > 0 {
		t.Errorf("the embedded and project layers, the rulebook's left out: %v", strs(refused))
	}
}

// TestTheCapsRefuseBeforeAnythingIsRead: a document over its byte cap, a
// layer over its count, a list, a text and an argument over theirs.
func TestTheCapsRefuseBeforeAnythingIsRead(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := string(read(t, "testdata/clean.yaml"))
	big := clean + "# " + strings.Repeat("x", declarative.MaxDocument) + "\n"
	if _, refused := declarative.Read(s, model.LayerProject, at, []byte(big)); len(refused) != 1 || refused[0].Check != declarative.CheckSize {
		t.Errorf("a document over the cap: %v", strs(refused))
	}
	if _, refused := declarative.Read(s, model.LayerProject, at, []byte(clean[:min(len(clean), declarative.MaxDocument)])); len(refused) > 0 {
		t.Errorf("the clean control: %v", strs(refused))
	}
	many := make([]model.File, declarative.MaxDocuments+1)
	for i := range many {
		many[i] = model.File{Path: fmt.Sprintf("adapters/lint-a/1.0.%d.yaml", i)}
	}
	if _, refused := declarative.Load(s, stand, declarative.Layer{Layer: model.LayerRulebook, Files: many}); len(refused) != 1 || refused[0].Check != declarative.CheckSize {
		t.Errorf("a layer over the cap: %v", strs(refused))
	}
	long := map[string]string{
		"an argv over its count": strings.Replace(clean, `--out, "{output}"`, strings.Repeat("-x, ", 64)+`"{output}"`, 1),
		"a long argument":        strings.Replace(clean, "--out", strings.Repeat("o", 257), 1),
		"a long text":            strings.Replace(clean, "option: docs-base", "option: "+strings.Repeat("d", 257), 1),
		"a long list":            strings.Replace(clean, "targets: [source, config]", "targets: ["+strings.Repeat("source, ", 32)+"config]", 1),
	}
	for name, doc := range long {
		_, refused := declarative.Read(s, model.LayerProject, at, []byte(doc))
		if !slices.ContainsFunc(refused, func(r declarative.Refusal) bool { return r.Check == declarative.CheckSize }) {
			t.Errorf("%s: %v, want a size refusal", name, strs(refused))
		}
	}
}

// TestNoMoreThanTwentyRefusalsAreGathered: a document wrong everywhere says
// the first twenty, so a refusal cannot be made to fill a screen or memory.
func TestNoMoreThanTwentyRefusalsAreGathered(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	var b strings.Builder
	b.WriteString("format: 3\n")
	for i := range 40 {
		fmt.Fprintf(&b, "unknown%d: x\n", i)
	}
	if _, refused := declarative.Read(s, model.LayerProject, at, []byte(b.String())); len(refused) != 20 {
		t.Errorf("%d refusals, want 20", len(refused))
	}
}

// TestARefusalReadsAsPathLineCheckAndSentence.
func TestARefusalReadsAsPathLineCheckAndSentence(t *testing.T) {
	t.Parallel()
	r := declarative.Refusal{Path: at, Line: 3, Check: declarative.CheckKey, Msg: "x"}
	if got := r.String(); got != at+":3: key: x" {
		t.Errorf("String = %q", got)
	}
	r.Line = 0
	if got := r.String(); got != at+": key: x" {
		t.Errorf("String with no line = %q", got)
	}
}

// TestAFileNotAtItsPlaceIsRefused: the path is adapters/<tool>/<version>.yaml
// and nothing else.
func TestAFileNotAtItsPlaceIsRefused(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := read(t, "testdata/clean.yaml")
	for _, p := range []string{"adapters/lint-a/1.0.yaml", "adapters/lint-a/1.0.0.yml", "adapters/Lint/1.0.0.yaml", "adapters/x/lint-a/1.0.0.yaml",
		"lint-a/1.0.0.yaml", "adapters/lint-a/01.0.0.yaml", "adapters/../lint-a/1.0.0.yaml", "adapters/lint-a/README.md"} {
		if _, refused := declarative.Read(s, model.LayerProject, p, clean); len(refused) != 1 || refused[0].Check != declarative.CheckPlace {
			t.Errorf("%s: %v, want the place refused", p, strs(refused))
		}
	}
}

// checks is the closed set of checks, for the fuzz target.
func checks() []declarative.Check {
	return []declarative.Check{declarative.CheckSize, declarative.CheckPlace, declarative.CheckYAML, declarative.CheckFormat, declarative.CheckKey, declarative.CheckConditional,
		declarative.CheckReference, declarative.CheckExpression, declarative.CheckPlaceholder, declarative.CheckShape, declarative.CheckRange, declarative.CheckOutput,
		declarative.CheckSchema, declarative.CheckLayer, declarative.CheckOverride}
}

// FuzzParseAdapter: whatever the bytes, Read answers an adapter or at most
// twenty refusals of the closed set of checks, never both and never neither,
// and an adapter it answers holds only literals and placeholders in its argv,
// a range that parses back to its Range, and the tool and version its path
// names.
func FuzzParseAdapter(f *testing.F) {
	for _, p := range []string{"testdata/clean.yaml", "testdata/plants/expression.yaml", "testdata/plants/placeholder.yaml",
		"testdata/plants/version-range.yaml"} {
		b, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		a, refused := declarative.Read(stubSchema{}, model.LayerProject, at, data)
		if len(refused) > 20 {
			t.Fatalf("%d refusals", len(refused))
		}
		for _, r := range refused {
			if !slices.Contains(checks(), r.Check) || r.Path != at {
				t.Fatalf("refusal %v names no check of the set, or another path", r)
			}
		}
		if len(refused) > 0 {
			if a.Tool != "" || a.Origin.Layer != "" {
				t.Fatalf("refused and still answered %+v", a)
			}
			return
		}
		if a.Tool != "lint-a" || a.Version != "1.0.0" || a.Origin.Path != at {
			t.Fatalf("read %s %s from %s", a.Tool, a.Version, at)
		}
		vr, err := declarative.ParseRange(a.ToolVersions)
		if err != nil || vr != a.Range {
			t.Fatalf("the range %q parses to %+v, %v; the adapter holds %+v", a.ToolVersions, vr, err, a.Range)
		}
		argvs := [][]string{a.Run.Argv}
		for _, c := range a.Config {
			argvs = append(argvs, c.Argv)
		}
		for _, argv := range argvs {
			for _, s := range argv {
				if !slices.Contains(model.Placeholders(), model.Placeholder(s)) && strings.ContainsAny(s, "{}$`\\") {
					t.Fatalf("the argument %q is neither a literal nor a placeholder", s)
				}
			}
		}
	})
}

// TestFuzzSeedsAreTheCheckedInOnes keeps the checked-in seeds in step with
// the plants: a seed per plant and the clean control, and one per field of the
// preset.
func TestFuzzSeedsAreTheCheckedInOnes(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("testdata/fuzz/FuzzParseAdapter")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if want := []string{"alias-bomb", "clean", "expression", "placeholder", "preset-comment", "preset-dir", "preset-each", "preset-format",
		"preset-import", "preset-install", "preset-key-value-options", "preset-lines", "preset-path", "preset-severity", "preset-severity-item", "preset-severity-table", "preset-shape", "version-range"}; !slices.Equal(names, want) {
		t.Errorf("seeds = %v, want %v", names, want)
	}
}

// TestTheDocumentCapIsHeldAtItsBoundary (tests.generated.7): a document of
// exactly MaxDocument bytes is read, and one byte more is refused for its
// size, so the cap is neither a byte short nor a byte long. Both are the clean
// document with a comment as long as it takes, so each is small beside what a
// reader holds. boundary: MaxDocument.
func TestTheDocumentCapIsHeldAtItsBoundary(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := read(t, "testdata/clean.yaml")
	padded := func(size int) []byte {
		pad := size - len(clean) - len("# \n")
		if pad < 0 {
			t.Fatalf("the clean document is %d bytes, over %d", len(clean), size)
		}
		return append(slices.Clone(clean), "# "+strings.Repeat("x", pad)+"\n"...)
	}
	exact, over := padded(declarative.MaxDocument), padded(declarative.MaxDocument+1)
	if len(exact) != declarative.MaxDocument || len(over) != declarative.MaxDocument+1 {
		t.Fatalf("the two documents are %d and %d bytes, want %d and %d", len(exact), len(over), declarative.MaxDocument, declarative.MaxDocument+1)
	}
	if _, refused := declarative.Read(s, model.LayerProject, at, exact); len(refused) > 0 {
		t.Errorf("a document of the cap: %v", strs(refused))
	}
	if _, refused := declarative.Read(s, model.LayerProject, at, over); len(refused) != 1 || refused[0].Check != declarative.CheckSize {
		t.Errorf("a document one byte over the cap: %v", strs(refused))
	}
}

// TestAnOutputNamesTheCodesThatMeanRanAndTheKeysThatMeanFailed: the two lists
// are read as given, and a code outside 0 to 255, a repeat, an
// empty list and a key that is not one token are each refused.
func TestAnOutputNamesTheCodesThatMeanRanAndTheKeysThatMeanFailed(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := string(read(t, "testdata/clean.yaml"))
	with := func(extra string) string {
		return strings.Replace(clean, "  output:\n    format: json\n", "  output:\n    format: json\n"+extra, 1)
	}
	a, refused := declarative.Read(s, model.LayerProject, at, []byte(with("    ran_codes: [0, 1]\n    error_keys: [parse-error, TS5083]\n")))
	if len(refused) > 0 {
		t.Fatalf("refused: %v", strs(refused))
	}
	if !slices.Equal(a.Run.Output.RanCodes, []int{0, 1}) || !slices.Equal(a.Run.Output.ErrorKeys, []string{"parse-error", "TS5083"}) {
		t.Errorf("read %v and %v, want [0 1] and [parse-error TS5083]", a.Run.Output.RanCodes, a.Run.Output.ErrorKeys)
	}
	if a, refused := declarative.Read(s, model.LayerProject, at, []byte(with("    error_keys: [\"x@typescript-eslint/no-x\", \"ts:5083\"]\n"))); len(refused) > 0 || len(a.Run.Output.ErrorKeys) != 2 {
		t.Errorf("a key with an at sign and one with a colon: refused %v, read %v; the reader takes the schema's whole key form", strs(refused), a.Run.Output.ErrorKeys)
	}
	for name, extra := range map[string]string{
		"a code over 255":      "    ran_codes: [256]\n",
		"a negative code":      "    ran_codes: [-1]\n",
		"a code that is text":  "    ran_codes: [zero]\n",
		"a repeated code":      "    ran_codes: [1, 1]\n",
		"an empty code list":   "    ran_codes: []\n",
		"a key with a space":   "    error_keys: [\"parse error\"]\n",
		"a repeated error key": "    error_keys: [a, a]\n",
		"an empty key list":    "    error_keys: []\n",
	} {
		if _, refused := declarative.Read(s, model.LayerProject, at, []byte(with(extra))); len(refused) == 0 {
			t.Errorf("%s: read clean, want a refusal", name)
		}
	}
}

// TestTheTwoOutputListsAreHeldAtTheirCaps: a list of exactly the cap reads and
// one over it is refused as a size. boundary: maxRanCodes.
// boundary: maxErrorKeys.
func TestTheTwoOutputListsAreHeldAtTheirCaps(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := string(read(t, "testdata/clean.yaml"))
	with := func(extra string) []byte {
		return []byte(strings.Replace(clean, "  output:\n    format: json\n", "  output:\n    format: json\n"+extra, 1))
	}
	codes := func(n int) string {
		l := make([]string, n)
		for i := range l {
			l[i] = strconv.Itoa(i)
		}
		return "    ran_codes: [" + strings.Join(l, ", ") + "]\n"
	}
	keys := func(n int) string {
		l := make([]string, n)
		for i := range l {
			l[i] = fmt.Sprintf("k%d", i)
		}
		return "    error_keys: [" + strings.Join(l, ", ") + "]\n"
	}
	for name, c := range map[string]struct{ at, over string }{
		"ran_codes":  {codes(16), codes(17)},
		"error_keys": {keys(32), keys(33)},
	} {
		if _, refused := declarative.Read(s, model.LayerProject, at, with(c.at)); len(refused) > 0 {
			t.Errorf("%s at the cap: %v", name, strs(refused))
		}
		if _, refused := declarative.Read(s, model.LayerProject, at, with(c.over)); !slices.ContainsFunc(refused, func(r declarative.Refusal) bool { return r.Check == declarative.CheckSize }) {
			t.Errorf("%s over the cap: %v, want a size refusal", name, strs(refused))
		}
	}
}
