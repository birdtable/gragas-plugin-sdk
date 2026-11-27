package plugin

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/gragas-plugin-sdk/pointer"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// This file is the answer's side: each operation's answer is made ready for the wire. It is sorted by the order
// rules of the plugin contract, so the engine, which refuses an answer out of order and never sorts it, never meets
// one from this SDK; and an answer the engine would refuse for what the author wrote (a key given twice, a
// word the contract does not have, a mark missing) is a bug that says so here, in the plugin's own test, and
// not as ErrAnswer in a project.

// maxSteps is the most steps a plan may give.
const maxSteps = 16

// finishPlan checks a plan: at most 16 steps, each a run or a read, argv words the engine will take, and either
// steps or the last round. Steps run in the order given, so they are not sorted.
func finishPlan(_ *wire.PlanRequest, r *wire.PlanResponse) *wire.Error {
	if len(r.Steps) > maxSteps {
		return bug("/steps", "a plan gives %d steps, and 16 are the most", len(r.Steps))
	}
	if len(r.Steps) == 0 && !r.Final {
		return bug("/steps", "a plan with no steps must be final: the engine refuses one that is neither")
	}
	if r.Steps == nil {
		r.Steps = []wire.Step{}
	}
	for i, s := range r.Steps {
		at := fmt.Sprintf("/steps/%d", i)
		switch {
		case s.Run != nil && s.Read != nil, s.Run == nil && s.Read == nil:
			return bug(at, "a step is a run or a read, one of the two")
		case s.Run != nil:
			if e := checkRun(at, s.Run); e != nil {
				return e
			}
		case s.Read.Glob == "" || len(s.Read.Glob) > 2000:
			return bug(at+"/glob", "a read names a glob of 1 to 2000 bytes")
		}
	}
	return nil
}

// wordFault is why w is not a word of an argv, by the engine's own rule: it is literal, except the word {scratch}, the
// word {null} and a word that starts with {scratch}/; any other { is refused. So --out={scratch}/a is not a word.
func wordFault(w string) string {
	rest := w
	switch {
	case w == "{scratch}" || w == "{null}":
		return ""
	case strings.HasPrefix(w, "{scratch}/"):
		rest = strings.TrimPrefix(w, "{scratch}/")
	}
	if strings.Contains(rest, "{") {
		return "has a { that is not the word {scratch} or {null}, or after a leading {scratch}/, and the engine refuses it"
	}
	return ""
}

// checkRun checks a run step for what an author gets wrong: a program with no name, a word with a brace the
// engine reads as a token it does not have, and a level the contract does not name.
func checkRun(at string, r *wire.Run) *wire.Error {
	switch {
	case r.Program == "" || len(r.Program) > 64:
		return bug(at+"/program", "a run names a program of 1 to 64 bytes")
	case r.Level != "" && !slices.Contains(wire.Levels(), r.Level):
		return bug(at+"/level", "the level %q is not isolated or sandboxed", string(r.Level))
	}
	if len(r.Argv) > 300 {
		return bug(at+"/argv", "a run has %d argv words, and 300 are the most", len(r.Argv))
	}
	for i, w := range r.Argv {
		if why := wordFault(w); why != "" {
			return bug(fmt.Sprintf("%s/argv/%d", at, i), "the argv word %q %s", w, why)
		}
	}
	if len(r.Place) > 20 || len(r.Collect) > 20 {
		return bug(at, "a run places %d files and collects %d, and 20 each are the most", len(r.Place), len(r.Collect))
	}
	if r.Argv == nil {
		r.Argv = []string{}
	}
	return nil
}

// The caps of an opaque value: 32 levels of arrays and objects, and 64 KiB.
const (
	maxValueDepth = 32
	maxValueBytes = 64 << 10
)

// valueFault is why the opaque value v is over its caps, "" when it is not or is none. The size is of the value in
// compact form, which is the canonical form's size but for the spelling of numbers.
func valueFault(v wire.Value) string {
	if len(bytes.TrimSpace(v)) == 0 {
		return ""
	}
	depth, deepest := 0, 0
	inString, escaped := false, false
	for _, c := range v {
		switch {
		case inString && escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case inString && c == '"':
			inString = false
		case inString:
		case c == '"':
			inString = true
		case c == '[' || c == '{':
			depth++
			deepest = max(deepest, depth)
		case c == ']' || c == '}':
			depth--
		}
	}
	if deepest > maxValueDepth {
		return fmt.Sprintf("nests %d levels, more than %d", deepest, maxValueDepth)
	}
	var b bytes.Buffer
	if json.Compact(&b, v) == nil && b.Len() > maxValueBytes {
		return fmt.Sprintf("is %d bytes, more than %d", b.Len(), maxValueBytes)
	}
	return ""
}

// finishEntries sorts the entries by key as UTF-8 bytes, each key once, and checks what the schema requires.
func finishEntries(_ *wire.EntriesRequest, r *wire.EntriesResponse) *wire.Error {
	if r.Entries == nil {
		r.Entries = []wire.Entry{}
	}
	for i, e := range r.Entries {
		at := fmt.Sprintf("/entries/%d", i)
		if e.Key == "" || len(e.Key) > 512 {
			return bug(at+"/key", "an entry's key is 1 to 512 bytes")
		}
		if e.Severity != "" && !slices.Contains(wire.Severities(), e.Severity) {
			return bug(at+"/severity", "the severity %q is not error, warning or info", string(e.Severity))
		}
		if why := valueFault(e.Value); why != "" {
			return bug(at+"/value", "the value %s, and the engine refuses it", why)
		}
		if why := valueFault(e.Options); why != "" {
			return bug(at+"/options", "the options %s, and the engine refuses them", why)
		}
	}
	slices.SortStableFunc(r.Entries, func(a, b wire.Entry) int { return strings.Compare(a.Key, b.Key) })
	for i := 1; i < len(r.Entries); i++ {
		if r.Entries[i].Key == r.Entries[i-1].Key {
			return bug("/entries", "the key %q is given twice: each key stands once", r.Entries[i].Key)
		}
	}
	return nil
}

// finishFindings sorts the findings by path, line, column, key and message, an absent field first.
func finishFindings(_ *wire.FindingsRequest, r *wire.FindingsResponse) *wire.Error {
	if r.Findings == nil {
		r.Findings = []wire.Finding{}
	}
	for i, f := range r.Findings {
		at := fmt.Sprintf("/findings/%d", i)
		switch {
		case f.Key == "" || len(f.Key) > 512:
			return bug(at+"/key", "a finding's key is 1 to 512 bytes")
		case f.Message == "":
			return bug(at+"/message", "a finding has a message")
		case f.Line < 0 || f.Column < 0 || int64(f.Line) > math.MaxUint32 || int64(f.Column) > math.MaxUint32:
			return bug(at, "a finding's line and column count from 1 to 4294967295; 0 leaves them out")
		case f.Severity != "" && !slices.Contains(wire.Severities(), f.Severity):
			return bug(at+"/severity", "the severity %q is not error, warning or info", string(f.Severity))
		}
	}
	// The display-only fields are cut at the schema's lengths, on a character, so a long line of a tool's output is not a bug.
	r.Why = clip(r.Why, 2048, "")
	for i := range r.Findings {
		r.Findings[i].Message = clip(r.Findings[i].Message, 8192, "")
		r.Findings[i].URL = clip(r.Findings[i].URL, 2048, "")
	}
	// "" and 0 are an absent path, line and column: the least of their kinds, so they sort first.
	slices.SortStableFunc(r.Findings, func(a, b wire.Finding) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Column, b.Column),
			strings.Compare(a.Key, b.Key), strings.Compare(a.Message, b.Message))
	})
	return nil
}

// finishRank checks that the words answer the pairs by position and equal them in number. They are not sorted:
// the engine reads them by position.
func finishRank(req *wire.RankRequest, r *wire.RankResponse) *wire.Error {
	if len(r.Words) != len(req.Pairs) {
		return bug("/words", "rank gives %d words for %d pairs: the words answer the pairs by position and equal them in number", len(r.Words), len(req.Pairs))
	}
	for i, w := range r.Words {
		if !slices.Contains(wire.Words(), w) {
			return bug(fmt.Sprintf("/words/%d", i), "the word %q is not tighter, weaker, same or unknown", string(w))
		}
	}
	return nil
}

// finishPreset sorts the files by path, each once, and holds each to the mark when the request names one: the
// mark stands in the first line of every preset file.
func finishPreset(req *wire.PresetRequest, r *wire.PresetResponse) *wire.Error {
	if len(r.Files) == 0 {
		return bug("/files", "a preset writes at least one file")
	}
	for i, f := range r.Files {
		at := fmt.Sprintf("/files/%d", i)
		if f.Path == "" || len(f.Path) > 4096 {
			return bug(at+"/path", "a preset file's path is 1 to 4096 bytes")
		}
		if req.Mark != "" {
			first, _, _ := strings.Cut(f.Text, "\n")
			if !strings.Contains(first, req.Mark) {
				return bug(at+"/text", "the file %q does not hold the mark %q in its first line", f.Path, req.Mark)
			}
		}
	}
	for i := range r.Notes {
		r.Notes[i] = clip(r.Notes[i], 1024, "")
	}
	slices.SortStableFunc(r.Files, func(a, b wire.PresetFile) int { return strings.Compare(a.Path, b.Path) })
	for i := 1; i < len(r.Files); i++ {
		if r.Files[i].Path == r.Files[i-1].Path {
			return bug("/files", "the path %q is given twice: each file stands once", r.Files[i].Path)
		}
	}
	return nil
}

// utf8Fault is the pointer of the first string of an answer that is not valid UTF-8, "" when every one is. encoding/json
// writes U+FFFD for each bad byte and says nothing, so a key, a path or a preset's text that came from a tool's raw bytes
// would leave as another string with ok. A wire.Bytes holds bytes and writes them as base64 when they are not text, and
// a Value is JSON the plugin wrote: neither is walked.
func utf8Fault(v any) (string, bool) {
	return walkUTF8(reflect.ValueOf(v), nil)
}

var (
	bytesType = reflect.TypeFor[wire.Bytes]()
	rawType   = reflect.TypeFor[json.RawMessage]()
)

func walkUTF8(v reflect.Value, at []string) (string, bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return "", false
		}
		return walkUTF8(v.Elem(), at)
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return pointer.Join(at...), true
		}
	case reflect.Slice:
		if v.Type() == rawType {
			return "", false
		}
		for i := range v.Len() {
			if p, bad := walkUTF8(v.Index(i), append(at, pointer.Index(i))); bad {
				return p, true
			}
		}
	case reflect.Struct:
		if v.Type() == bytesType {
			return "", false
		}
		for i := range v.NumField() {
			f := v.Type().Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if !f.IsExported() || name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			if p, bad := walkUTF8(v.Field(i), append(at, name)); bad {
				return p, true
			}
		}
	}
	return "", false
}
