package declarative

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// plain is the plugin of testdata/bridge/plain.yaml: an adapter of the tool lint-a that covers the targets source and
// config, with no settings input.
func plain(t *testing.T) *Plugin {
	t.Helper()
	data, err := os.ReadFile("testdata/bridge/plain.yaml")
	if err != nil {
		t.Fatal(err)
	}
	p, err := New(data)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestAnEntryIsABindingOfTheAdaptersTool: each wire entry becomes the candidate the moved writer takes: the tool is
// the adapter's, the key, target and severity are the entry's, the rule is the first the entry names, trial is
// trial, a whole number is an int64 and a fraction a float64, an object keeps its keys, and an absent value and
// absent options are nil.
// Risk: the step between the request and the moved writer changes what is written.
func TestAnEntryIsABindingOfTheAdaptersTool(t *testing.T) {
	t.Parallel()
	cs, notes, err := plain(t).candidates([]wire.PresetEntry{
		{Key: "a", Target: "source", Severity: "error", Rules: []string{"go.a.1", "go.a.2"}},
		{Key: "b", Target: "config", Severity: "warning", Trial: true, Value: json.RawMessage(`300`), Options: json.RawMessage(`{"n":2,"f":0.5,"l":["x",null],"o":{"k":true}}`), Rules: []string{"go.b.1"}},
		{Key: "c", Target: "source", Value: json.RawMessage(`null`), Rules: []string{"go.c.1"}},
		{Key: "d", Target: "source", Value: json.RawMessage(`"s"`), Options: json.RawMessage(`null`), Rules: []string{"go.d.1"}},
	})
	if err != nil || len(notes) != 0 {
		t.Fatalf("err %v, notes %q", err, notes)
	}
	want := []candidate{
		{rule: "go.a.1", b: model.Binding{Tool: "lint-a", Target: "source", Key: "a", Severity: "error"}},
		{rule: "go.b.1", trial: true, b: model.Binding{Tool: "lint-a", Target: "config", Key: "b", Severity: "warning", Value: int64(300),
			Options: map[string]any{"n": int64(2), "f": 0.5, "l": []any{"x", nil}, "o": map[string]any{"k": true}}}},
		{rule: "go.c.1", b: model.Binding{Tool: "lint-a", Target: "source", Key: "c"}},
		{rule: "go.d.1", b: model.Binding{Tool: "lint-a", Target: "source", Key: "d", Value: "s"}},
	}
	if !reflect.DeepEqual(cs, want) {
		t.Errorf("candidates =\n%#v\nwant\n%#v", cs, want)
	}
}

// TestAnEntryOfATargetTheToolDoesNotCoverIsNotShippedAndSaysSo: the words are gragas's collect's, with the entry's first
// rule, and the entry is no candidate.
func TestAnEntryOfATargetTheToolDoesNotCoverIsNotShippedAndSaysSo(t *testing.T) {
	t.Parallel()
	cs, notes, err := plain(t).candidates([]wire.PresetEntry{{Key: "k", Target: "prose", Rules: []string{"go.d.1"}}})
	if err != nil || len(cs) != 0 || len(notes) != 1 || notes[0] != "go.d.1: lint-a prose k is not shipped: the tool does not cover that target" {
		t.Errorf("candidates %v, notes %q, err %v", cs, notes, err)
	}
}

// TestOptionsThatAreNotAnObjectAndAValueThatIsNotJSONAreRefusedAtTheirPointer: refused, with the pointer of the field
// in the request, since the plugin writes options as an object only.
func TestOptionsThatAreNotAnObjectAndAValueThatIsNotJSONAreRefusedAtTheirPointer(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		e    wire.PresetEntry
		path string
	}{
		"options a list":   {wire.PresetEntry{Key: "k", Target: "source", Options: json.RawMessage(`[1]`), Rules: []string{"r"}}, "/entries/1/options"},
		"options a number": {wire.PresetEntry{Key: "k", Target: "source", Options: json.RawMessage(`3`), Rules: []string{"r"}}, "/entries/1/options"},
		"options not JSON": {wire.PresetEntry{Key: "k", Target: "source", Options: json.RawMessage(`{`), Rules: []string{"r"}}, "/entries/1/options"},
		"value not JSON":   {wire.PresetEntry{Key: "k", Target: "source", Value: json.RawMessage(`{"a":`), Rules: []string{"r"}}, "/entries/1/value"},
	} {
		_, _, err := plain(t).candidates([]wire.PresetEntry{{Key: "ok", Target: "source", Rules: []string{"r"}}, c.e})
		we, ok := err.(*wire.Error)
		if !ok || we.Code != wire.CodeRefused || we.Path != c.path {
			t.Errorf("%s: %v, want refused at %s", name, err, c.path)
		}
	}
	var _ plugin.Plugin = (*Plugin)(nil)
}
