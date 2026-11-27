package declarative_test

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// fetched is the layers an adapter may be fetched in: a rulebook's and a
// project's.
func fetched() []model.AdapterLayer {
	return []model.AdapterLayer{model.LayerRulebook, model.LayerProject}
}

// overriding loads the clean adapter as the embedded one and doc at path in
// layer above it.
func overriding(t *testing.T, layer model.AdapterLayer, path, doc string) (declarative.Table, []declarative.Refusal) {
	t.Helper()
	s, _ := schema(t)
	return declarative.Load(s, stand,
		declarative.Layer{Layer: model.LayerEmbedded, Files: []model.File{{Path: at, Data: read(t, "testdata/clean.yaml")}}},
		declarative.Layer{Layer: layer, Files: []model.File{{Path: path, Data: []byte(doc)}}},
	)
}

// refusedFor fails the test unless the table is empty and the refusals are
// each of the check c and say every one of says between them.
func refusedFor(t *testing.T, table declarative.Table, refused []declarative.Refusal, c declarative.Check, says ...string) {
	t.Helper()
	if len(table.All()) != 0 || len(refused) == 0 {
		t.Fatalf("table = %d adapters, refusals %v; want none and refused", len(table.All()), strs(refused))
	}
	all := strings.Join(strs(refused), "\n")
	for _, r := range refused {
		if r.Check != c {
			t.Errorf("refusal %v, want the check %s", r, c)
		}
	}
	for _, s := range says {
		if !strings.Contains(all, s) {
			t.Errorf("refusals %v do not say %q", strs(refused), s)
		}
	}
}

// TestAFetchedAdapterThatRunsCodeIsRefused (the plant): a rulebook's
// or a project's adapter that hands -c code to a program below the project
// is refused, and the table with it, whether the code is in the version
// argv, a config read or the run, and whether the program is an interpreter
// its paths name or the tool itself. The code here is a flag and a word, so
// the literal form passes it and the override check alone refuses it.
// Risk: a fetched rulebook runs code of its choosing in the version or config
// read, which no sandbox holds.
func TestAFetchedAdapterThatRunsCodeIsRefused(t *testing.T) {
	t.Parallel()
	clean := string(read(t, "testdata/clean.yaml"))
	python := strings.Replace(clean, "  - ecosystem: node\n    path: node_modules/.bin/lint-a", "  - ecosystem: python\n    path: .venv/bin/python", 1)
	cases := map[string]struct {
		doc  string
		says []string
	}{
		"-c code in the version argv, run by an interpreter": {strings.Replace(python, "version_argv: [--version]", "version_argv: [-c, x=1]", 1),
			[]string{"changes the embedded adapter's version_argv", "paths[0].path names the program python"}},
		"-c code in a config read": {strings.Replace(clean, `argv: [print-config, "{fixtures}"]`, `argv: [-c, x=1, "{fixtures}"]`, 1),
			[]string{"changes the embedded adapter's config,"}},
		"-c code in the run": {strings.Replace(clean, `argv: [run, --config,`, `argv: [-c, x=1, --config,`, 1),
			[]string{"changes the embedded adapter's run,"}},
		"an interpreter at its path": {python, []string{"paths[0].path names the program python, and a"}},
	}
	for name, c := range cases {
		for _, layer := range fetched() {
			t.Run(name+"/"+string(layer), func(t *testing.T) {
				if c.doc == clean {
					t.Fatal("the plant did not change the clean adapter")
				}
				table, refused := overriding(t, layer, at, c.doc)
				refusedFor(t, table, refused, declarative.CheckOverride, append(c.says, "a "+string(layer)+"'s")...)
			})
		}
	}
	t.Run("code with a space, refused by its form first", func(t *testing.T) {
		table, refused := overriding(t, model.LayerRulebook, at, strings.Replace(python, "version_argv: [--version]", `version_argv: [-c, "import os; os.system('id')"]`, 1))
		refusedFor(t, table, refused, declarative.CheckShape, "version_argv[1]", "a literal is one flag or word")
	})
	t.Run("the control: other paths to the same program", func(t *testing.T) {
		for _, layer := range fetched() {
			table, refused := overriding(t, layer, at, strings.Replace(clean, "path: node_modules/.bin/lint-a", "path: tools/lint-a", 1))
			if a, ok := table.Get("lint-a", "1.0.0"); len(refused) > 0 || !ok || a.Origin.Layer != layer || a.Paths[0].Path != "tools/lint-a" {
				t.Errorf("%s: %+v, %v; want the override taken", layer, a.Origin, strs(refused))
			}
		}
	})
}

// TestAFetchedAdapterOverridesOnlyOneTheEngineEmbeds: a rulebook's or
// a project's adapter of a tool, or a version, the engine does not embed is
// refused, and so is one with no embedded layer under it.
// Risk: a fetched adapter that adds a tool or a version brings argv no
// engine release ever shipped.
func TestAFetchedAdapterOverridesOnlyOneTheEngineEmbeds(t *testing.T) {
	t.Parallel()
	clean := string(read(t, "testdata/clean.yaml"))
	for _, layer := range fetched() {
		t.Run(string(layer), func(t *testing.T) {
			table, refused := overriding(t, layer, "adapters/lint-a/1.1.0.yaml", strings.Replace(clean, "version: 1.0.0", "version: 1.1.0", 1))
			refusedFor(t, table, refused, declarative.CheckOverride, "the engine embeds no adapter of lint-a at 1.1.0")
			table, refused = overriding(t, layer, "adapters/lint-b/1.0.0.yaml", strings.Replace(clean, "tool: lint-a", "tool: lint-b", 1))
			refusedFor(t, table, refused, declarative.CheckOverride, "the engine embeds no adapter of lint-b at 1.0.0")
			s, _ := schema(t)
			table, refused = declarative.Load(s, stand, declarative.Layer{Layer: layer, Files: []model.File{{Path: at, Data: []byte(clean)}}})
			refusedFor(t, table, refused, declarative.CheckOverride, "the engine embeds no adapter of lint-a at 1.0.0")
		})
	}
}

// step is one step from an adapter to a value inside it: a field, an element
// of a list, or the value a pointer points at.
type step struct {
	field, elem int
	deref       bool
}

// leaf is a value of the adapter that holds no other: where it is, its name
// for a person, and the top-level field it is under.
type leaf struct {
	at   []step
	name string
	top  reflect.StructField
}

// leaves is every leaf under v, which stands at at under the top-level field
// top. A struct, a pointer and a list of structs are walked into; a nil
// pointer or an empty list of structs fails the test, since the clean
// adapter must hold each one for its fields to be set otherwise; a nil
// pointer or an empty list in an item of a list is passed over, since items may each hold
// one of two shapes (a config read's argv or files), and the
// test names the leaves it wants of each shape.
func leaves(t *testing.T, v reflect.Value, at []step, name string, top reflect.StructField) []leaf {
	t.Helper()
	switch {
	case v.Kind() == reflect.Struct:
		var out []leaf
		for i := range v.NumField() {
			out = append(out, leaves(t, v.Field(i), append(slices.Clip(at), step{field: i, elem: -1}), name+"."+v.Type().Field(i).Name, top)...)
		}
		return out
	case v.Kind() == reflect.Pointer:
		if v.IsNil() && strings.Contains(name, "[") {
			return nil
		}
		if v.IsNil() {
			t.Fatalf("the clean adapter leaves %s nil, so its fields cannot be set otherwise", name)
		}
		return leaves(t, v.Elem(), append(slices.Clip(at), step{field: -1, elem: -1, deref: true}), name, top)
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Struct:
		if v.Len() == 0 && strings.Contains(name, "[") {
			return nil
		}
		if v.Len() == 0 {
			t.Fatalf("the clean adapter holds no %s, so its fields cannot be set otherwise", name)
		}
		var out []leaf
		for i := range v.Len() {
			out = append(out, leaves(t, v.Index(i), append(slices.Clip(at), step{field: -1, elem: i}), fmt.Sprintf("%s[%d]", name, i), top)...)
		}
		return out
	}
	return []leaf{{at: at, name: name, top: top}}
}

// follow is the value at the steps from v.
func follow(v reflect.Value, at []step) reflect.Value {
	for _, s := range at {
		switch {
		case s.deref:
			v = v.Elem()
		case s.elem >= 0:
			v = v.Index(s.elem)
		default:
			v = v.Field(s.field)
		}
	}
	return v
}

// setOtherwise gives the leaf v another value of its kind.
func setOtherwise(t *testing.T, name string, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.String:
		v.SetString(v.String() + "x")
	case reflect.Bool:
		v.SetBool(!v.Bool())
	case reflect.Int:
		v.SetInt(v.Int() + 1)
	case reflect.Slice:
		v.Set(reflect.Append(v, reflect.Zero(v.Type().Elem())))
	default:
		t.Fatalf("%s is a %s, which the test cannot set otherwise: teach it", name, v.Kind())
	}
}

// TestEveryFieldButPathsIsHeldToTheEmbeddedOne: each leaf of the
// adapter, however deep (a field of the run's output, of a config read's
// entries, of a severity level), set otherwise in a fresh copy, is named as a
// change of its top-level field, and a leaf of the paths is not. So a field
// added to model.Adapter, or to any type inside it, is held to the embedded
// adapter or this test fails.
// Risk: a field added later that decides what runs, or how a cell passes,
// passes an override unseen.
func TestEveryFieldButPathsIsHeldToTheEmbeddedOne(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := append(read(t, "testdata/clean.yaml"), "values:\n  keys:\n    - {key: level, order: [loose, strict]}\n"...)
	// a second config read, of entry files, so a leaf of Files is walked too
	clean = []byte(strings.Replace(string(clean), "\nrun:\n", "\n  - target: config\n    files: {config: lint.yaml, dirs: /ruleDirs, ext: [.yaml]}\n    format: yaml\n    entries: {each: \"\", key: /id, severity: /severity}\nrun:\n", 1))
	fresh := func() model.Adapter {
		a, refused := declarative.Read(s, model.LayerEmbedded, at, clean)
		if len(refused) > 0 {
			t.Fatal(strs(refused))
		}
		return a
	}
	a := fresh()
	if d := declarative.Differs(a, fresh()); d != nil {
		t.Errorf("the adapter differs from itself in %v", d)
	}
	ty, av := reflect.TypeFor[model.Adapter](), reflect.ValueOf(a)
	var all []leaf
	for i := range ty.NumField() {
		if f := ty.Field(i); f.Name != "Origin" {
			all = append(all, leaves(t, av.Field(i), []step{{field: i, elem: -1}}, f.Name, f)...)
		}
	}
	for _, l := range all {
		b := fresh()
		setOtherwise(t, l.name, follow(reflect.ValueOf(&b).Elem(), l.at))
		want := []string{strings.Split(l.top.Tag.Get("json"), ",")[0]}
		switch l.top.Name {
		case "Paths":
			want = nil
		case "Range":
			want = []string{"tool_versions"}
		}
		if got := declarative.Differs(b, a); !slices.Equal(got, want) {
			t.Errorf("%s set otherwise: differs names %v, want %v", l.name, got, want)
		}
	}
	for _, want := range []string{"Config[0].Argv", "Config[0].Entries.Also[0].Each", "Config[1].Files.Config", "Config[1].Files.Dirs", "Config[1].Files.Ext"} {
		if !slices.ContainsFunc(all, func(l leaf) bool { return l.name == want }) {
			t.Errorf("the walk did not reach %s", want)
		}
	}
	if len(all) < 30 {
		t.Errorf("only %d leaves were walked; the walk stopped short", len(all))
	}
}

// TestALiteralIsOneFlagOrWord (the second guard): a literal argument
// with a space, a quote, a bracket or a shell character is refused in every
// layer, the engine's own among them, and in each argv; a flag, a flag with
// its value and a path are read.
// Risk: an argv literal carries code a program it reaches runs.
func TestALiteralIsOneFlagOrWord(t *testing.T) {
	t.Parallel()
	s, _ := schema(t)
	clean := string(read(t, "testdata/clean.yaml"))
	for _, lit := range []string{"import os", "a;b", "'x'", `\"x\"`, "(x)", "a|b", "a>b", "a<b", "*", "a?", "~/x", "a&b", "#x", "!x", "a[0]", "a b"} {
		for _, k := range []string{"version_argv: [--version", "argv: [print-config", "argv: [run"} {
			doc := strings.Replace(clean, k, k+`, "`+lit+`"`, 1)
			_, refused := declarative.Read(s, model.LayerEmbedded, at, []byte(doc))
			if len(refused) != 1 || refused[0].Check != declarative.CheckShape || !strings.Contains(refused[0].Msg, "a literal is one flag or word") {
				t.Errorf("%q after %s: %v, want refused by its form", lit, k, strs(refused))
			}
		}
	}
	for _, lit := range []string{"-c", "--show-stats=false", os.DevNull, "stdout", "a,b", "host:1", "--output.sarif.path", "x_y"} {
		doc := strings.Replace(clean, "argv: [run", `argv: [run, "`+lit+`"`, 1)
		if _, refused := declarative.Read(s, model.LayerEmbedded, at, []byte(doc)); len(refused) > 0 {
			t.Errorf("%q: %v, want read", lit, strs(refused))
		}
	}
}

// stand is a neutral stand-in for the system's file name of a program: the
// core knows no system's suffix.
func stand(id string) string { return id + ".x" }

// TestAPathMayEndInTheSystemsFileNameOfAProgramTheAdapterNames: a path
// ending in the tool, as it is or as fileName spells it, is held; one ending
// in another program, spelled as fileName spells it, is refused.
// Risk: an override refused on a system that spells a program with a suffix,
// or one that names an interpreter by that spelling.
func TestAPathMayEndInTheSystemsFileNameOfAProgramTheAdapterNames(t *testing.T) {
	t.Parallel()
	clean := string(read(t, "testdata/clean.yaml"))
	for _, c := range []struct {
		tail string
		ok   bool
	}{
		{"lint-a", true},
		{"lint-a.x", true},
		{"python.x", false},
	} {
		t.Run(c.tail, func(t *testing.T) {
			table, refused := overriding(t, model.LayerProject, at, strings.Replace(clean, "path: node_modules/.bin/lint-a", "path: tools/"+c.tail, 1))
			if c.ok {
				if a, ok := table.Get("lint-a", "1.0.0"); len(refused) > 0 || !ok || a.Paths[0].Path != "tools/"+c.tail {
					t.Errorf("%+v, %v; want the override taken", a.Paths, strs(refused))
				}
				return
			}
			refusedFor(t, table, refused, declarative.CheckOverride, "paths[0].path names the program "+c.tail)
		})
	}
}
