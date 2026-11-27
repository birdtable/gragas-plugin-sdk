package declarative

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/canon"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
)

// The caps on what a tool printed. It is a program the engine does not
// control, so what it prints is bounded before and while it is read.
const (
	// MaxPrinted is the most bytes of one tool output that are read.
	MaxPrinted = 16 << 20
	// MaxFindings is the most diagnostics or entries one output may hold.
	MaxFindings = 100000
	maxLine     = 64 << 10
)

// ErrPrinted is output that does not read as the adapter says the tool
// prints it. Every refusal below wraps it and says where.
var ErrPrinted = errors.New("the tool's output does not read as its adapter says")

func printed(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrPrinted, fmt.Sprintf(format, a...))
}

// ReadDiagnostics reads the diagnostics a tool printed, as the adapter's
// output declaration says: SARIF names its own fields, a JSON document is
// walked by the declared pointers, and lines are matched by the declared
// pattern. Output with no finding gives none and no error. Output the
// declaration does not fit, such as a finding with no key, is refused whole:
// a half-read run is no proof (gates.plants.1).
func ReadDiagnostics(o model.AdapterOutput, out []byte) ([]model.Diagnostic, error) {
	if len(out) > MaxPrinted {
		return nil, printed("%d bytes, over the %d read", len(out), MaxPrinted)
	}
	switch o.Format {
	case model.OutputSARIF:
		return readSARIF(out)
	case model.OutputJSON:
		return readJSON(o, out)
	case model.OutputLines:
		return readLines(o, out)
	}
	return nil, printed("the format %q is not one of sarif, json and lines", o.Format)
}

// ReadEntries reads the entries of the effective config a tool printed, as
// the adapter's entry map says. In an object each member's name is the key;
// in a list the key is read at the declared pointer.
func ReadEntries(c model.ConfigRead, out []byte) ([]model.ToolEntry, error) {
	typed, err := readEntries(c, out)
	if err != nil {
		return nil, err
	}
	entries := make([]model.ToolEntry, 0, len(typed))
	for _, e := range typed {
		entries = append(entries, e.ToolEntry)
	}
	return entries, nil
}

// entry is one tool entry with its value and its options as the document holds
// them, typed: what Observe compares, never the text again.
type entry struct {
	model.ToolEntry
	value, options any
	// null is whether the config prints the value, and prints it as null,
	// which is not the same as printing none.
	null bool
}

// readEntries is ReadEntries with each entry's typed value and options. The
// document is read by its own format's rules: JSON by JSON's (canon.Decode),
// and YAML only where no plain value of it reads one way under YAML 1.1 and
// another under 1.2, since which one the tool read is not known.
func readEntries(c model.ConfigRead, out []byte) ([]entry, error) {
	if len(out) > MaxPrinted {
		return nil, printed("%d bytes, over the %d read", len(out), MaxPrinted)
	}
	doc, err := configDocument(c.Format, out)
	if err != nil {
		return nil, err
	}
	return entriesOf(c, doc)
}

// entriesOf is the entries of doc, a config document, as the read's entry
// map says: what a printed config and a list of entry files both hold.
func entriesOf(c model.ConfigRead, doc any) ([]entry, error) {
	at, ok := pointer(doc, c.Entries.Each)
	if !ok {
		return nil, printed("%q is not in the printed config", c.Entries.Each)
	}
	members, err := keyedMembers(c.Entries, at)
	if err != nil {
		return nil, err
	}
	var entries []entry
	for _, m := range members {
		if len(entries) >= MaxFindings {
			return nil, printed("more than %d entries", MaxFindings)
		}
		entries = append(entries, entryOf(c.Entries, m.key, m.value))
	}
	for _, s := range c.Entries.Also {
		more, ok := pointer(doc, s.Each)
		if !ok {
			return nil, printed("%q is not in the printed config", s.Each)
		}
		if more == nil {
			// A tool prints null for a list it holds nothing in.
			continue
		}
		ms, err := keyedMembers(model.EntryMap{Each: s.Each, Key: s.Key}, more)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			if len(entries) >= MaxFindings {
				return nil, printed("more than %d entries", MaxFindings)
			}
			entries = append(entries, entry{Key: m.key, Value: s.As, value: asValue(s.As)})
		}
	}
	return entries, nil
}

// configDocument reads a printed config by its format's own rules.
func configDocument(format model.ConfigFormat, out []byte) (any, error) {
	switch format {
	case model.ConfigJSON:
		return jsonDocument(out)
	case model.ConfigYAML:
		v, err := document(out)
		if err != nil {
			return nil, err
		}
		s, ok, err := apart(out)
		switch {
		case err != nil:
			return nil, printed("%v", err)
		case ok:
			return nil, printed("line %d holds the value %q, which YAML 1.1 and YAML 1.2 read as different values, and which of them the tool read is not known: nothing in it is compared", s.Line, s.Text)
		}
		return v, nil
	case model.ConfigYAML12:
		v, err := ordered.ParseStrict12(out)
		if err != nil {
			return nil, printed("%v", err)
		}
		if v == nil {
			return nil, printed("it printed no document")
		}
		return v, nil
	}
	return nil, printed("the format %q is neither json nor yaml", format)
}

// jsonDocument reads printed JSON by JSON's own rules, canon's: one value, no
// key twice, every number as the binary64 nearest to it and none past what
// one holds exactly, so 1e3 is 1000, a quoted "true" is text and yes is no
// JSON at all. It answers the tree the rest of the reader walks. Every value
// in it is canon's; an object's members keep the order the tool printed them
// in, taken from the same text read as YAML where that read succeeds, and
// canon's order of keys where it does not, so the order decides nothing.
func jsonDocument(out []byte) (any, error) {
	if len(strings.TrimSpace(string(out))) == 0 {
		return nil, printed("it printed no document")
	}
	v, err := canon.Decode(out)
	if err != nil {
		return nil, printed("%v", err)
	}
	hint, err := ordered.ParseStrict(out)
	if err != nil {
		hint = nil
	}
	return fromCanon(v, hint), nil
}

// fromCanon is a canon value as the reader's tree holds one: a mapping, a
// list, text, a whole number, a binary64, a boolean or nil. hint is the same
// value read as YAML, or nil, and gives only an object's order of members and
// its lines, never a value.
func fromCanon(v canon.Value, hint any) any {
	switch x := v.(type) {
	case canon.Object:
		h, isMap := hint.(*ordered.Map)
		if !isMap {
			h = nil
		}
		line := 0
		if h != nil {
			line = h.Line
		}
		m := ordered.NewMap(line)
		for _, k := range memberOrder(x, h) {
			var sub any
			if h != nil {
				sub = h.Val(k)
			}
			m.Set(k, fromCanon(x[k], sub), 0)
		}
		return m
	case canon.Array:
		h, isList := hint.([]any)
		if !isList {
			h = nil
		}
		out := make([]any, len(x))
		for i, e := range x {
			var sub any
			if i < len(h) {
				sub = h[i]
			}
			out[i] = fromCanon(e, sub)
		}
		return out
	case canon.String:
		return string(x)
	case canon.Int:
		return int64(x)
	case canon.Float:
		return float64(x)
	case canon.Bool:
		return bool(x)
	}
	return nil
}

// memberOrder is the keys of x in the order hint holds them, where hint holds
// exactly x's keys, and in canon's order of keys otherwise.
func memberOrder(x canon.Object, hint *ordered.Map) []string {
	if hint != nil && hint.Len() == len(x) {
		keys := hint.Keys()
		if !slices.ContainsFunc(keys, func(k string) bool { _, ok := x[k]; return !ok }) {
			return keys
		}
	}
	keys := make([]string, 0, len(x))
	for k := range x {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, canon.Compare)
	return keys
}

// apart is the first scalar of the YAML text out, key or value, that YAML
// 1.1 and YAML 1.2's core schema read as different values, and whether there
// is one. It judges the node
// tree the one YAML owner reads, never the text: each scalar's reading under
// 1.1 is the value the reader compares, and its reading under 1.2 is what a
// tool on that version runs with. A scalar with a tag is read by its tag, and
// a quoted, literal or folded one is text under both. A plain y, Y, n or N is
// judged apart too: YAML 1.1's own tables read it as a boolean, where the
// reader's tables, as most 1.1 readers', read it as text, so which a tool
// read is not known either.
func apart(out []byte) (ordered.Scalar, bool, error) {
	scalars, err := ordered.Scalars(out)
	if err != nil {
		return ordered.Scalar{}, false, err
	}
	for _, s := range scalars {
		if stable(s.Read11) != stable(s.Read12) || s.Plain && s.Tag == "" && slices.Contains([]string{"y", "Y", "n", "N"}, s.Text) {
			return s, true, nil
		}
	}
	return ordered.Scalar{}, false, nil
}

// member is one entry as printed: its key and its whole value.
type member struct {
	key   string
	value any
}

// keyedMembers is the members of at, each with the key m says: an object's
// member names, or the text at m.Key in each item of a list. A member with no
// key is refused, whichever form it is in.
func keyedMembers(m model.EntryMap, at any) ([]member, error) {
	switch list := at.(type) {
	case *ordered.Map:
		var out []member
		for i, k := range list.Keys() {
			if k == "" {
				return nil, printed("entry %d has no key: its name is empty", i+1)
			}
			out = append(out, member{k, list.Val(k)})
		}
		return out, nil
	case []any:
		var out []member
		for i, v := range list {
			k, ok := pointer(v, m.Key)
			s, isText := k.(string)
			if !ok || !isText || s == "" {
				return nil, printed("entry %d has no key at %q", i+1, m.Key)
			}
			out = append(out, member{s, v})
		}
		return out, nil
	}
	return nil, printed("%q is neither an object nor a list", m.Each)
}

// entryOf is the entry of key whose printed value is v, read as m says. An
// entry map naming nothing but Each is an object of bare members, as a
// compiler's options: the value of an entry is its member's whole value.
func entryOf(m model.EntryMap, key string, v any) entry {
	if m.Key == "" && m.Value == "" && m.Options == "" && !m.OptionsTop && m.Severity == "" && m.As == "" && len(m.Also) == 0 {
		return entry{Key: key, Value: asText(v), value: v, null: v == nil}
	}
	e := entry{Key: key, Value: text(v, m.Value), Options: compact(v, m.Options), Severity: text(v, m.Severity)}
	if m.OptionsTop {
		if top := topOptions(v, m); top.Len() > 0 {
			e.Options, e.options = ordered.JSON(top, -1), top
		}
	}
	if m.Value != "" {
		var there bool
		e.value, there = pointer(v, m.Value)
		e.null = there && e.value == nil
	} else if m.As != "" {
		// An entry of a list that names no value pointer takes the list's own
		// value, written in the adapter as JSON text.
		e.Value, e.value = m.As, asValue(m.As)
	}
	if m.Options != "" {
		e.options, _ = pointer(v, m.Options)
	}
	return e
}

// topOptions is, for an entry read whole from its file, the members at its top
// that the map's key, value and severity pointers do not name: the options a
// files preset with options_top wrote there. An entry that is no
// mapping has none.
func topOptions(v any, m model.EntryMap) *ordered.Map {
	out := ordered.NewMap(0)
	entry, ok := v.(*ordered.Map)
	if !ok {
		return out
	}
	named := map[string]bool{}
	for _, p := range []string{m.Key, m.Value, m.Severity} {
		if steps := segments(p); len(steps) > 0 {
			named[steps[0]] = true
		}
	}
	for _, k := range entry.Keys() {
		if !named[k] {
			got, _ := entry.Get(k)
			out.Set(k, got, 0)
		}
	}
	return out
}

// asValue is the value an adapter's as gives, a JSON scalar the decoder has
// already checked, as the typed value an entry holds.
func asValue(as string) any {
	v, err := canon.Decode([]byte(as))
	if err != nil {
		return as
	}
	return fromCanon(v, nil)
}

// document reads one JSON or YAML document a tool printed. An empty one is
// refused: a tool that ran prints something.
func document(out []byte) (any, error) {
	v, err := ordered.ParseStrict(out)
	if err != nil {
		return nil, printed("%v", err)
	}
	if v == nil {
		return nil, printed("it printed no document")
	}
	return v, nil
}

// pointer walks a JSON pointer (RFC 6901) over a parsed document.
func pointer(v any, p string) (any, bool) {
	if p == "" {
		return v, true
	}
	for step := range strings.SplitSeq(p[1:], "/") {
		step = strings.ReplaceAll(strings.ReplaceAll(step, "~1", "/"), "~0", "~")
		switch c := v.(type) {
		case *ordered.Map:
			next, ok := c.Get(step)
			if !ok {
				return nil, false
			}
			v = next
		case []any:
			i, err := strconv.Atoi(step)
			if err != nil || i < 0 || i >= len(c) || strconv.Itoa(i) != step {
				return nil, false
			}
			v = c[i]
		default:
			return nil, false
		}
	}
	return v, true
}

// text is the value at p from v as text: a string as it is, anything else as
// JSON on one line, "" where p is empty or the value is not there.
func text(v any, p string) string {
	if p == "" {
		return ""
	}
	got, ok := pointer(v, p)
	if !ok || got == nil {
		return ""
	}
	if s, isText := got.(string); isText {
		return s
	}
	return ordered.JSON(got, -1)
}

// asText is v as text: a string as it is, null as "", anything else as JSON
// on one line.
func asText(v any) string {
	if v == nil {
		return ""
	}
	if s, isText := v.(string); isText {
		return s
	}
	return ordered.JSON(v, -1)
}

// word is the string at p from v, "" where p is empty, the value is not there
// or it is not a string: a key is a name, never a structure.
func word(v any, p string) string {
	got, ok := pointer(v, p)
	if s, isText := got.(string); ok && isText {
		return s
	}
	return ""
}

// compact is the value at p from v as JSON on one line, "" where p is empty or
// the value is not there.
func compact(v any, p string) string {
	if p == "" {
		return ""
	}
	got, ok := pointer(v, p)
	if !ok {
		return ""
	}
	return ordered.JSON(got, -1)
}

// readSARIF reads the results of a SARIF log: the rule id is the key, the
// level the severity, and the first location the place.
func readSARIF(out []byte) ([]model.Diagnostic, error) {
	doc, err := document(out)
	if err != nil {
		return nil, err
	}
	runs, ok := pointer(doc, "/runs")
	list, isList := runs.([]any)
	if !ok || !isList {
		return nil, printed("a SARIF log holds a list of runs, and /runs is not one")
	}
	var found []model.Diagnostic
	for ri, run := range list {
		results, ok := pointer(run, "/results")
		if !ok || results == nil {
			continue
		}
		rl, isList := results.([]any)
		if !isList {
			return nil, printed("run %d: results is not a list", ri+1)
		}
		for i, r := range rl {
			if len(found) >= MaxFindings {
				return nil, printed("more than %d findings", MaxFindings)
			}
			d := model.Diagnostic{}
			d.Key = word(r, "/ruleId")
			if d.Key == "" {
				return nil, printed("run %d result %d has no ruleId", ri+1, i+1)
			}
			d.Message = text(r, "/message/text")
			d.Severity = text(r, "/level")
			d.Path = text(r, "/locations/0/physicalLocation/artifactLocation/uri")
			d.Line = number(r, "/locations/0/physicalLocation/region/startLine")
			d.Column = number(r, "/locations/0/physicalLocation/region/startColumn")
			found = append(found, d)
		}
	}
	return found, nil
}

// number is the value at p as a non-negative whole number, 0 where it is
// not one.
func number(v any, p string) int {
	got, ok := pointer(v, p)
	if !ok {
		return 0
	}
	switch n := got.(type) {
	case int64:
		if n > 0 && n < 1<<31 {
			return int(n)
		}
	case string:
		if i, err := strconv.Atoi(n); err == nil && i > 0 && i <= math.MaxInt32 {
			return i
		}
	}
	return 0
}

// readJSON reads the diagnostics of a JSON document by the declared pointers.
func readJSON(o model.AdapterOutput, out []byte) ([]model.Diagnostic, error) {
	if o.Fields == nil {
		return nil, printed("the adapter names no fields for a JSON output")
	}
	doc, err := document(out)
	if err != nil {
		return nil, err
	}
	at, ok := pointer(doc, o.Each)
	if !ok {
		return nil, printed("%q is not in the printed document", o.Each)
	}
	list, isList := at.([]any)
	if !isList {
		return nil, printed("%q is not a list of findings", o.Each)
	}
	if len(list) > MaxFindings {
		return nil, printed("more than %d findings", MaxFindings)
	}
	found := make([]model.Diagnostic, 0, len(list))
	for i, r := range list {
		f := o.Fields
		d := model.Diagnostic{Line: number(r, f.Line), Column: number(r, f.Column)}
		d.Key = word(r, f.Key)
		if d.Key == "" {
			return nil, printed("finding %d has no key at %q", i+1, f.Key)
		}
		d.Message = text(r, f.Message)
		d.Path = text(r, f.Path)
		d.Severity = text(r, f.Severity)
		d.URL = text(r, f.URL)
		found = append(found, d)
	}
	return found, nil
}

// readLines reads the diagnostics of plain lines: each line the pattern
// matches is one, its named groups the fields. A line it does not match is
// not one.
func readLines(o model.AdapterOutput, out []byte) ([]model.Diagnostic, error) {
	re, err := regexp.Compile(o.Pattern)
	if err != nil || o.Pattern == "" {
		return nil, printed("the adapter's pattern does not compile")
	}
	names := re.SubexpNames()
	var found []model.Diagnostic
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if len(line) > maxLine {
			continue
		}
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if len(found) >= MaxFindings {
			return nil, printed("more than %d findings", MaxFindings)
		}
		d := model.Diagnostic{}
		for i, n := range names {
			switch model.Field(n) {
			case model.FieldKey:
				d.Key = m[i]
			case model.FieldMessage:
				d.Message = m[i]
			case model.FieldPath:
				d.Path = m[i]
			case model.FieldLine:
				d.Line = whole(m[i])
			case model.FieldColumn:
				d.Column = whole(m[i])
			case model.FieldSeverity:
				d.Severity = m[i]
			case model.FieldURL:
				d.URL = m[i]
			}
		}
		if d.Key == "" {
			return nil, printed("a line the pattern matches has no key: %q", truncate(line))
		}
		found = append(found, d)
	}
	return found, nil
}

func whole(s string) int {
	i, err := strconv.Atoi(s)
	if err != nil || i < 0 || i > math.MaxInt32 {
		return 0
	}
	return i
}

func truncate(s string) string {
	if r := []rune(s); len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}
