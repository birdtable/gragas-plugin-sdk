package declarative

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
)

// installForm is a word of a tool's install line: letters, digits and the
// few marks a package name and its version are written with. No space, quote,
// bracket or shell character, and no placeholder.
var installForm = regexp.MustCompile(`^[A-Za-z0-9@._/:,=+-]+$`)

// lineKey is the key of a fixed line and lineValue a word of its value: names
// and literals, no space, quote, bracket or placeholder.
var (
	lineKey   = regexp.MustCompile("^[A-Za-z][A-Za-z0-9_.-]*$")
	lineValue = regexp.MustCompile("^[A-Za-z0-9@._/:+-]+$")
)

const (
	lineKeySays   = "a name of letters, digits and _ . -, starting with a letter"
	lineValueSays = "text of letters, digits and @ . _ / : + -"
)

const installSays = "one word of letters, digits and @ . _ / : , = + -"

// preset reads how a preset of the tool is written: its file,
// its format, the comment of the generated mark, where an entry goes and the
// wiring.
func (r *reader) preset(m *ordered.Map) model.Preset {
	p := r.sub(m, "", "preset", true)
	if p == nil {
		return model.Preset{}
	}
	const where = "preset"
	r.keys(p, where, "path", "format", "comment", "lines", "placement", "settings", "wiring")
	out := model.Preset{
		Path:    r.relative(p, where, "path", true),
		Format:  oneOf(r, p, where, "format", model.PresetFormats(), true),
		Comment: oneOf(r, p, where, "comment", model.CommentSyntaxes(), true),
	}
	if out.Format != "" && out.Comment != "" && !out.Format.Holds(out.Comment) {
		r.refuse(p.KeyLine("comment"), CheckShape, "preset.comment is %s, which a %s file cannot hold", out.Comment, out.Format)
	}
	out.Lines = r.lines(p, where)
	if pl := r.sub(p, where, "placement", true); pl != nil {
		out.Placement = r.placement(pl)
		r.optionsAtMeetsTheRest(pl, out)
	}
	if st := r.sub(p, where, "settings", false); st != nil {
		out.Settings = r.settings(st)
		if out.Settings.File != "" && out.Settings.File == out.Path {
			r.refuse(st.KeyLine("file"), CheckShape, "preset.settings.file is the preset's own path %s, and a file cannot be its own input", out.Path)
		}
		if out.Placement.OptionsAt != "" {
			r.refuse(st.Line, CheckShape, "preset.settings is set with placement.options_at: the options of a binding are written in the generated document, and a settings input holds only the one list between its markers, so they would be dropped")
		}
	}
	if w := r.sub(p, where, "wiring", false); w != nil {
		out.Wiring = r.wiring(w)
	}
	return out
}

// settings reads the tool's settings input: the file, below the preset
// folder, and the two marker lines, each one line that is not blank and holds
// no line break, the two different.
func (r *reader) settings(m *ordered.Map) *model.PresetSettings {
	const where = "preset.settings"
	r.keys(m, where, "file", "begin", "end")
	out := &model.PresetSettings{File: r.relative(m, where, "file", true)}
	for k, dst := range map[string]*string{"begin": &out.Begin, "end": &out.End} {
		s, ok := r.text(m, where, k, maxText, true)
		if !ok {
			continue
		}
		if strings.TrimSpace(s) == "" || strings.ContainsAny(s, "\r\n") || s != strings.TrimSpace(s) {
			r.refuse(m.KeyLine(k), CheckShape, "%s.%s is %q, and it must be one line, with no blank at either end and no line break", where, k, s)
			continue
		}
		*dst = s
	}
	if out.Begin != "" && out.Begin == out.End {
		r.refuse(m.KeyLine("end"), CheckShape, "%s.end is the line %s.begin is, and the two markers must differ", where, where)
	}
	return out
}

// optionsAt reads where a list preset's binding options go: a JSON pointer
// from the document's root in which one step is exactly {key}, the binding's
// key, and no other step holds a brace. It is a plain lookup, as every pointer.
func (r *reader) optionsAt(m *ordered.Map, where string) string {
	s, ok := r.text(m, where, "options_at", maxText, false)
	if !ok {
		return ""
	}
	steps := strings.Split(s, "/")
	n := 0
	for i, st := range steps {
		if st == "{key}" {
			steps[i] = "key"
			n++
		}
	}
	plain := strings.Join(steps, "/")
	switch {
	case n != 1:
		r.refuse(m.KeyLine("options_at"), CheckShape, "%s is %q, and it must hold the binding's key as one step written {key}, exactly once", at(where, "options_at"), s)
	case expressive(plain):
		r.refuse(m.KeyLine("options_at"), CheckExpression, "%s is %q, which holds an expression's characters; a pointer is a plain lookup", at(where, "options_at"), s)
	case !pointerRE.MatchString(plain):
		r.refuse(m.KeyLine("options_at"), CheckShape, "%s is %q, which is no JSON pointer: \"\" or steps each starting with /, ~ written ~0 and / written ~1", at(where, "options_at"), s)
	default:
		return s
	}
	return ""
}

// optionsAtMeetsTheRest refuses an options_at that is written where the preset writes
// something else: over a fixed line (its first step is a line's key) or on the
// severity table (its steps before {key} and the table's pointer are a prefix of each
// other), where one side would be lost without a word.
func (r *reader) optionsAtMeetsTheRest(m *ordered.Map, p model.Preset) {
	if p.Placement.OptionsAt == "" {
		return
	}
	steps := segments(p.Placement.OptionsAt)
	before := steps[:slices.Index(steps, "{key}")]
	for _, l := range p.Lines {
		if len(before) > 0 && before[0] == l.Key || len(before) == 0 {
			r.refuse(m.KeyLine("options_at"), CheckShape, "preset.placement.options_at is %q, which is written over the fixed line %q, and one of the two would be lost", p.Placement.OptionsAt, l.Key)
		}
	}
	if table := p.Placement.SeverityTable; table != "" {
		if ts := segments(table); hasPrefix(ts, before) || hasPrefix(before, ts) {
			r.refuse(m.KeyLine("options_at"), CheckShape, "preset.placement.options_at is %q, which is written where severity_table %q is, and one of the two would be lost", p.Placement.OptionsAt, table)
		}
	}
}

// hasPrefix is whether the steps of whole start with the steps of prefix.
func hasPrefix(whole, prefix []string) bool {
	return len(prefix) <= len(whole) && slices.Equal(whole[:len(prefix)], prefix)
}

// optionsAtPlace holds an options_at to where it can be written: a list only, not beside
// options, and not through the list's own pointer or a step that put reads as an index,
// where a binding's options would replace the list.
func (r *reader) optionsAtPlace(m *ordered.Map, where string, shape model.Shape, pl model.Placement, hasEach bool) {
	if pl.OptionsAt == "" {
		return
	}
	line := m.KeyLine("options_at")
	if shape != model.ShapeList {
		r.refuse(line, CheckShape, "%s.options_at is set, and the shape is %s, which places options on the entry: only a list has no member of its own to hold them", where, shape)
	}
	if pl.Options != "" {
		r.refuse(line, CheckShape, "%s holds options and options_at, and a tool places its options in one of them", where)
	}
	steps := segments(pl.OptionsAt)
	before := steps[:slices.Index(steps, "{key}")]
	if each := segments(pl.Each); shape == model.ShapeList && hasEach && (hasPrefix(each, before) || hasPrefix(before, each)) {
		r.refuse(line, CheckShape, "%s.options_at is %q, whose steps before {key} are a prefix of placement.each %q or run through it, so a binding whose key is a step of the list would replace the list", where, pl.OptionsAt, pl.Each)
	}
	if i := slices.IndexFunc(before, isIndex); i >= 0 {
		r.refuse(line, CheckShape, "%s.options_at is %q, whose step %q is digits, which the generator reads as a list index and not as a name, so the list would be lost", where, pl.OptionsAt, before[i])
	}
}

// optionsTopPlace holds an options_top to where it can be written: files only, whose entry
// is a file of its own, and not beside options or options_at, since a tool places its
// options in one of them.
func (r *reader) optionsTopPlace(m *ordered.Map, where string, shape model.Shape, pl model.Placement) {
	if !pl.OptionsTop {
		return
	}
	line := m.KeyLine("options_top")
	if shape != model.ShapeFiles && shape != "" {
		r.refuse(line, CheckShape, "%s.options_top is set, and the shape is %s, whose entries share one document: only an entry file has a top of its own to hold them", where, shape)
	}
	if pl.Options != "" || pl.OptionsAt != "" {
		r.refuse(line, CheckShape, "%s holds options_top and options or options_at, and a tool places its options in one of them", where)
	}
	for _, p := range []struct{ name, ptr string }{{"key", pl.Key}, {"value", pl.Value}, {"severity", pl.Severity}} {
		if steps := segments(p.ptr); len(steps) > 0 && isIndex(steps[0]) {
			r.refuse(line, CheckShape, "%s.options_top is set, and %s.%s is %q, whose first step is digits, so the top of the file is a list and holds no option by name", where, where, p.name, p.ptr)
		}
	}
}

// placement reads where each part of an entry goes. A list and an object take
// each, files take dir, and a severity is on the entry or in a table, not both.
func (r *reader) placement(m *ordered.Map) model.Placement {
	const where = "preset.placement"
	r.keys(m, where, "shape", "each", "dir", "key", "value", "options", "options_at", "options_top", "severity", "severity_table", "severity_item")
	shape := oneOf(r, m, where, "shape", model.Shapes(), true)
	pl := model.Placement{
		Shape:         shape,
		Key:           r.pointer(m, where, "key", false),
		Value:         r.pointer(m, where, "value", false),
		Options:       r.pointer(m, where, "options", false),
		OptionsAt:     r.optionsAt(m, where),
		OptionsTop:    r.boolean(m, where, "options_top", false),
		Severity:      r.pointer(m, where, "severity", false),
		SeverityTable: r.pointer(m, where, "severity_table", false),
	}
	_, hasEach := m.Get("each")
	_, hasDir := m.Get("dir")
	switch shape {
	case model.ShapeList, model.ShapeObject:
		pl.Each = r.pointer(m, where, "each", true)
		if hasDir {
			r.refuse(m.KeyLine("dir"), CheckShape, "%s.dir is set, and a %s takes each", where, shape)
		}
	case model.ShapeFiles:
		pl.Dir = r.relative(m, where, "dir", true)
		if hasEach {
			r.refuse(m.KeyLine("each"), CheckShape, "%s.each is set, and files take dir", where)
		}
	}
	r.optionsAtPlace(m, where, shape, pl, hasEach)
	r.optionsTopPlace(m, where, shape, pl)
	_, hasSeverity := m.Get("severity")
	_, hasTable := m.Get("severity_table")
	if hasSeverity && hasTable {
		r.refuse(m.KeyLine("severity_table"), CheckShape, "%s holds severity and severity_table, and a tool keeps its severity in one of them", where)
	}
	if i := r.sub(m, where, "severity_item", false); i != nil {
		pl.SeverityItem = r.severityItem(i)
		if !hasTable {
			r.refuse(m.KeyLine("severity_item"), CheckShape, "%s holds severity_item and no severity_table: the item is the shape of the table's items", where)
		}
	}
	return pl
}

// severityItem reads the shape of one override of the severity table: a list
// of maps names its key and level places, an object names only where the level
// is under a member.
func (r *reader) severityItem(m *ordered.Map) model.SeverityItem {
	const where = "preset.placement.severity_item"
	r.keys(m, where, "shape", "key", "key_form", "level")
	it := model.SeverityItem{Shape: oneOf(r, m, where, "shape", []model.Shape{model.ShapeList, model.ShapeObject}, true)}
	switch it.Shape {
	case model.ShapeList:
		it.Key = r.pointer(m, where, "key", true)
		it.KeyForm = oneOf(r, m, where, "key_form", model.KeyForms(), true)
		it.Level = r.pointer(m, where, "level", true)
	case model.ShapeObject:
		it.Level = r.pointer(m, where, "level", false)
		for _, k := range []string{"key", "key_form"} {
			if _, has := m.Get(k); has {
				r.refuse(m.KeyLine(k), CheckShape, "%s.%s is set, and an object's member is named by the entry, so it takes level only", where, k)
			}
		}
	case model.ShapeFiles:
		// oneOf above refused it, so nothing more is read.
	}
	return it
}

// lines reads the fixed top-level lines of the file: a name and a text (value)
// or a list of texts (items), one of the two, every name once.
func (r *reader) lines(m *ordered.Map, where string) []model.FixedLine {
	const key = "lines"
	var out []model.FixedLine
	seen := map[string]bool{}
	for i, e := range r.maps(m, where, key, maxArgv, false) {
		w := fmt.Sprintf("%s.%s[%d]", where, key, i)
		r.keys(e, w, "key", "value", "items")
		l := model.FixedLine{Key: r.word(e, w, "key", lineKey, lineKeySays, true)}
		_, hasValue := e.Get("value")
		_, hasItems := e.Get("items")
		switch {
		case hasValue == hasItems:
			r.refuse(e.Line, CheckShape, "%s holds exactly one of value and items, and it holds %s", w, map[bool]string{true: "both", false: "neither"}[hasValue])
			continue
		case hasValue:
			l.Value = r.word(e, w, "value", lineValue, lineValueSays, true)
			if l.Value == "" {
				continue
			}
		default:
			l.Items = r.words(e, w, "items", lineValue, lineValueSays, true)
			if len(l.Items) == 0 {
				continue
			}
		}
		if seen[l.Key] {
			r.refuse(e.KeyLine("key"), CheckShape, "%s.key is %q, which an earlier line already carries", w, l.Key)
			continue
		}
		seen[l.Key] = true
		out = append(out, l)
	}
	return out
}

// words reads the list under k as texts, each one a word of form, and refuses
// the rest, saying what a word is.
func (r *reader) words(m *ordered.Map, where, k string, form *regexp.Regexp, says string, required bool) []string {
	var out []string
	for i, v := range r.list(m, where, k, maxArgv, required) {
		s, ok := v.(string)
		if !ok || !form.MatchString(s) {
			r.refuse(m.KeyLine(k), CheckShape, "%s.%s[%d] is %s, and it must be %s", where, k, i, ordered.Repr(v), says)
			continue
		}
		out = append(out, s)
	}
	return out
}

// wiring reads how a project comes to use the preset: an import, an install,
// or both.
func (r *reader) wiring(m *ordered.Map) *model.Wiring {
	const where = "preset.wiring"
	r.keys(m, where, "import", "install")
	w := &model.Wiring{}
	if i := r.sub(m, where, "import", false); i != nil {
		const iw = where + ".import"
		r.keys(i, iw, "at", "form")
		w.Import = &model.ImportAt{
			At:   r.pointer(i, iw, "at", true),
			Form: oneOf(r, i, iw, "form", model.ImportForms(), true),
		}
	}
	_, installed := m.Get("install")
	if installed {
		w.Install = r.words(m, where, "install", installForm, installSays, false)
	}
	if _, imported := m.Get("import"); !imported && !installed {
		r.refuse(m.Line, CheckShape, "%s is empty: leave it out", where)
	}
	return w
}
