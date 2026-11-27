// Package ordered holds YAML documents as values that keep their key order and
// source lines, and encodes them as JSON the way the rulebook's outputs were
// first written: key order kept, one-space indent, non-ASCII left as is.
//
// A value is one of: nil, bool, int64, float64, string, Timestamp, []any or *Map.
//
// # Line positions
//
// Render (M1) edits a person-written rule file only in the lines it owns, and
// finds them through the lines this package records. Every mapping knows the
// 1-based source line it starts on (Map.Line), and every key the line it is
// written on (Map.KeyLine); a key a value was never read for has line 0. A
// key written twice keeps its first place and the line of its last write.
// The lines count every "\n" of the bytes ParseYAML was given. A gragas block's
// YAML starts at markdown.Block.BodyLine, so line n of the parsed body is line
// BodyLine+n-1 of the guide. LineAt reads the line of a path of keys and list
// indexes. ParseYAML records no column and no byte offset: a caller that needs
// the bytes of a line reads them from its own source.
package ordered

import "math"

// Map is a mapping that keeps the order its keys were first written in.
// A key written twice keeps its first position and takes its last value,
// and Repeats says where, for a reader that refuses it.
type Map struct {
	// Line is the 1-based source line of the mapping, 0 when unknown.
	Line     int
	keys     []string
	vals     map[string]any
	keyLines map[string]int
	repeats  []Repeat
}

// Repeat is a key written twice in one mapping of a YAML document: the
// line it was written on before (First), and the line it was written again
// on (Again).
type Repeat struct {
	Key          string
	First, Again int
}

// Repeats is every key the YAML document wrote twice in this mapping, in
// the order they were written again. The caller must not change the slice.
func (m *Map) Repeats() []Repeat { return m.repeats }

// Timestamp is a plain YAML scalar that YAML 1.1 reads as a date or time.
// It is kept apart from strings so that a schema expecting a string refuses it.
type Timestamp string

// NewMap returns an empty map that starts at the given source line.
func NewMap(line int) *Map {
	return &Map{Line: line, vals: map[string]any{}, keyLines: map[string]int{}}
}

// Set stores v under k; line is the key's source line.
func (m *Map) Set(k string, v any, line int) {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
	m.keyLines[k] = line
}

// Keys returns the keys in order. The caller must not change the slice.
func (m *Map) Keys() []string { return m.keys }

// Len is the number of keys.
func (m *Map) Len() int { return len(m.keys) }

// Get returns the value under k and whether k is present.
func (m *Map) Get(k string) (any, bool) {
	v, ok := m.vals[k]
	return v, ok
}

// KeyLine is the source line of key k, 0 when unknown.
func (m *Map) KeyLine(k string) int { return m.keyLines[k] }

// Str returns the string under k, or "" when k is absent or not a string.
func (m *Map) Str(k string) string {
	if s, ok := m.vals[k].(string); ok {
		return s
	}
	return ""
}

// List returns the list under k, or nil when k is absent or not a list.
func (m *Map) List(k string) []any {
	if l, ok := m.vals[k].([]any); ok {
		return l
	}
	return nil
}

// Sub returns the mapping under k, or nil when k is absent or not a mapping.
func (m *Map) Sub(k string) *Map {
	if s, ok := m.vals[k].(*Map); ok {
		return s
	}
	return nil
}

// Truthy reports whether the value under k is present and not empty,
// as an optional field is tested in the rule data: "" , [], {} and nil are empty.
func (m *Map) Truthy(k string) bool {
	switch v := m.vals[k].(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case *Map:
		return v.Len() > 0
	case bool:
		return v
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return true
	}
}

// Maps returns the mappings in a list, skipping items that are not mappings.
func Maps(l []any) []*Map {
	out := make([]*Map, 0, len(l))
	for _, v := range l {
		if m, ok := v.(*Map); ok {
			out = append(out, m)
		}
	}
	return out
}

// Strings returns the strings in a list, skipping items that are not strings.
func Strings(l []any) []string {
	out := make([]string, 0, len(l))
	for _, v := range l {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// LineAt returns the source line of the value at path below v: the deepest
// line known on the way down, 0 when none is known.
func LineAt(v any, path []string) int {
	line := 0
	if m, ok := v.(*Map); ok {
		line = m.Line
	}
	for _, seg := range path {
		switch cur := v.(type) {
		case *Map:
			next, ok := cur.Get(seg)
			if !ok {
				return line
			}
			if l := cur.KeyLine(seg); l > 0 {
				line = l
			}
			v = next
		case []any:
			i, ok := index(seg, len(cur))
			if !ok {
				return line
			}
			v = cur[i]
			if m, ok := v.(*Map); ok && m.Line > 0 {
				line = m.Line
			}
		default:
			return line
		}
	}
	return line
}

func index(seg string, n int) (int, bool) {
	i := 0
	if seg == "" {
		return 0, false
	}
	for _, r := range seg {
		if r < '0' || r > '9' {
			return 0, false
		}
		i = i*10 + int(r-'0')
		if i >= n {
			return 0, false
		}
	}
	return i, true
}

// Plain converts v to the plain Go values a JSON Schema validator reads:
// map[string]any, []any, string, bool, int64, float64 and nil. A value with no
// JSON form (a Timestamp, NaN or an infinity) becomes a one-key object
// {"$yaml": text}, which any string, number or enum schema refuses.
func Plain(v any) any {
	switch x := v.(type) {
	case *Map:
		out := make(map[string]any, x.Len())
		for _, k := range x.keys {
			out[k] = Plain(x.vals[k])
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = Plain(e)
		}
		return out
	case Timestamp:
		return map[string]any{"$yaml": string(x)}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return map[string]any{"$yaml": FloatRepr(x)}
		}
		return x
	default:
		return v
	}
}

// Val returns the value under k, or nil when k is absent.
func (m *Map) Val(k string) any { return m.vals[k] }

// ValOr returns the value under k, or d when k is absent.
func (m *Map) ValOr(k string, d any) any {
	if v, ok := m.vals[k]; ok {
		return v
	}
	return d
}
