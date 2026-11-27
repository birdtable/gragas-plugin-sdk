package declarative

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// override refuses an adapter of a rulebook or a project that is more than an
// embedded adapter with other paths. Such an adapter is fetched
// data, and its argv is run: the version and config reads run with no
// sandbox, so an argv it chose could hand a program below the project code to
// run. It therefore overrides only an adapter the engine embeds, at the same
// tool and version; every field but paths must be the embedded one's; and
// each path must end in a program the embedded adapter's paths end in, or in
// the tool's own name, so that a path override cannot name an interpreter
// for the embedded argv to drive. The paths are where the tool lives, which
// the project's own layout decides; nothing else of an adapter is.
//
// fileName is the system's file name of a program by its id (a suffix a system
// adds to one), so a path may end in that name too. The core
// holds no system's suffix: the host's tool port supplies the function.
func override(a model.Adapter, embedded map[[2]string]model.Adapter, fileName func(id string) string) []Refusal {
	at, layer := a.Origin.Path, a.Origin.Layer
	e, ok := embedded[[2]string{a.Tool, a.Version}]
	if !ok {
		return []Refusal{{Path: at, Check: CheckOverride, Msg: fmt.Sprintf(
			"the engine embeds no adapter of %s at %s, and a %s's adapter only changes the paths of one it embeds", a.Tool, a.Version, layer)}}
	}
	var out []Refusal
	if d := differs(a, e); len(d) > 0 {
		out = append(out, Refusal{Path: at, Check: CheckOverride, Msg: fmt.Sprintf(
			"it changes the embedded adapter's %s, and a %s's adapter changes only its paths (copy every other field of the embedded adapter, config_loads_code with them)", strings.Join(d, ", "), layer)})
	}
	names := programs(e, fileName)
	for i, p := range a.Paths {
		if name := path.Base(p.Path); !slices.Contains(names, name) {
			out = append(out, Refusal{Path: at, Check: CheckOverride, Msg: fmt.Sprintf(
				"paths[%d].path names the program %s, and a %s's path ends in one the embedded adapter names: %s", i, name, layer, strings.Join(names, ", "))})
		}
	}
	return out
}

// programs is the file names a path of an override may end in: the tool's
// name and the last step of each of the embedded adapter's paths, each as it
// is and as fileName gives it.
func programs(e model.Adapter, fileName func(id string) string) []string {
	var out []string
	add := func(name string) {
		for _, n := range []string{name, fileName(name)} {
			if !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	add(e.Tool)
	for _, p := range e.Paths {
		add(path.Base(p.Path))
	}
	return out
}

// differs is the field of a that is not e's, by its name in the document,
// each field but paths compared, and each field inside one. A test sets each
// leaf of the adapter otherwise, at any depth, so a field added later is
// compared here or the test fails.
func differs(a, e model.Adapter) []string {
	var out []string
	add := func(same bool, field string) {
		if !same {
			out = append(out, field)
		}
	}
	add(a.Format == e.Format, "format")
	add(a.Tool == e.Tool, "tool")
	add(a.Version == e.Version, "version")
	add(a.ToolVersions == e.ToolVersions && a.Range == e.Range, "tool_versions")
	add(slices.Equal(a.Targets, e.Targets), "targets")
	add(a.Extends == e.Extends, "extends")
	add(slices.EqualFunc(a.Config, e.Config, sameRead), "config")
	add(samePointer(a.ConfigLoadsCode, e.ConfigLoadsCode), "config_loads_code")
	add(sameInvocation(a.Run, e.Run), "run")
	add(slices.Equal(a.VersionArgv, e.VersionArgv), "version_argv")
	add(sameScale(a.Severities, e.Severities), "severities")
	add(sameValues(a.Values.Off, e.Values.Off) && sameValues(a.Values.Order, e.Values.Order) && slices.EqualFunc(a.Values.Keys, e.Values.Keys, sameKeyScale), "values")
	add(samePointer(a.DocsURL, e.DocsURL), "docs_url")
	add(a.ResolveFrom == e.ResolveFrom, "resolve_from")
	add(samePreset(a.Preset, e.Preset), "preset")
	return out
}

func samePreset(a, b model.Preset) bool {
	same := a.Path == b.Path && a.Format == b.Format && a.Comment == b.Comment && a.Placement == b.Placement &&
		slices.EqualFunc(a.Lines, b.Lines, func(x, y model.FixedLine) bool {
			return x.Key == y.Key && x.Value == y.Value && slices.Equal(x.Items, y.Items)
		})
	if (a.Settings == nil) != (b.Settings == nil) || (a.Settings != nil && *a.Settings != *b.Settings) {
		return false
	}
	if a.Wiring == nil || b.Wiring == nil {
		return same && a.Wiring == b.Wiring
	}
	return same && samePointer(a.Wiring.Import, b.Wiring.Import) && slices.Equal(a.Wiring.Install, b.Wiring.Install)
}

func sameRead(a, b model.ConfigRead) bool {
	return a.Target == b.Target && slices.Equal(a.Argv, b.Argv) && sameFiles(a.Files, b.Files) && a.Format == b.Format && sameEntries(a.Entries, b.Entries)
}

// sameFiles is whether two reads of entry files read the same files.
func sameFiles(a, b *model.FileRead) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Config == b.Config && a.Dirs == b.Dirs && slices.Equal(a.Ext, b.Ext)
}

// sameEntries is whether two entry maps read the same: every pointer, the
// value an entry takes where the print gives none, and the further lists.
func sameEntries(a, b model.EntryMap) bool {
	return a.Each == b.Each && a.Key == b.Key && a.Value == b.Value && a.Options == b.Options && a.OptionsTop == b.OptionsTop &&
		a.Severity == b.Severity && a.As == b.As && slices.Equal(a.Also, b.Also)
}

func sameInvocation(a, b model.Invocation) bool {
	o, p := a.Output, b.Output
	return slices.Equal(a.Argv, b.Argv) && o.Format == p.Format && o.Each == p.Each && samePointer(o.Fields, p.Fields) &&
		o.Pattern == p.Pattern && o.OnlyForm == p.OnlyForm &&
		slices.Equal(o.RanCodes, p.RanCodes) && slices.Equal(o.ErrorKeys, p.ErrorKeys)
}

func sameScale(a, b model.SeverityScale) bool {
	return a.Form == b.Form && slices.Equal(a.Levels, b.Levels) && a.Warning == b.Warning && a.Default == b.Default && slices.Equal(a.Off, b.Off)
}

// samePointer is whether a and b are both nil or point at equal values.
func samePointer[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// sameValues is whether two lists of values hold the same values in the same
// order, each compared as its stable text.
func sameKeyScale(a, b model.KeyScale) bool {
	return a.Key == b.Key && sameValues(a.Off, b.Off) && sameValues(a.Order, b.Order)
}

func sameValues(a, b []any) bool {
	return slices.EqualFunc(a, b, func(x, y any) bool { return stable(x) == stable(y) })
}
