package ordered

import (
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The rule data was first read by a YAML 1.1 loader, so a plain scalar is
// resolved by the YAML 1.1 tables, not YAML 1.2's: "yes" and "off" are booleans,
// "010" is octal and "2026-01-02" is a date.
var (
	reNull  = regexp.MustCompile(`^(?:~|null|Null|NULL|)$`)
	reBool  = regexp.MustCompile(`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$`)
	reInt   = regexp.MustCompile(`^(?:[-+]?0b[0-1_]+|[-+]?0[0-7_]+|[-+]?(?:0|[1-9][0-9_]*)|[-+]?0x[0-9a-fA-F_]+|[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+)$`)
	reFloat = regexp.MustCompile(`^(?:[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+][0-9]+)?|\.[0-9_]+(?:[eE][-+][0-9]+)?|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`)
	reTime  = regexp.MustCompile(`^(?:[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]|[0-9][0-9][0-9][0-9]-[0-9][0-9]?-[0-9][0-9]?(?:[Tt]|[ \t]+)[0-9][0-9]?:[0-9][0-9]:[0-9][0-9](?:\.[0-9]*)?(?:[ \t]*(?:Z|[-+][0-9][0-9]?(?::[0-9][0-9])?))?)$`)
)

// maxDepth bounds nesting and alias chains, so a cycle fails cleanly.
const maxDepth = 200

// aliasBudget is how much the aliases of a document may expand to, for each
// byte of it, and aliasFloor the least they may expand to whatever its size:
// a node costs 1, and a scalar 1 and its length, as resolve reads the whole
// of it again for each alias. Each alias is read again in full where it
// stands, so ten aliases of ten aliases grow a document tenfold a level: a
// few hundred bytes would expand to billions of nodes, for hours, and a long
// plain scalar read through many aliases costs its length each time.
// A budget linear in the document keeps what a parse allocates and the time
// it takes a fixed multiple of its length. Six a byte reads a block that is
// most of a document through several aliases, and the floor reads a small
// document through many.
const (
	aliasBudget = 6
	aliasFloor  = 64 << 10
)

// ErrSyntax marks a document that is not readable YAML.
var ErrSyntax = errors.New("yaml")

// SyntaxError is a document that is not readable YAML: Detail says where and
// why, with no prefix, for a person. It matches ErrSyntax.
type SyntaxError struct {
	Detail string
	// Line is the line the refusal is at, 0 when it is not known apart from
	// Detail, as for the parser's own errors.
	Line int
	err  error
}

func (e *SyntaxError) Error() string { return ErrSyntax.Error() + ": " + e.Detail }

// Is makes a SyntaxError match ErrSyntax.
func (e *SyntaxError) Is(target error) bool { return target == ErrSyntax }

// Unwrap is the parser's own error under it, if any.
func (e *SyntaxError) Unwrap() error { return e.err }

// syntax is a SyntaxError, its detail written as fmt.Errorf writes it.
func syntax(format string, args ...any) error {
	inner := fmt.Errorf(format, args...)
	return &SyntaxError{Detail: inner.Error(), err: errors.Unwrap(inner)}
}

// syntaxAt is a SyntaxError at line, its detail starting "line <line>: ".
func syntaxAt(line int, format string, args ...any) error {
	inner := fmt.Errorf("line %d: "+format, append([]any{line}, args...)...)
	return &SyntaxError{Detail: inner.Error(), Line: line, err: errors.Unwrap(inner)}
}

// ParseYAML reads one YAML document. An empty document is nil.
func ParseYAML(data []byte) (any, error) {
	n, err := decodeOne(data)
	if n == nil || err != nil {
		return nil, err
	}
	c := converter{budget: max(aliasFloor, aliasBudget*len(data))}
	c.left = c.budget
	return c.convert(n, 0)
}

// ParseTextKeys reads one YAML document as GitHub reads a workflow: it is
// ParseYAML with every untagged scalar key taken as its text, so the key on
// is the name of a workflow's triggers and not YAML 1.1's true. Values are
// read as ParseYAML reads them.
func ParseTextKeys(data []byte) (any, error) {
	n, err := decodeOne(data)
	if n == nil || err != nil {
		return nil, err
	}
	c := converter{budget: max(aliasFloor, aliasBudget*len(data)), textKeys: true}
	c.left = c.budget
	return c.convert(n, 0)
}

// decodeOne is the root node of the one YAML document in data, nil for an
// empty document. It refuses what is not UTF-8 and any byte order mark but one
// at the start, so the source a caller reads beside the nodes is the source the
// parser read.
func decodeOne(data []byte) (*yaml.Node, error) {
	if err := utf8Only(data); err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, syntax("%w", err)
	}
	var more yaml.Node
	if err := dec.Decode(&more); !errors.Is(err, io.EOF) {
		return nil, syntax("expected a single document, found another after line %d", doc.Line)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	return doc.Content[0], nil
}

// converter turns a document's nodes into values. budget is how much its
// aliases may expand to, and left how much of it remains; alias is the
// line of the alias being expanded, 0 outside every alias. textKeys takes
// an untagged scalar key as its text.
type converter struct {
	budget, left, alias int
	textKeys            bool
	// core12 reads an untagged plain scalar by YAML 1.2's core schema.
	core12 bool
}

func (c *converter) convert(n *yaml.Node, depth int) (any, error) {
	if depth > maxDepth {
		return nil, syntax("line %d: nesting or aliases deeper than %d", n.Line, maxDepth)
	}
	if c.alias != 0 {
		cost := 1
		if n.Kind == yaml.ScalarNode {
			cost += len(n.Value)
		}
		if cost > c.left {
			return nil, syntax("line %d: aliases expand to more than %d nodes and scalar bytes, the larger of %d for each byte of the document and %d", c.alias, c.budget, aliasBudget, aliasFloor)
		}
		c.left -= cost
	}
	switch n.Kind {
	case yaml.ScalarNode:
		if c.core12 && n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle|yaml.TaggedStyle) == 0 {
			return resolve12(n.Value), nil
		}
		return scalar(n)
	case yaml.SequenceNode:
		return c.convertSeq(n, depth)
	case yaml.MappingNode:
		return c.convertMap(n, depth)
	case yaml.AliasNode:
		outer := c.alias
		if outer == 0 {
			c.alias = n.Line
		}
		v, err := c.convert(n.Alias, depth+1)
		c.alias = outer
		return v, err
	case yaml.DocumentNode:
		if len(n.Content) == 0 {
			return nil, nil
		}
		return c.convert(n.Content[0], depth+1)
	default:
		return nil, syntax("line %d: unknown node kind %d", n.Line, n.Kind)
	}
}

// convertSeq converts a sequence node to a list.
func (c *converter) convertSeq(n *yaml.Node, depth int) (any, error) {
	out := make([]any, 0, len(n.Content))
	for _, child := range n.Content {
		v, err := c.convert(child, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// convertMap converts a mapping node to a Map, keeping key order and lines.
func (c *converter) convertMap(n *yaml.Node, depth int) (any, error) {
	m := NewMap(n.Line)
	for i := 0; i+1 < len(n.Content); i += 2 {
		kn, vn := n.Content[i], n.Content[i+1]
		if kn.Tag == "!!merge" {
			return nil, syntax("line %d: merge keys (<<) are not supported; write the keys out", kn.Line)
		}
		k, err := c.key(kn, depth)
		if err != nil {
			return nil, err
		}
		ks, ok := k.(string)
		if !ok {
			return nil, syntax("line %d: a mapping key must be a string, got %s", kn.Line, Repr(k))
		}
		v, err := c.convert(vn, depth+1)
		if err != nil {
			return nil, err
		}
		if first, again := m.keyLines[ks]; again {
			m.repeats = append(m.repeats, Repeat{Key: ks, First: first, Again: kn.Line})
		}
		m.Set(ks, v, kn.Line)
	}
	return m, nil
}

// key converts a mapping's key node.
func (c *converter) key(kn *yaml.Node, depth int) (any, error) {
	if c.textKeys && kn.Kind == yaml.ScalarNode && kn.Style&yaml.TaggedStyle == 0 {
		return kn.Value, nil
	}
	return c.convert(kn, depth+1)
}

func scalar(n *yaml.Node) (any, error) {
	quoted := n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0
	if n.Style&yaml.TaggedStyle != 0 {
		return tagged(n)
	}
	if quoted {
		return n.Value, nil
	}
	return resolve(n.Value, n.Line)
}

func tagged(n *yaml.Node) (any, error) {
	switch n.Tag {
	case "!!str":
		return n.Value, nil
	case "!!null":
		return nil, nil
	case "!!bool", "!!int", "!!float", "!!timestamp":
		v, err := resolve(n.Value, n.Line)
		if err != nil {
			return nil, err
		}
		return v, nil
	default:
		return nil, syntax("line %d: tag %s is not supported", n.Line, n.Tag)
	}
}

func resolve(s string, line int) (any, error) {
	switch {
	case reNull.MatchString(s):
		return nil, nil
	case reBool.MatchString(s):
		switch s[0] {
		case 'y', 'Y', 't', 'T':
			return true, nil
		case 'o', 'O':
			return s == "on" || s == "On" || s == "ON", nil
		default:
			return false, nil
		}
	case reInt.MatchString(s):
		v, err := parseInt(s)
		if err != nil {
			return nil, syntax("line %d: integer %s: %w", line, s, err)
		}
		return v, nil
	case reFloat.MatchString(s):
		return parseFloat(s)
	case reTime.MatchString(s):
		return Timestamp(s), nil
	default:
		return s, nil
	}
}

func parseInt(s string) (int64, error) {
	s = strings.ReplaceAll(s, "_", "")
	sign := int64(1)
	if s[0] == '-' || s[0] == '+' {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	var v int64
	var err error
	switch {
	case s == "0":
		return 0, nil
	case strings.HasPrefix(s, "0b"):
		v, err = strconv.ParseInt(s[2:], 2, 64)
	case strings.HasPrefix(s, "0x"):
		v, err = strconv.ParseInt(s[2:], 16, 64)
	case s[0] == '0':
		v, err = strconv.ParseInt(s[1:], 8, 64)
	case strings.Contains(s, ":"):
		for part := range strings.SplitSeq(s, ":") {
			d, perr := strconv.ParseInt(part, 10, 64)
			if perr != nil {
				return 0, fmt.Errorf("sexagesimal part %q: %w", part, perr)
			}
			v = v*60 + d
		}
	default:
		v, err = strconv.ParseInt(s, 10, 64)
	}
	if err != nil {
		return 0, fmt.Errorf("parse: %w", err)
	}
	return sign * v, nil
}

func parseFloat(s string) (float64, error) {
	s = strings.ToLower(strings.ReplaceAll(s, "_", ""))
	sign := 1.0
	if s[0] == '-' || s[0] == '+' {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	switch {
	case s == ".inf":
		return sign * math.Inf(1), nil
	case s == ".nan":
		return math.NaN(), nil
	case strings.Contains(s, ":"):
		v := 0.0
		for part := range strings.SplitSeq(s, ":") {
			d, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return 0, syntax("float %s: %w", s, err)
			}
			v = v*60 + d
		}
		return sign * v, nil
	default:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, syntax("float %s: %w", s, err)
		}
		return sign * v, nil
	}
}

// utf8Only refuses data that is not UTF-8 (UTF-16 or UTF-32 by a byte order
// mark or by its NUL bytes, or any other bytes that are not UTF-8 text) and a
// byte order mark anywhere but once at the start. The parser decodes UTF-16 and
// keeps its line and column against the decoded text, and it misreads a second
// mark at the start, with comment lines read as content; a caller
// holding the bytes beside the nodes would find another place than the parser.
func utf8Only(data []byte) error {
	text := string(data)
	switch {
	case strings.HasPrefix(text, "\xFE\xFF"), strings.HasPrefix(text, "\xFF\xFE"), strings.HasPrefix(text, "\x00\x00\xFE\xFF"):
		return syntax("the document is UTF-16 or UTF-32; write it as UTF-8")
	case strings.IndexByte(text, 0) >= 0:
		return syntax("the document holds a NUL byte, so it is not UTF-8 text; write it as UTF-8")
	case strings.ToValidUTF8(text, "") != text:
		return syntax("the document is not valid UTF-8; write it as UTF-8")
	}
	if strings.Contains(strings.TrimPrefix(text, "\xEF\xBB\xBF"), "\xEF\xBB\xBF") {
		return syntax("a byte order mark is allowed once, at the start of the document; remove the other")
	}
	return nil
}
