package canon

import (
	"encoding/json/jsontext"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Layout is how Encode lays a value out. Every layout is canonical: one value
// has one encoding in each, and the three differ only in white space outside
// strings.
type Layout int

const (
	// Document lays a value out for a file a person may read: two spaces of
	// indent per level, one member or element per line, a key written as
	// "key": value, an empty object or array as {} or [], LF line ends and
	// one LF at the end. Every document gragas writes uses it.
	Document Layout = iota
	// Line is Canonical with one LF at the end, for a JSON Lines file such
	// as the history.
	Line
	// Canonical is RFC 8785's bytes exactly: no white space outside strings
	// and no LF at the end. A digest is of these bytes alone.
	Canonical
)

// Encode is v encoded in the given layout, with the LF at the end that
// Document and Line have. The bytes are jsontext's: canon checks v against its
// own limits, hands the checked tree to jsontext.Value.Canonicalize, which
// sorts the keys, writes the numbers and escapes the strings as RFC 8785 says,
// and for Document has jsontext indent the result. canon writes none of that
// itself.
//
// It refuses a nil Value with NilValue, a String or key that is not UTF-8 with
// BadUTF8, a Float that is not finite with NotFinite, an Int or Float past
// MaxSafe with TooLarge, a value nested deeper than MaxDepth with TooDeep, a
// type outside canon's set (one that implements Value by embedding a canon
// type) with Misuse, and a layout that is not one of the three with Misuse.
// The *Error's Path names where.
func Encode(v Value, layout Layout) ([]byte, error) {
	if layout != Document && layout != Line && layout != Canonical {
		return nil, &Error{Kind: Misuse, Offset: -1, Detail: "layout " + strconv.Itoa(int(layout)) + " is not one of Document, Line and Canonical"}
	}
	c := checker{}
	if err := c.value(v, 0); err != nil {
		return nil, err
	}
	b := jsontext.Value(c.buf)
	if err := b.Canonicalize(); err != nil {
		return nil, fmt.Errorf("canon: canonicalize: %w", err)
	}
	if layout == Document {
		if err := b.Indent(jsontext.WithIndent("  ")); err != nil {
			return nil, fmt.Errorf("canon: indent: %w", err)
		}
	}
	out := []byte(b)
	if layout != Canonical {
		out = append(out, '\n')
	}
	return out, nil
}

// checker walks a value, refusing what has no canonical form, and writes it as
// JSON in no particular order or number form: jsontext puts that right.
type checker struct {
	buf  []byte
	path []string // the pointer segments of the place being checked
}

func (c *checker) fail(k Kind, detail string) error {
	var b strings.Builder
	for _, s := range c.path {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(s))
	}
	return &Error{Kind: k, Offset: -1, Path: b.String(), Detail: detail}
}

func (c *checker) value(v Value, depth int) error {
	switch x := v.(type) {
	case nil:
		return c.fail(NilValue, "a nil Value: write Null{} for null, or leave the member out")
	case Null:
		c.buf = append(c.buf, "null"...)
	case Bool:
		c.buf = strconv.AppendBool(c.buf, bool(x))
	case Int:
		if x < -MaxSafe || x > MaxSafe {
			return c.fail(TooLarge, strconv.FormatInt(int64(x), 10)+" is past 2^53 - 1, so a reader would round it")
		}
		c.buf = strconv.AppendInt(c.buf, int64(x), 10)
	case Float:
		f := float64(x)
		switch {
		case f != f || f > 1.7976931348623157e308 || f < -1.7976931348623157e308:
			return c.fail(NotFinite, strconv.FormatFloat(f, 'g', -1, 64)+" has no JSON form")
		case f > MaxSafe || f < -MaxSafe:
			return c.fail(TooLarge, strconv.FormatFloat(f, 'g', -1, 64)+" is a whole number past 2^53 - 1")
		}
		c.buf = strconv.AppendFloat(c.buf, f, 'g', -1, 64)
	case String:
		return c.text(string(x), "a string ")
	case Array:
		return c.array(x, depth)
	case Object:
		return c.object(x, depth)
	default:
		return c.fail(Misuse, fmt.Sprintf("%T is not one of canon's values", v))
	}
	return nil
}

func (c *checker) text(s, what string) error {
	if !utf8.ValidString(s) {
		return c.fail(BadUTF8, what+strconv.QuoteToASCII(s))
	}
	b, err := jsontext.AppendQuote(c.buf, s)
	if err != nil {
		return c.fail(BadUTF8, what+strconv.QuoteToASCII(s))
	}
	c.buf = b
	return nil
}

func (c *checker) array(a Array, depth int) error {
	if depth >= MaxDepth {
		return c.fail(TooDeep, "more than "+strconv.Itoa(MaxDepth)+" levels")
	}
	c.buf = append(c.buf, '[')
	for i, v := range a {
		if i > 0 {
			c.buf = append(c.buf, ',')
		}
		c.path = append(c.path, strconv.Itoa(i))
		if err := c.value(v, depth+1); err != nil {
			return err
		}
		c.path = c.path[:len(c.path)-1]
	}
	c.buf = append(c.buf, ']')
	return nil
}

func (c *checker) object(o Object, depth int) error {
	if depth >= MaxDepth {
		return c.fail(TooDeep, "more than "+strconv.Itoa(MaxDepth)+" levels")
	}
	c.buf = append(c.buf, '{')
	// Byte order, only so the first refusal is the same on every run; the
	// bytes' order is jsontext's.
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for i, k := range keys {
		if i > 0 {
			c.buf = append(c.buf, ',')
		}
		if err := c.text(k, "a key "); err != nil {
			return err
		}
		c.buf = append(c.buf, ':')
		c.path = append(c.path, k)
		if err := c.value(o[k], depth+1); err != nil {
			return err
		}
		c.path = c.path[:len(c.path)-1]
	}
	c.buf = append(c.buf, '}')
	return nil
}
