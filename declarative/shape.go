package declarative

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
)

// The forms a text value must have, and how a person is told each.
const (
	slugSays   = "lowercase letters and digits in words joined by -"
	semverSays = "major.minor.patch"
	wordSays   = "one word of letters, digits and . _ / -"
	// literalSays is the flag-and-word form of a literal argument:
	// one token, so no literal carries code with a space, a
	// quote, a bracket or a shell character in it.
	literalSays = "one flag or word of letters, digits and . _ / : , = -, with no space, quote, bracket or shell character"
)

// pointerClass is what a pointer's step may not hold: a control (Cc), a format
// character (Cf, the zero-width and the bidi controls among them) or a line or
// paragraph separator (Zl, Zp). It is the one definition of the class: the
// reader's pointerRE is made from it, and the adapter schema's pointer and
// options_at patterns spell it, held equal by TestThePointerClassIsOneDefinition.
const pointerClass = `\p{Cc}\p{Cf}\p{Zl}\p{Zp}`

var (
	slugForm    = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	semverForm  = regexp.MustCompile(`^(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)$`)
	wordForm    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	literalForm = regexp.MustCompile(`^[A-Za-z0-9._/:,=-]+$`)
	stepForm    = regexp.MustCompile(`^\.{0,2}[A-Za-z0-9_-][A-Za-z0-9._-]*$`)
	pointerRE   = regexp.MustCompile(`^(?:/(?:[^~/` + pointerClass + `]|~[01])*)*$`)
	controlRE   = regexp.MustCompile(`[\x00-\x1f\x7f]`)
	groupRE     = regexp.MustCompile(`^[a-z]+$`)
)

// argvTakes is what an argument list may hold besides literals.
type argvTakes int

const (
	// argvRun is the run's list: any placeholder, and the fixtures always.
	argvRun argvTakes = iota
	// argvConfig is a config read's list: any placeholder.
	argvConfig
	// argvLiteral is the version list: literals only.
	argvLiteral
)

// argv reads the argument list at key k: each item a literal, or one
// placeholder of the closed set standing alone, each placeholder at most
// once, as takes allows.
func (r *reader) argv(m *ordered.Map, where, k string, takes argvTakes) []string {
	l := r.list(m, where, k, maxArgv, true)
	if l == nil {
		return nil
	}
	line, out := m.KeyLine(k), make([]string, 0, len(l))
	for i, v := range l {
		place := fmt.Sprintf("%s[%d]", at(where, k), i)
		s, ok := v.(string)
		switch {
		case !ok:
			r.refuse(line, CheckShape, "%s must be text, and it is %s", place, ordered.Repr(v))
		case s == "":
			r.refuse(line, CheckShape, "%s is empty", place)
		case len(s) > maxText:
			r.refuse(line, CheckSize, "%s is %d bytes, over the cap of %d", place, len(s), maxText)
		case takes == argvLiteral && slices.Contains(model.Placeholders(), model.Placeholder(s)):
			r.refuse(line, CheckPlaceholder, "%s is %s, and this list takes no placeholder: it runs before any file is placed", place, s)
		case slices.Contains(model.Placeholders(), model.Placeholder(s)):
			if slices.Contains(out, s) {
				r.refuse(line, CheckPlaceholder, "%s is %s, which the list holds already", place, s)
			}
		case placeholderShaped(s):
			r.refuse(line, CheckPlaceholder, "%s is %s, which is no placeholder; the placeholders are %s, each a whole argument", place, s, joined(model.Placeholders()))
		case expressive(s):
			r.refuse(line, CheckExpression, "%s is %q, which holds an expression's characters; an argument is a literal or a whole placeholder (%s)", place, s, joined(model.Placeholders()))
		case controlRE.MatchString(s):
			r.refuse(line, CheckShape, "%s holds a control character", place)
		case !literalForm.MatchString(s):
			r.refuse(line, CheckShape, "%s is %q, and a literal is %s", place, s, literalSays)
		}
		out = append(out, s)
	}
	if takes == argvRun && !slices.Contains(out, string(model.PlaceFixtures)) {
		r.refuse(line, CheckPlaceholder, "%s does not take %s, and a run is over fixtures", at(where, k), model.PlaceFixtures)
	}
	return out
}

// placeholderShaped is whether s is written as a placeholder: a word in
// braces, with nothing around it.
func placeholderShaped(s string) bool {
	inner, ok := strings.CutPrefix(s, "{")
	if !ok {
		return false
	}
	inner, ok = strings.CutSuffix(inner, "}")
	return ok && inner != "" && !strings.ContainsAny(inner, "{}")
}

// pointer reads a JSON pointer (RFC 6901), refusing one that holds an
// expression's characters as well: a pointer is a plain lookup.
func (r *reader) pointer(m *ordered.Map, where, k string, required bool) string {
	s, ok := r.text(m, where, k, maxText, required)
	switch {
	case !ok:
		return ""
	case expressive(s):
		r.refuse(m.KeyLine(k), CheckExpression, "%s is %q, which holds an expression's characters; a pointer is a plain lookup", at(where, k), s)
	case !pointerRE.MatchString(s):
		r.refuse(m.KeyLine(k), CheckShape, "%s is %q, which is no JSON pointer: \"\" or steps each starting with /, ~ written ~0 and / written ~1", at(where, k), s)
	default:
		return s
	}
	return ""
}

// relative reads a slash-separated path below the config set: no leading
// slash, no drive, no backslash, and no step that is . or ..
func (r *reader) relative(m *ordered.Map, where, k string, required bool) string {
	s, ok := r.text(m, where, k, maxText, required)
	if !ok {
		return ""
	}
	if expressive(s) {
		r.refuse(m.KeyLine(k), CheckExpression, "%s is %q, which holds an expression's characters", at(where, k), s)
		return ""
	}
	for step := range strings.SplitSeq(s, "/") {
		if !stepForm.MatchString(step) {
			r.refuse(m.KeyLine(k), CheckShape, "%s is %q, and it must be a path below the config set: slash-separated, no step empty, . or .., no drive", at(where, k), s)
			return ""
		}
	}
	return s
}

// output reads how a diagnostic is read: each format takes its own fields
// and no other's.
func (r *reader) output(o *ordered.Map) model.AdapterOutput {
	const where = "run.output"
	r.keys(o, where, "format", "each", "fields", "pattern", "only_form", "ran_codes", "error_keys")
	out := model.AdapterOutput{Format: oneOf(r, o, where, "format", model.OutputFormats(), true)}
	takes := map[model.OutputFormat][]string{
		model.OutputSARIF: nil,
		model.OutputJSON:  {"each", "fields"},
		model.OutputLines: {"pattern", "only_form"},
	}
	if out.Format == "" {
		return out
	}
	for _, k := range []string{"each", "fields", "pattern", "only_form"} {
		_, has := o.Get(k)
		switch want := slices.Contains(takes[out.Format], k); {
		case has && !want:
			r.refuse(o.KeyLine(k), CheckOutput, "%s.%s is not read for the format %s", where, k, out.Format)
		case !has && want:
			r.refuse(o.Line, CheckOutput, "%s.%s is missing, and the format %s reads it", where, k, out.Format)
		}
	}
	switch out.Format {
	case model.OutputSARIF:
		// SARIF names its own fields; nothing more is read.
	case model.OutputJSON:
		out.Each = r.pointer(o, where, "each", false)
		out.Fields = r.fields(o, where)
	case model.OutputLines:
		out.OnlyForm = r.boolean(o, where, "only_form", false)
		if _, has := o.Get("only_form"); has && !out.OnlyForm {
			r.refuse(o.KeyLine("only_form"), CheckOutput, "%s.only_form is false, and lines are read only where the tool prints no machine form", where)
		}
		out.Pattern = r.pattern(o, where)
	}
	r.health(o, where, &out)
	return out
}

// maxRanCodes and maxErrorKeys cap the two lists an output may declare.
const (
	maxRanCodes  = 16
	maxErrorKeys = 32
)

// errorKeyForm is a key a tool gives a diagnostic: one token, no expression's
// characters.
var errorKeyForm = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/@:-]{0,127}$`)

// health reads, for every format, the exit codes that mean the tool ran and
// the keys that mean it failed, which a proof run holds a side to.
func (r *reader) health(o *ordered.Map, where string, out *model.AdapterOutput) {
	for i, v := range r.list(o, where, "ran_codes", maxRanCodes, false) {
		n, ok := v.(int64)
		switch {
		case !ok || n < 0 || n > 255:
			r.refuse(o.KeyLine("ran_codes"), CheckShape, "%s[%d] must be an exit code, a whole number from 0 to 255, and it is %s", at(where, "ran_codes"), i, ordered.Repr(v))
		case slices.Contains(out.RanCodes, int(n)):
			r.refuse(o.KeyLine("ran_codes"), CheckShape, "%s lists %d twice", at(where, "ran_codes"), n)
		default:
			out.RanCodes = append(out.RanCodes, int(n))
		}
	}
	for i, v := range r.list(o, where, "error_keys", maxErrorKeys, false) {
		s, ok := v.(string)
		switch {
		case !ok || !errorKeyForm.MatchString(s):
			r.refuse(o.KeyLine("error_keys"), CheckShape, "%s[%d] must be a key of one token, and it is %s", at(where, "error_keys"), i, ordered.Repr(v))
		case slices.Contains(out.ErrorKeys, s):
			r.refuse(o.KeyLine("error_keys"), CheckShape, "%s lists %s twice", at(where, "error_keys"), s)
		default:
			out.ErrorKeys = append(out.ErrorKeys, s)
		}
	}
}

// fields reads where each named field is in one diagnostic of a JSON
// document. The key is always named.
func (r *reader) fields(o *ordered.Map, where string) *model.Fields {
	f := r.sub(o, where, "fields", false)
	if f == nil {
		return nil
	}
	fw := where + ".fields"
	names := make([]string, len(model.FieldNames()))
	for i, n := range model.FieldNames() {
		names[i] = string(n)
	}
	r.keys(f, fw, names...)
	out := &model.Fields{
		Key: r.pointer(f, fw, "key", true), Message: r.pointer(f, fw, "message", false),
		Path: r.pointer(f, fw, "path", false), Line: r.pointer(f, fw, "line", false),
		Column: r.pointer(f, fw, "column", false), Severity: r.pointer(f, fw, "severity", false),
		URL: r.pointer(f, fw, "url", false),
	}
	return out
}

// pattern reads the expression one line of output matches. It is the one
// value that may hold an expression's characters, since it is compiled as a
// regular expression, which only matches text and runs in time linear in the
// line. Its named groups are field names, the key among them.
func (r *reader) pattern(o *ordered.Map, where string) string {
	s, ok := r.text(o, where, "pattern", maxPattern, false)
	if !ok {
		return ""
	}
	re, err := regexp.Compile(s)
	if err != nil {
		r.refuse(o.KeyLine("pattern"), CheckOutput, "%s.pattern does not compile: %v", where, err)
		return ""
	}
	var named []string
	for _, g := range re.SubexpNames()[1:] {
		if g == "" {
			continue
		}
		if !groupRE.MatchString(g) || !slices.Contains(model.FieldNames(), model.Field(g)) {
			r.refuse(o.KeyLine("pattern"), CheckOutput, "%s.pattern names the group %s, and the groups are %s", where, g, joined(model.FieldNames()))
			continue
		}
		if slices.Contains(named, g) {
			r.refuse(o.KeyLine("pattern"), CheckOutput, "%s.pattern names the group %s twice", where, g)
		}
		named = append(named, g)
	}
	if !slices.Contains(named, string(model.FieldKey)) {
		r.refuse(o.KeyLine("pattern"), CheckOutput, "%s.pattern names no group key, and a diagnostic maps to a tool entry by its key", where)
	}
	return s
}
