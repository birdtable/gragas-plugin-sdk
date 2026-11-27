package declarative

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
)

// conditionals are keys that would make a value depend on something, and
// references keys that would make one place stand for another. Neither is a
// key of the adapter's shape; each is refused as what it is, so a person sees
// why the shape has no such thing (gates.registry.8).
func conditionals() []string {
	return []string{"if", "when", "unless", "then", "else", "switch", "case"}
}

func references() []string { return []string{"ref", "include", "import", "use", "inherit"} }

// keys refuses each key of m that known does not name, saying what it would
// have been.
func (r *reader) keys(m *ordered.Map, where string, known ...string) {
	for _, k := range m.Keys() {
		if slices.Contains(known, k) {
			continue
		}
		line, low := m.KeyLine(k), strings.ToLower(k)
		switch {
		case slices.Contains(conditionals(), low):
			r.refuse(line, CheckConditional, "%s is a conditional, and %s states one value for every case; write it without %s", at(where, k), r.an(), k)
		case strings.HasPrefix(k, "$") || slices.Contains(references(), low):
			r.refuse(line, CheckReference, "%s refers to a value elsewhere, and %s writes every value out", at(where, k), r.an())
		case expressive(k):
			r.refuse(line, CheckExpression, "the key %q is written as an expression, and %s holds none", k, r.an())
		default:
			r.refuse(line, CheckKey, "%s is not a field of %s; the fields here are %s", at(where, k), r.an(), strings.Join(known, ", "))
		}
	}
}

// at is the dotted place of key k below where.
func at(where, k string) string {
	if where == "" {
		return k
	}
	return where + "." + k
}

// expressive is whether s holds a character an expression or a substitution
// is written with.
func expressive(s string) bool { return strings.ContainsAny(s, "{}$`\\") }

// text reads the string under k, refusing another type and one over n bytes.
// A key that is absent is "" with ok false; required says whether that is a
// refusal.
func (r *reader) text(m *ordered.Map, where, k string, n int, required bool) (string, bool) {
	v, has := m.Get(k)
	if !has {
		if required {
			r.refuse(m.Line, CheckShape, "%s is missing", at(where, k))
		}
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		r.refuse(m.KeyLine(k), CheckShape, "%s must be text, and it is %s", at(where, k), ordered.Repr(v))
		return "", false
	}
	if len(s) > n {
		r.refuse(m.KeyLine(k), CheckSize, "%s is %d bytes, over the cap of %d", at(where, k), len(s), n)
		return "", false
	}
	return s, true
}

// word reads the text under k and refuses it outside form, after an
// expression's characters, which are refused as an expression.
func (r *reader) word(m *ordered.Map, where, k string, form *regexp.Regexp, says string, required bool) string {
	s, ok := r.text(m, where, k, maxText, required)
	if !ok {
		return ""
	}
	if expressive(s) {
		r.refuse(m.KeyLine(k), CheckExpression, "%s holds %q, an expression's characters, and %s holds no expression", at(where, k), s, r.an())
		return ""
	}
	if !form.MatchString(s) {
		r.refuse(m.KeyLine(k), CheckShape, "%s is %q, and it must be %s", at(where, k), s, says)
		return ""
	}
	return s
}

// oneOf reads the text under k and refuses it outside set.
func oneOf[T ~string](r *reader, m *ordered.Map, where, k string, set []T, required bool) T {
	s, ok := r.text(m, where, k, maxText, required)
	if !ok {
		return ""
	}
	if !slices.Contains(set, T(s)) {
		r.refuse(m.KeyLine(k), CheckShape, "%s is %q, and it must be one of %s", at(where, k), s, joined(set))
		return ""
	}
	return T(s)
}

func joined[T ~string](set []T) string {
	out := make([]string, len(set))
	for i, v := range set {
		out[i] = string(v)
	}
	return strings.Join(out, ", ")
}

// boolean reads the true or false under k.
func (r *reader) boolean(m *ordered.Map, where, k string, required bool) bool {
	v, has := m.Get(k)
	if !has {
		if required {
			r.refuse(m.Line, CheckShape, "%s is missing", at(where, k))
		}
		return false
	}
	b, ok := v.(bool)
	if !ok {
		r.refuse(m.KeyLine(k), CheckShape, "%s must be true or false, and it is %s", at(where, k), ordered.Repr(v))
	}
	return b
}

// sub reads the mapping under k.
func (r *reader) sub(m *ordered.Map, where, k string, required bool) *ordered.Map {
	v, has := m.Get(k)
	if !has {
		if required {
			r.refuse(m.Line, CheckShape, "%s is missing", at(where, k))
		}
		return nil
	}
	s, ok := v.(*ordered.Map)
	if !ok {
		r.refuse(m.KeyLine(k), CheckShape, "%s must be a mapping, and it is %s", at(where, k), ordered.Repr(v))
	}
	return s
}

// list reads the list under k, of at least one item and at most n.
func (r *reader) list(m *ordered.Map, where, k string, n int, required bool) []any {
	v, has := m.Get(k)
	if !has {
		if required {
			r.refuse(m.Line, CheckShape, "%s is missing", at(where, k))
		}
		return nil
	}
	l, ok := v.([]any)
	switch {
	case !ok:
		r.refuse(m.KeyLine(k), CheckShape, "%s must be a list, and it is %s", at(where, k), ordered.Repr(v))
		return nil
	case len(l) == 0:
		r.refuse(m.KeyLine(k), CheckShape, "%s is an empty list; leave it out or give it an item", at(where, k))
		return nil
	case len(l) > n:
		r.refuse(m.KeyLine(k), CheckSize, "%s has %d items, over the cap of %d", at(where, k), len(l), n)
		return nil
	}
	return l
}

// maps reads the list under k as mappings, each one at its line.
func (r *reader) maps(m *ordered.Map, where, k string, n int, required bool) []*ordered.Map {
	l := r.list(m, where, k, n, required)
	var out []*ordered.Map
	for i, v := range l {
		e, ok := v.(*ordered.Map)
		if !ok {
			r.refuse(m.KeyLine(k), CheckShape, "%s[%d] must be a mapping, and it is %s", at(where, k), i, ordered.Repr(v))
			continue
		}
		out = append(out, e)
	}
	return out
}

// adapter reads the document's fields.
func (r *reader) adapter(m *ordered.Map) model.Adapter {
	r.keys(m, "", "format", "tool", "version", "tool_versions", "targets", "extends", "config", "config_loads_code", "run", "severities",
		"values", "docs_url", "preset", "resolve_from", "paths", "version_argv")
	if !r.format(m) {
		return model.Adapter{}
	}
	a := model.Adapter{
		Format:      model.FormatVersion,
		Tool:        r.word(m, "", "tool", slugForm, slugSays, true),
		Version:     r.word(m, "", "version", semverForm, semverSays, true),
		Extends:     r.boolean(m, "", "extends", true),
		ResolveFrom: r.relative(m, "", "resolve_from", false),
	}
	a.ToolVersions, a.Range = r.toolVersions(m)
	a.Targets = r.targets(m)
	a.Config = r.configs(m, a.Targets)
	if _, said := m.Get("config_loads_code"); said {
		loads := r.boolean(m, "", "config_loads_code", false)
		a.ConfigLoadsCode = &loads
	}
	a.Run = r.run(m)
	a.Severities = r.severities(m)
	a.Values = r.values(m)
	a.Preset = r.preset(m)
	if d := r.sub(m, "", "docs_url", false); d != nil {
		r.keys(d, "docs_url", "option")
		a.DocsURL = &model.DocsOption{Option: r.word(d, "docs_url", "option", wordForm, wordSays, true)}
	}
	a.Paths = r.paths(m)
	a.VersionArgv = r.argv(m, "", "version_argv", argvLiteral)
	return a
}

// format refuses a document of any format but this engine's: an older one,
// since adapters began at format 3 and there is nothing to carry forward, and
// a newer one, which is left as it is (store.schemas.2).
func (r *reader) format(m *ordered.Map) bool {
	v, has := m.Get("format")
	if !has {
		r.refuse(m.Line, CheckFormat, "format is missing; %s says the format it is written in, and this engine reads format %d", r.an(), model.FormatVersion)
		return false
	}
	n, ok := v.(int64)
	switch {
	case !ok:
		r.refuse(m.KeyLine("format"), CheckFormat, "format must be a whole number, and it is %s", ordered.Repr(v))
	case n < model.FormatVersion:
		r.refuse(m.KeyLine("format"), CheckFormat, "the document is format %d, older than %d, and %s began at format %d: write it in format %d", n, model.FormatVersion, r.many(), model.FormatVersion, model.FormatVersion)
	case n > model.FormatVersion:
		r.refuse(m.KeyLine("format"), CheckFormat, "the document is format %d, newer than this engine's %d: it is left as it is, and a newer gragas reads it", n, model.FormatVersion)
	default:
		return true
	}
	return false
}

// toolVersions reads and parses the tool version range.
func (r *reader) toolVersions(m *ordered.Map) (string, model.VersionRange) {
	s, ok := r.text(m, "", "tool_versions", maxText, true)
	if !ok {
		return "", model.VersionRange{}
	}
	vr, err := ParseRange(s)
	if err != nil {
		r.refuse(m.KeyLine("tool_versions"), CheckRange, "tool_versions %q: %v", s, err)
		return "", model.VersionRange{}
	}
	return s, vr
}

// targets reads the target types, each once.
func (r *reader) targets(m *ordered.Map) []model.TargetType {
	var out []model.TargetType
	for i, v := range r.list(m, "", "targets", maxList, true) {
		s, ok := v.(string)
		if !ok || !slices.Contains(model.TargetTypes(), model.TargetType(s)) {
			r.refuse(m.KeyLine("targets"), CheckShape, "targets[%d] is %s, and it must be one of %s", i, ordered.Repr(v), joined(model.TargetTypes()))
			continue
		}
		if slices.Contains(out, model.TargetType(s)) {
			r.refuse(m.KeyLine("targets"), CheckShape, "targets names %s twice", s)
			continue
		}
		out = append(out, model.TargetType(s))
	}
	return out
}

// configs reads how each target type's config is read, at most one each, and
// only for a target the adapter covers.
func (r *reader) configs(m *ordered.Map, targets []model.TargetType) []model.ConfigRead {
	var out []model.ConfigRead
	seen := map[model.TargetType]bool{}
	for i, c := range r.maps(m, "", "config", maxList, false) {
		where := fmt.Sprintf("config[%d]", i)
		r.keys(c, where, "target", "argv", "files", "format", "entries")
		cr := model.ConfigRead{
			Target: oneOf(r, c, where, "target", model.TargetTypes(), true),
			Format: oneOf(r, c, where, "format", model.ConfigFormats(), true),
		}
		_, hasArgv := c.Get("argv")
		_, hasFiles := c.Get("files")
		switch {
		case hasArgv && hasFiles:
			r.refuse(c.KeyLine("files"), CheckShape, "%s has argv and files, and a config read is one of them: a tool that prints its config is read by its print, never by its files", where)
		case hasFiles:
			cr.Files = r.fileRead(c, where)
		case !hasArgv:
			r.refuse(c.Line, CheckShape, "%s has neither argv nor files, and a config read is one of them: argv runs the tool and reads its print, files reads the entry files its project config names", where)
		default:
			cr.Argv = r.argv(c, where, "argv", argvConfig)
		}
		if cr.Target != "" && !slices.Contains(targets, cr.Target) {
			r.refuse(c.KeyLine("target"), CheckShape, "%s.target is %s, which targets does not list", where, cr.Target)
		}
		if seen[cr.Target] {
			r.refuse(c.KeyLine("target"), CheckShape, "%s.target is %s, which an earlier config reads already", where, cr.Target)
		}
		seen[cr.Target] = true
		if e := r.sub(c, where, "entries", true); e != nil {
			ew := where + ".entries"
			r.keys(e, ew, "each", "key", "value", "options", "options_top", "severity", "as", "also")
			cr.Entries = model.EntryMap{
				Each:       r.pointer(e, ew, "each", true),
				Key:        r.pointer(e, ew, "key", false),
				Value:      r.pointer(e, ew, "value", false),
				Options:    r.pointer(e, ew, "options", false),
				OptionsTop: r.boolean(e, ew, "options_top", false),
				Severity:   r.pointer(e, ew, "severity", false),
				As:         r.valueText(e, ew, "as", false),
			}
			cr.Entries.Also = r.sources(e, ew)
			r.optionsTopRead(e, ew, cr.Entries, hasFiles)
		}
		out = append(out, cr)
	}
	return out
}

// optionsTopRead holds an entry map's options_top to a read of entry files, each
// entry its own file read whole, as a files preset with options_top writes them:
// not in a printed config, whose entries share one document, and
// not beside options or more lists of entries.
func (r *reader) optionsTopRead(e *ordered.Map, ew string, m model.EntryMap, files bool) {
	if !m.OptionsTop {
		return
	}
	line := e.KeyLine("options_top")
	switch {
	case !files:
		r.refuse(line, CheckShape, "%s.options_top is set, and the read is of a printed config, whose entries share one document: only an entry file has a top of its own", ew)
	case m.Each != "":
		r.refuse(line, CheckShape, "%s.options_top is set, and each is %q: an entry file is read whole, each \"\"", ew, m.Each)
	case m.Options != "" || len(m.Also) > 0 || m.As != "":
		r.refuse(line, CheckShape, "%s holds options_top and options, also or as, and an entry's options are read from one place", ew)
	}
}

// dataEndings are the endings a files read may name, its project config's or
// its entry files': JSON and YAML, the forms the reader parses, so no file
// that is code is read as data .
func dataEndings() []string { return []string{".json", ".yaml", ".yml"} }

// fileRead reads a config read's files: the project config, a JSON or YAML
// file below the project root and never code, the pointer of its list of
// entry folders, and the endings of the entry files.
func (r *reader) fileRead(c *ordered.Map, where string) *model.FileRead {
	f := r.sub(c, where, "files", true)
	if f == nil {
		return nil
	}
	fw := where + ".files"
	r.keys(f, fw, "config", "dirs", "ext")
	out := &model.FileRead{Config: r.relative(f, fw, "config", true), Dirs: r.pointer(f, fw, "dirs", true)}
	if !slices.Contains(dataEndings(), strings.ToLower(path.Ext(out.Config))) && out.Config != "" {
		r.refuse(f.KeyLine("config"), CheckShape, "%s.config is %q, and a project config read as data is JSON or YAML: a config that is code is read only by the tool's own print", fw, out.Config)
	}
	for i, v := range r.list(f, fw, "ext", maxList, true) {
		place := fmt.Sprintf("%s.ext[%d]", fw, i)
		s, ok := v.(string)
		switch {
		case !ok:
			r.refuse(f.KeyLine("ext"), CheckShape, "%s must be text, and it is %s", place, ordered.Repr(v))
		case !slices.Contains(dataEndings(), s):
			r.refuse(f.KeyLine("ext"), CheckShape, "%s is %q, and an entry file read as data ends in one of %s: a file that is code is read only by the tool's own print", place, s, strings.Join(dataEndings(), ", "))
		case slices.Contains(out.Ext, s):
			r.refuse(f.KeyLine("ext"), CheckShape, "%s is %q, which the list holds already", place, s)
		default:
			out.Ext = append(out.Ext, s)
		}
	}
	return out
}

// run reads how the tool is run over fixtures.
func (r *reader) run(m *ordered.Map) model.Invocation {
	run := r.sub(m, "", "run", true)
	if run == nil {
		return model.Invocation{}
	}
	r.keys(run, "run", "argv", "output")
	inv := model.Invocation{Argv: r.argv(run, "run", "argv", argvRun)}
	if o := r.sub(run, "run", "output", true); o != nil {
		inv.Output = r.output(o)
	}
	return inv
}

// severities reads the tool's severity words.
func (r *reader) severities(m *ordered.Map) model.SeverityScale {
	s := r.sub(m, "", "severities", true)
	if s == nil {
		return model.SeverityScale{}
	}
	r.keys(s, "severities", "form", "levels", "warning", "default", "off_words")
	out := model.SeverityScale{Form: oneOf(r, s, "severities", "form", model.SeverityForms(), true)}
	for i, l := range r.maps(s, "severities", "levels", maxList, true) {
		where := fmt.Sprintf("severities.levels[%d]", i)
		r.keys(l, where, "name", "is")
		lv := model.SeverityLevelName{Name: r.word(l, where, "name", wordForm, wordSays, true), Is: oneOf(r, l, where, "is", model.Severities(), true)}
		if lv.Name != "" && slices.ContainsFunc(out.Levels, func(o model.SeverityLevelName) bool { return o.Name == lv.Name }) {
			r.refuse(l.KeyLine("name"), CheckShape, "%s.name %s is a level already", where, lv.Name)
		}
		out.Levels = append(out.Levels, lv)
	}
	if w := r.word(s, "severities", "warning", wordForm, wordSays, false); w != "" {
		if !slices.Contains(out.Levels, model.SeverityLevelName{Name: w, Is: model.SeverityWarning}) {
			r.refuse(s.KeyLine("warning"), CheckShape, "severities.warning is %s, which is no level that is a warning", w)
		}
		out.Warning = w
	}
	if d := r.word(s, "severities", "default", wordForm, wordSays, false); d != "" {
		if !slices.ContainsFunc(out.Levels, func(l model.SeverityLevelName) bool { return l.Name == d }) {
			r.refuse(s.KeyLine("default"), CheckShape, "severities.default is %s, which is no level", d)
		}
		out.Default = d
	}
	out.Off = r.offWords(s, out.Levels)
	return out
}

// packageForm is an npm package name, scoped or not, as npm writes it
// (lower-case, with no space or slash but the scope's).
var packageForm = regexp.MustCompile(`^(?:@[a-z0-9~-][a-z0-9._~-]*/)?[a-z0-9~-][a-z0-9._~-]*$`)

// binForm is a key of a package.json bin map: a command name.
var binForm = regexp.MustCompile(`^[a-z0-9~-][a-z0-9._~-]*$`)

// paths reads where a project's tool lives, one entry per ecosystem. An entry
// names a path or a package, never both and never neither; a
// bin key goes with a package alone, and a package is a node one.
func (r *reader) paths(m *ordered.Map) []model.ToolPath {
	var out []model.ToolPath
	for i, p := range r.maps(m, "", "paths", maxList, false) {
		where := fmt.Sprintf("paths[%d]", i)
		r.keys(p, where, "ecosystem", "path", "package", "bin")
		tp := model.ToolPath{Ecosystem: oneOf(r, p, where, "ecosystem", model.Ecosystems(), true)}
		_, hasPath := p.Get("path")
		_, hasPackage := p.Get("package")
		_, hasBin := p.Get("bin")
		switch {
		case hasPath && hasPackage:
			r.refuse(p.KeyLine("package"), CheckShape, "%s names both path and package, and an entry names one of the two", where)
		case !hasPath && !hasPackage:
			r.refuse(p.Line, CheckShape, "%s names neither path nor package, and an entry names one of the two", where)
		case hasPath:
			tp.Path = r.relative(p, where, "path", true)
			if hasBin {
				r.refuse(p.KeyLine("bin"), CheckShape, "%s names a bin with no package, and a bin is the key of a package's bin map", where)
			}
		default:
			tp.Package = r.word(p, where, "package", packageForm, "an npm package name, as npm writes it", true)
			if hasBin {
				tp.Bin = r.word(p, where, "bin", binForm, "a command name, the key of a package.json bin map", true)
			}
			if tp.Ecosystem != "" && tp.Ecosystem != model.EcosystemNode {
				r.refuse(p.KeyLine("package"), CheckShape, "%s names a package in the %s ecosystem, and a package is a node one", where, tp.Ecosystem)
			}
		}
		if tp.Ecosystem != "" && slices.ContainsFunc(out, func(o model.ToolPath) bool { return o.Ecosystem == tp.Ecosystem }) {
			r.refuse(p.KeyLine("ecosystem"), CheckShape, "%s.ecosystem %s has a path already", where, tp.Ecosystem)
		}
		out = append(out, tp)
	}
	return out
}

// offWords reads the words the tool uses for off: each a word, none a level
// of the tool's, none twice. The list is absent where the tool has no off
// word.
func (r *reader) offWords(s *ordered.Map, levels []model.SeverityLevelName) []string {
	var out []string
	for i, item := range r.list(s, "severities", "off_words", maxList, false) {
		where := fmt.Sprintf("severities.off_words[%d]", i)
		w, ok := item.(string)
		switch {
		case !ok:
			r.refuse(s.KeyLine("off_words"), CheckShape, "%s must be text, and it is %s", where, ordered.Repr(item))
		case expressive(w):
			r.refuse(s.KeyLine("off_words"), CheckExpression, "%s holds %q, an expression's characters, and an adapter holds no expression", where, w)
		case !wordForm.MatchString(w) || len(w) > maxText:
			r.refuse(s.KeyLine("off_words"), CheckShape, "%s is %q, and it must be %s", where, w, wordSays)
		case slices.Contains(out, w):
			r.refuse(s.KeyLine("off_words"), CheckShape, "%s %s is an off word already", where, w)
		case slices.ContainsFunc(levels, func(l model.SeverityLevelName) bool { return l.Name == w }):
			r.refuse(s.KeyLine("off_words"), CheckShape, "%s %s is a level, and a word is a level or off, not both", where, w)
		default:
			out = append(out, w)
		}
	}
	return out
}

// values reads how the tool's entry values stand against one another: the
// values that read as off and the order of the others, loosest first.
// Each is a scalar as the adapter writes it, none twice, and
// none both off and in the order; a malformed one is refused with a finding,
// never guessed at. Absent, the adapter names no order, and a value that
// differs from the preset's has no word.
func (r *reader) values(m *ordered.Map) model.ValueScale {
	v := r.sub(m, "", "values", false)
	if v == nil {
		return model.ValueScale{}
	}
	r.keys(v, "values", "off_values", "order", "keys")
	_, off := v.Get("off_values")
	_, order := v.Get("order")
	_, keys := v.Get("keys")
	if !off && !order && !keys {
		r.refuse(v.Line, CheckShape, "values names none of off_values, order and keys; leave it out or give it one")
	}
	seen := map[string]string{}
	return model.ValueScale{Off: r.scalars(v, "values", "off_values", seen), Order: r.scalars(v, "values", "order", seen), Keys: r.keyScales(v)}
}

// keyScales reads values.keys: each an entry key with its own off values and
// order, a key at most once, and each holding at least one of the two.
func (r *reader) keyScales(v *ordered.Map) []model.KeyScale {
	var out []model.KeyScale
	for i, k := range r.maps(v, "values", "keys", maxList, false) {
		where := fmt.Sprintf("values.keys[%d]", i)
		r.keys(k, where, "key", "off_values", "order")
		ks := model.KeyScale{Key: r.word(k, where, "key", wordForm, wordSays, true)}
		if ks.Key != "" && slices.ContainsFunc(out, func(o model.KeyScale) bool { return o.Key == ks.Key }) {
			r.refuse(k.KeyLine("key"), CheckShape, "%s.key %s has a scale already; a key has one direction", where, ks.Key)
		}
		_, off := k.Get("off_values")
		_, order := k.Get("order")
		if !off && !order {
			r.refuse(k.Line, CheckShape, "%s names neither off_values nor order; give it one", where)
		}
		seen := map[string]string{}
		ks.Off, ks.Order = r.scalars(k, where, "off_values", seen), r.scalars(k, where, "order", seen)
		out = append(out, ks)
	}
	return out
}

// scalars reads the list under k of values: each a bool, a finite number or a
// one-line text with no expression's characters, and none already in seen,
// which maps each value read, as stable text, to the list it is in.
func (r *reader) scalars(v *ordered.Map, owner, k string, seen map[string]string) []any {
	var out []any
	line := v.KeyLine(k)
	for i, item := range r.list(v, owner, k, maxList, false) {
		where := fmt.Sprintf("%s.%s[%d]", owner, k, i)
		switch x := item.(type) {
		case bool, int64:
		case float64:
			// x-x is 0 for every finite number and NaN for NaN and either
			// infinity; the core imports no math package.
			if x-x != 0 {
				r.refuse(line, CheckShape, "%s is %s, and a value of the order is a finite number", where, ordered.Repr(item))
				continue
			}
		case string:
			if expressive(x) {
				r.refuse(line, CheckExpression, "%s holds %q, an expression's characters, and an adapter holds no expression", where, x)
				continue
			}
			if x == "" || len(x) > maxText || strings.ContainsAny(x, "\n\r") {
				r.refuse(line, CheckShape, "%s must be one line of text of 1 to %d bytes", where, maxText)
				continue
			}
		default:
			r.refuse(line, CheckShape, "%s must be a bool, a number or a text, and it is %s", where, ordered.Repr(item))
			continue
		}
		key := stable(item)
		if in, twice := seen[key]; twice {
			if in == k {
				r.refuse(line, CheckShape, "%s %s is in %s.%s already", where, key, owner, k)
			} else {
				r.refuse(line, CheckShape, "%s %s is in %s.%s too, and a value is off or in the order, not both", where, key, owner, in)
			}
			continue
		}
		seen[key] = k
		out = append(out, item)
	}
	return out
}

// valueText reads the text under k that stands for a value, as JSON text: a
// scalar the tool prints, such as true, 2 or "strict".
func (r *reader) valueText(m *ordered.Map, where, k string, required bool) string {
	s, ok := r.text(m, where, k, maxText, required)
	if !ok {
		return ""
	}
	if expressive(s) {
		r.refuse(m.KeyLine(k), CheckExpression, "%s holds %q, an expression's characters, and an adapter holds no expression", at(where, k), s)
		return ""
	}
	switch v, err := ordered.ParseStrict([]byte(s)); v.(type) {
	case bool, int64, float64, string:
		if err == nil {
			return s
		}
	}
	r.refuse(m.KeyLine(k), CheckShape, "%s is %q, and it must be one value as JSON text: true, false, a number or a quoted text", at(where, k), s)
	return ""
}

// sources reads the further lists of entries under entries.also: each with
// its list, its key where the list is a list, and the value of its entries.
func (r *reader) sources(e *ordered.Map, ew string) []model.EntrySource {
	var out []model.EntrySource
	for i, s := range r.maps(e, ew, "also", maxList, false) {
		where := fmt.Sprintf("%s.also[%d]", ew, i)
		r.keys(s, where, "each", "key", "as")
		out = append(out, model.EntrySource{
			Each: r.pointer(s, where, "each", true),
			Key:  r.pointer(s, where, "key", false),
			As:   r.valueText(s, where, "as", true),
		})
	}
	return out
}
