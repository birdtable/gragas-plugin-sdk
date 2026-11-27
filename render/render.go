// Package render writes the files a preset hands back: YAML, JSONC and TOML, from an ordered value. A plugin's
// preset holds the tool's own settings, so what it writes is a document of a few shapes (tables, lists, strings,
// numbers, booleans), written the same way every time: keys in the order given, two spaces of indent, a double-quoted
// string wherever a plain one could be read as something else, one newline at the end. The writers are small and
// make no attempt at every feature of the formats: no anchors, no multi-line strings, no dates, no comments but a
// header. A value they cannot write is an error, never a different document.
//
// The value is a Map (ordered), a []any, a string, a bool, nil, an int or a float, or a json.Number; a plain
// map[string]any is written with its keys sorted. FromJSON reads a plugin's Value into one, keeping the order of
// its keys and the text of its numbers.
package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Member is one key of a Map with its value.
type Member struct {
	Key   string
	Value any
}

// Map is an object whose keys keep the order given. A key given twice is an error when it is written.
type Map []Member

// Option changes how a document is written.
type Option func(*options)

type options struct{ header []string }

// WithHeader puts a comment of the given lines at the head of the document, each line after the format's own
// comment mark. A preset's mark goes here: it must stand in the first line of every file.
func WithHeader(lines ...string) Option {
	return func(o *options) { o.header = append(o.header, lines...) }
}

func settings(opts []Option) options {
	var o options
	for _, f := range opts {
		f(&o)
	}
	return o
}

// comment is the header as comment lines with mark in front, and "" with no header. A line that holds a line end
// is an error: a comment is one line.
func (o options) comment(mark string) (string, error) {
	var b strings.Builder
	for _, l := range o.header {
		for _, r := range l {
			// A comment is one line. Every control character but tab ends one for some reader or is refused by it
			// (\r \n, the vertical tab and form feed, U+0085), and U+2028 and U+2029 are line ends to YAML and to
			// JavaScript's reading of a // comment. The bidirectional controls (U+202E, U+2066 to U+2069 and the rest of
			// Bidi_Control) are Cf, not Cc: they make the lines after them read in another order in an editor.
			if r != '\t' && (unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || unicode.Is(unicode.Bidi_Control, r)) {
				return "", fmt.Errorf("a header line holds the control or line-end character U+%04X: a comment is one line", r)
			}
		}
		if !utf8.ValidString(l) {
			return "", errors.New("a header line is not valid UTF-8")
		}
		b.WriteString(strings.TrimRight(mark+" "+l, " ") + "\n")
	}
	return b.String(), nil
}

// FromJSON reads a JSON document into a value the writers take, keeping the order of every object's keys and the
// text of every number. A document that is not one JSON value is an error.
func FromJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("more than one JSON value")
	}
	return v, nil
}

func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // a string, a json.Number, a bool or nil
	}
	if d == '[' {
		out := []any{}
		for dec.More() {
			v, err := readValue(dec)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		_, err = dec.Token()
		return out, err
	}
	out := Map{}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil, err
		}
		v, err := readValue(dec)
		if err != nil {
			return nil, err
		}
		out = append(out, Member{Key: k.(string), Value: v})
	}
	_, err = dec.Token()
	return out, err
}

// members is the members of a table, in order: a Map as given, a map[string]any by its sorted keys. The second
// answer is false for a value that is not a table.
func members(v any) (Map, bool, error) {
	switch t := v.(type) {
	case Map:
		seen := map[string]bool{}
		for _, m := range t {
			if seen[m.Key] {
				return nil, true, fmt.Errorf("the key %q is given twice", m.Key)
			}
			seen[m.Key] = true
		}
		return t, true, nil
	case map[string]any:
		out := make(Map, 0, len(t))
		for _, k := range slices.Sorted(mapKeys(t)) {
			out = append(out, Member{k, t[k]})
		}
		return out, true, nil
	}
	return nil, false, nil
}

func mapKeys(m map[string]any) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// items is the items of a list, and false for a value that is not one.
func items(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

// number is v as the text of a number, and false for a value that is not one. NaN and infinity are errors.
func number(v any) (text string, ok bool, err error) {
	switch t := v.(type) {
	case int:
		return strconv.Itoa(t), true, nil
	case int64:
		return strconv.FormatInt(t, 10), true, nil
	case uint64:
		return strconv.FormatUint(t, 10), true, nil
	case float64:
		if math.IsNaN(t) || math.IsInf(t, 0) {
			return "", true, errors.New("NaN and infinity are not numbers a file holds")
		}
		return strconv.FormatFloat(t, 'g', -1, 64), true, nil
	case json.Number:
		if !jsonNumber.MatchString(string(t)) {
			return "", true, fmt.Errorf("%q is not a number in JSON's form", string(t))
		}
		return string(t), true, nil
	}
	return "", false, nil
}

// jsonNumber is JSON's number form: no plus, no leading zero, a digit each side of a point, no NaN, infinity or
// underscore, which TOML and YAML 1.1 read as other numbers.
var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// quote is s as a double-quoted string that YAML, JSON and TOML all read the same: the quote, the backslash, a
// control character and the line separators of YAML escaped, everything else as it is. A string that is not
// UTF-8 is an error.
func quote(s string) (string, error) {
	if !utf8.ValidString(s) {
		return "", errors.New("a string is not valid UTF-8")
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// indentOf is n levels of two spaces.
func indentOf(n int) string { return strings.Repeat("  ", n) }
