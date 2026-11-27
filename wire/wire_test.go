package wire_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/contract"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// schemaEnum is the enum at the dotted path of the pinned wire schema's $defs, as strings.
func schemaEnum(t *testing.T, at ...string) []string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(contract.Wire(), &doc); err != nil {
		t.Fatalf("the pinned wire schema does not read: %v", err)
	}
	var node any = doc["$defs"]
	for _, k := range at {
		m, ok := node.(map[string]any)
		if !ok {
			t.Fatalf("no %s in the schema", strings.Join(at, "."))
		}
		node = m[k]
	}
	m, _ := node.(map[string]any)
	vals, ok := m["enum"].([]any)
	if !ok {
		t.Fatalf("%s holds no enum", strings.Join(at, "."))
	}
	var out []string
	for _, v := range vals {
		out = append(out, v.(string))
	}
	return out
}

func names[T ~string](vs []T) []string {
	var out []string
	for _, v := range vs {
		out = append(out, string(v))
	}
	return out
}

// TestEveryEnumHereIsTheSchemasOwn: each list of names the package gives equals the enum of the pinned wire schema,
// in its order. A schema copied at a newer commit, or a name mistyped here, is red.
// Risk: the SDK answers or accepts a word the contract does not have, or refuses one it has.
func TestEveryEnumHereIsTheSchemasOwn(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		got  []string
		want []string
	}{
		"target":   {names(wire.Targets()), schemaEnum(t, "target")},
		"severity": {names(wire.Severities()), schemaEnum(t, "severity")},
		"level":    {names(wire.Levels()), schemaEnum(t, "level")},
		"op":       {names(wire.Ops()), schemaEnum(t, "request", "properties", "op")},
		"for":      {names(wire.Fors()), schemaEnum(t, "plan.request", "properties", "for")},
		"aspect":   {names(wire.Aspects()), schemaEnum(t, "rank.request", "properties", "pairs", "items", "properties", "aspect")},
		"word":     {names(wire.Words()), schemaEnum(t, "rank.response", "properties", "words", "items")},
		"code":     {names(wire.Codes()), schemaEnum(t, "error", "properties", "code")},
		"kind":     {names([]wire.Kind{wire.KindRun, wire.KindRead}), schemaEnum(t, "result", "properties", "kind")},
	} {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s: the package has %v, the pinned schema %v", name, c.got, c.want)
		}
	}
}

func TestBytesAreTextWhenTheyAreUTF8AndBase64WhenNot(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		in   []byte
		want string
	}{
		{[]byte("hello\n"), `{"text":"hello\n"}`},
		{[]byte{}, `{"text":""}`},
		{[]byte{0xff, 0xfe, 'a'}, `{"base64":"//5h"}`},
	} {
		got, err := json.Marshal(wire.Bytes{Data: c.in})
		if err != nil || string(got) != c.want {
			t.Errorf("Marshal(%q) = %s, %v, want %s", c.in, got, err, c.want)
		}
		var back wire.Bytes
		if err := json.Unmarshal(got, &back); err != nil || !reflect.DeepEqual(append([]byte{}, back.Data...), append([]byte{}, c.in...)) {
			t.Errorf("round trip of %q gave %q, %v", c.in, back.Data, err)
		}
	}
	for _, bad := range []string{`{}`, `{"text":"a","base64":"YQ=="}`, `{"base64":"!!"}`, `{"text":1}`, `"a"`} {
		var b wire.Bytes
		if err := json.Unmarshal([]byte(bad), &b); err == nil {
			t.Errorf("%s was read as bytes: %q", bad, b.Data)
		}
	}
}

func TestAStepIsARunOrAReadByItsKind(t *testing.T) {
	t.Parallel()
	var steps []wire.Step
	in := `[{"kind":"run","program":"eslint","argv":["--format","json","{scratch}/a"],"collect":["out.json"],"level":"sandboxed"},{"kind":"read","glob":"**/*.json"}]`
	if err := json.Unmarshal([]byte(in), &steps); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Run == nil || steps[0].Run.Program != "eslint" || steps[0].Run.Level != wire.LevelSandboxed || steps[1].Read == nil || steps[1].Read.Glob != "**/*.json" {
		t.Fatalf("steps = %+v", steps)
	}
	out, err := json.Marshal(steps)
	if err != nil || string(out) != in {
		t.Errorf("Marshal = %s, %v, want %s", out, err, in)
	}
	for _, bad := range []string{`{"kind":"walk"}`, `{}`} {
		var s wire.Step
		if err := json.Unmarshal([]byte(bad), &s); err == nil {
			t.Errorf("%s was read as a step", bad)
		}
	}
	if _, err := json.Marshal(wire.Step{}); err == nil {
		t.Error("an empty step was written")
	}
	if _, err := json.Marshal(wire.Step{Run: &wire.Run{}, Read: &wire.Read{}}); err == nil {
		t.Error("a step that is both a run and a read was written")
	}
}

// TestAnAbsentValueIsNotANullValue: in a value, null is a value the tool may hold and differs from an absent one,
// so the two read and write apart.
func TestAnAbsentValueIsNotANullValue(t *testing.T) {
	t.Parallel()
	var absent, null wire.Entry
	if err := json.Unmarshal([]byte(`{"key":"k","on":true}`), &absent); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"key":"k","on":true,"value":null}`), &null); err != nil {
		t.Fatal(err)
	}
	if len(absent.Value) != 0 || string(null.Value) != "null" {
		t.Fatalf("absent value = %q, null value = %q", absent.Value, null.Value)
	}
	a, _ := json.Marshal(absent)
	n, _ := json.Marshal(null)
	if string(a) != `{"key":"k","on":true}` || string(n) != `{"key":"k","on":true,"value":null}` {
		t.Errorf("written as %s and %s", a, n)
	}
}
