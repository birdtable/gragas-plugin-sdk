package ordered

import (
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// The YAML 1.2 core schema (1.2.2, 10.3.2), the other reading of a plain
// scalar than the 1.1 tables above. The two part on "yes", "010", "0o10",
// "1_000", "0b11", "1e3" and a date, so a value a 1.1 loader read one way and
// a 1.2 loader reads another is a place two tools disagree.
var (
	re12Bool  = regexp.MustCompile(`^(?:true|True|TRUE|false|False|FALSE)$`)
	re12Int   = regexp.MustCompile(`^[-+]?[0-9]+$`)
	re12Oct   = regexp.MustCompile(`^0o[0-7]+$`)
	re12Hex   = regexp.MustCompile(`^0x[0-9a-fA-F]+$`)
	re12Float = regexp.MustCompile(`^[-+]?(?:\.[0-9]+|[0-9]+(?:\.[0-9]*)?)(?:[eE][-+]?[0-9]+)?$`)
	re12Inf   = regexp.MustCompile(`^[-+]?\.(?:inf|Inf|INF)$`)
	re12NaN   = regexp.MustCompile(`^\.(?:nan|NaN|NAN)$`)
)

// Scalar is one scalar of a document, with the two readings a loader may give
// it. Text is the scalar as written, with quotes and escapes resolved; Line is
// where it starts. Key is a mapping key. Plain is a scalar written with no
// quotes and not as a literal or folded block: the only kind a loader resolves
// to something other than its text. Tag is "!" for a scalar written with the
// non-specific tag, and empty for the rest. ParseStrict refuses every other
// explicit tag, so Scalars never gives one; it reads a "! yes" as it reads a
// "yes", because the parser it stands on drops that tag from the node, and
// Scalars finds it in the source.
//
// Read11 is the value ParseStrict reads, by the YAML 1.1 tables: nil, bool,
// int64, float64, Timestamp or string. Read12 is the value of the YAML 1.2
// core schema: nil, bool, int64, float64 or string, where an integer too large
// for int64 is the float it rounds to and a date is its text. A quoted or
// block scalar reads as its text under both, and so does one tagged "!", which
// YAML 1.2 reads as a string whatever it spells; its Read11 is still what
// ParseStrict gives. NaN is not equal to itself, so a comparison of the two
// readings with == fails closed on .nan.
type Scalar struct {
	Text   string
	Line   int
	Key    bool
	Plain  bool
	Tag    string
	Read11 any
	Read12 any
}

// Scalars is every scalar of the one document in data, keys and values, in the
// order the document writes them, each with its two readings. It refuses with
// ParseStrict's error whatever ParseStrict refuses, so one gate decides what a
// caller sees: no alias, anchor, merge key, explicit tag, repeated key,
// non-string key or second document ever reaches it. An empty document has no
// scalars.
func Scalars(data []byte) ([]Scalar, error) {
	v, err := ParseStrict(data)
	if v == nil || err != nil {
		return nil, err
	}
	n, err := decodeOne(data)
	if n == nil || err != nil {
		return nil, err
	}
	w := scalarWalk{lines: sourceLines(string(data))}
	if err := w.walk(n, false); err != nil {
		return nil, err
	}
	return w.out, nil
}

// scalarWalk collects the scalars below a node; lines is the source, by line.
type scalarWalk struct {
	lines [][]rune
	out   []Scalar
}

// walk appends the scalars below n. A node of a mapping at an even place is a
// key.
func (w *scalarWalk) walk(n *yaml.Node, key bool) error {
	switch n.Kind {
	case yaml.ScalarNode:
		r11, err := scalar(n)
		if err != nil {
			return err
		}
		plain := n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) == 0
		tag := ""
		if n.Style&yaml.TaggedStyle != 0 {
			tag = n.Tag
		} else if w.startsWithBang(n) {
			tag = "!"
		}
		var r12 any = n.Value
		if plain && tag == "" {
			r12 = resolve12(n.Value)
		}
		w.out = append(w.out, Scalar{Text: n.Value, Line: n.Line, Key: key, Plain: plain, Tag: tag, Read11: r11, Read12: r12})
	case yaml.MappingNode:
		for i, c := range n.Content {
			if err := w.walk(c, i%2 == 0); err != nil {
				return err
			}
		}
	case yaml.AliasNode:
		return nil // ParseStrict has refused every alias
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, c := range n.Content {
			if err := w.walk(c, false); err != nil {
				return err
			}
		}
	}
	return nil
}

// startsWithBang is whether the source at n's position is a "!": the
// non-specific tag, which the node does not keep. A plain scalar cannot begin
// with "!", and a quoted or block one begins with its quote or indicator, so
// the character there is a tag.
func (w *scalarWalk) startsWithBang(n *yaml.Node) bool {
	if n.Line < 1 || n.Line > len(w.lines) {
		return false
	}
	row := w.lines[n.Line-1]
	return n.Column >= 1 && n.Column <= len(row) && row[n.Column-1] == '!'
}

// sourceLines is s by line, split where YAML breaks a line: \n, \r, \r\n and
// the Unicode breaks NEL, LS and PS, with a leading byte order mark dropped
// as the parser drops it.
func sourceLines(s string) [][]rune {
	rs := []rune(s)
	if len(rs) > 0 && rs[0] == rune(0xFEFF) {
		rs = rs[1:]
	}
	var lines [][]rune
	start := 0
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\r', '\n', rune(0x85), rune(0x2028), rune(0x2029):
			lines = append(lines, rs[start:i])
			if rs[i] == '\r' && i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	return append(lines, rs[start:])
}

// resolve12 is a plain scalar's value by the YAML 1.2 core schema.
func resolve12(s string) any {
	switch {
	case reNull.MatchString(s):
		return nil
	case re12Bool.MatchString(s):
		return s[0] == 't' || s[0] == 'T'
	case re12Int.MatchString(s):
		return integer12(s, 10)
	case re12Oct.MatchString(s):
		return integer12(s[2:], 8)
	case re12Hex.MatchString(s):
		return integer12(s[2:], 16)
	case re12Inf.MatchString(s):
		if s[0] == '-' {
			return math.Inf(-1)
		}
		return math.Inf(1)
	case re12NaN.MatchString(s):
		return math.NaN()
	case re12Float.MatchString(s):
		f, err := strconv.ParseFloat(s, 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return s
		}
		return f
	default:
		return s
	}
}

// integer12 is digits in base as an int64, or the float it rounds to when it
// does not fit.
func integer12(digits string, base int) any {
	if v, err := strconv.ParseInt(digits, base, 64); err == nil {
		return v
	}
	if base == 10 {
		if f, err := strconv.ParseFloat(digits, 64); err == nil || errors.Is(err, strconv.ErrRange) {
			return f
		}
		return digits
	}
	f := 0.0
	for _, c := range digits {
		d := float64(strings.IndexRune("0123456789abcdef", unicode.ToLower(c)))
		f = f*float64(base) + d
	}
	return f
}
