package canon

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Decode reads b, JSON that anyone may have written, as one value. It reads
// RFC 8259 JSON with white space anywhere it allows and keys in any order,
// and refuses what two readers could read two ways or a reader could not
// hold:
//
//   - anything but one value with only white space around it, a byte order
//     mark included (BadSyntax);
//   - bytes that are not UTF-8, and an escape naming half a surrogate pair
//     (BadUTF8);
//   - an object that names one key twice (DuplicateKey);
//   - a number whose magnitude is past MaxSafe, too large for a binary64
//     included (TooLarge), and one other than zero too small for a binary64,
//     which would read as 0 (Underflow);
//   - arrays and objects nested deeper than MaxDepth (TooDeep).
//
// A number reads as RFC 8785 reads it, as the binary64 nearest to it. A whole
// one is an Int and any other a Float, whatever its spelling: 1, 1.0 and 1e0
// are all Int(1), and -0 is Int(0). An empty array or object is a non-nil
// empty Array or Object. The *Error's Offset is where in b Decode stopped.
func Decode(b []byte) (Value, error) {
	d := decoder{b: b}
	d.space()
	v, err := d.value(0)
	if err != nil {
		return nil, err
	}
	d.space()
	if d.i < len(d.b) {
		return nil, d.refuse(BadSyntax, "something after the value")
	}
	return v, nil
}

type decoder struct {
	b []byte
	i int
}

func (d *decoder) refuse(k Kind, detail string) *Error {
	return &Error{Kind: k, Offset: d.i, Detail: detail}
}

// space skips the white space JSON allows: space, tab, LF and CR.
func (d *decoder) space() {
	for d.i < len(d.b) {
		switch d.b[d.i] {
		case ' ', '\t', '\n', '\r':
			d.i++
		default:
			return
		}
	}
}

// value reads one value at nesting depth (the containers around it).
func (d *decoder) value(depth int) (Value, error) {
	if d.i >= len(d.b) {
		return nil, d.refuse(BadSyntax, "the input ends where a value was due")
	}
	switch c := d.b[d.i]; {
	case c == '{':
		return d.object(depth + 1)
	case c == '[':
		return d.array(depth + 1)
	case c == '"':
		s, err := d.text()
		return String(s), err
	case c == '-' || (c >= '0' && c <= '9'):
		return d.number()
	case c == 't':
		return Bool(true), d.word("true")
	case c == 'f':
		return Bool(false), d.word("false")
	case c == 'n':
		return Null{}, d.word("null")
	default:
		return nil, d.refuse(BadSyntax, "a value cannot start with "+strconv.QuoteRune(rune(c)))
	}
}

func (d *decoder) word(w string) error {
	if len(d.b)-d.i < len(w) || string(d.b[d.i:d.i+len(w)]) != w {
		return d.refuse(BadSyntax, "not "+w)
	}
	d.i += len(w)
	return nil
}

func (d *decoder) object(depth int) (Value, error) {
	if depth > MaxDepth {
		return nil, d.refuse(TooDeep, "more than "+strconv.Itoa(MaxDepth)+" levels")
	}
	d.i++ // {
	o := Object{}
	d.space()
	if d.i < len(d.b) && d.b[d.i] == '}' {
		d.i++
		return o, nil
	}
	for {
		if d.i >= len(d.b) || d.b[d.i] != '"' {
			return nil, d.refuse(BadSyntax, "a key was due")
		}
		at := d.i
		k, err := d.text()
		if err != nil {
			return nil, err
		}
		if _, dup := o[k]; dup {
			return nil, &Error{Kind: DuplicateKey, Offset: at, Detail: strconv.Quote(k) + " twice"}
		}
		d.space()
		if d.i >= len(d.b) || d.b[d.i] != ':' {
			return nil, d.refuse(BadSyntax, "a colon was due after a key")
		}
		d.i++
		d.space()
		v, err := d.value(depth)
		if err != nil {
			return nil, err
		}
		o[k] = v
		if done, err := d.next('}'); done || err != nil {
			return o, err
		}
	}
}

func (d *decoder) array(depth int) (Value, error) {
	if depth > MaxDepth {
		return nil, d.refuse(TooDeep, "more than "+strconv.Itoa(MaxDepth)+" levels")
	}
	d.i++ // [
	a := Array{}
	d.space()
	if d.i < len(d.b) && d.b[d.i] == ']' {
		d.i++
		return a, nil
	}
	for {
		v, err := d.value(depth)
		if err != nil {
			return nil, err
		}
		a = append(a, v)
		if done, err := d.next(']'); done || err != nil {
			return a, err
		}
	}
}

// next reads what follows a member or element: a comma and the space after
// it, or the container's end, which makes done true.
func (d *decoder) next(end byte) (done bool, err error) {
	d.space()
	switch {
	case d.i < len(d.b) && d.b[d.i] == end:
		d.i++
		return true, nil
	case d.i < len(d.b) && d.b[d.i] == ',':
		d.i++
		d.space()
		return false, nil
	default:
		return false, d.refuse(BadSyntax, "a comma or "+strconv.QuoteRune(rune(end))+" was due")
	}
}

// text reads a string, from its opening quote.
func (d *decoder) text() (string, error) {
	d.i++ // "
	var out []byte
	start := d.i
	for d.i < len(d.b) {
		c := d.b[d.i]
		switch {
		case c == '"':
			out = append(out, d.b[start:d.i]...)
			d.i++
			return string(out), nil
		case c == '\\':
			out = append(out, d.b[start:d.i]...)
			var err error
			if out, err = d.escape(out); err != nil {
				return "", err
			}
			start = d.i
		case c < 0x20:
			return "", d.refuse(BadSyntax, "a control character in a string")
		case c < utf8.RuneSelf:
			d.i++
		default:
			r, n := utf8.DecodeRune(d.b[d.i:])
			if r == utf8.RuneError && n == 1 {
				return "", d.refuse(BadUTF8, "a byte that is not UTF-8")
			}
			d.i += n
		}
	}
	return "", d.refuse(BadSyntax, "the input ends inside a string")
}

// escape reads one escape, from its backslash, onto out.
func (d *decoder) escape(out []byte) ([]byte, error) {
	if d.i+1 >= len(d.b) {
		return nil, d.refuse(BadSyntax, "the input ends inside an escape")
	}
	if c, ok := short(d.b[d.i+1]); ok {
		d.i += 2
		return append(out, c), nil
	}
	if d.b[d.i+1] != 'u' {
		return nil, d.refuse(BadSyntax, "an escape JSON does not have")
	}
	r, err := d.hex4()
	if err != nil {
		return nil, err
	}
	switch {
	case r >= 0xdc00 && r <= 0xdfff:
		d.i -= 6
		return nil, d.refuse(BadUTF8, "the second half of a surrogate pair alone")
	case r >= 0xd800 && r <= 0xdbff:
		if len(d.b)-d.i < 2 || d.b[d.i] != '\\' || d.b[d.i+1] != 'u' {
			d.i -= 6
			return nil, d.refuse(BadUTF8, "the first half of a surrogate pair alone")
		}
		low, err := d.hex4()
		if err != nil {
			return nil, err
		}
		if low < 0xdc00 || low > 0xdfff {
			d.i -= 12
			return nil, d.refuse(BadUTF8, "the first half of a surrogate pair alone")
		}
		r = 0x10000 + (r-0xd800)<<10 + (low - 0xdc00)
	}
	return utf8.AppendRune(out, r), nil
}

// short is the byte a one-letter escape stands for.
func short(c byte) (byte, bool) {
	switch c {
	case '"', '\\', '/':
		return c, true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	default:
		return 0, false
	}
}

// hex4 reads one \uXXXX escape, from its backslash, which the caller has
// seen is followed by u.
func (d *decoder) hex4() (rune, error) {
	if len(d.b)-d.i < 6 {
		return 0, d.refuse(BadSyntax, "the input ends inside an escape")
	}
	var r rune
	for _, c := range d.b[d.i+2 : d.i+6] {
		var v byte
		switch {
		case c >= '0' && c <= '9':
			v = c - '0'
		case c >= 'a' && c <= 'f':
			v = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = c - 'A' + 10
		default:
			return 0, d.refuse(BadSyntax, "a \\u escape with a digit that is not hex")
		}
		r = r<<4 | rune(v)
	}
	d.i += 6
	return r, nil
}

// number reads a number in JSON's grammar: an optional minus, an integer part
// with no leading zero, then an optional fraction and exponent.
func (d *decoder) number() (Value, error) {
	start := d.i
	if d.b[d.i] == '-' {
		d.i++
	}
	switch {
	case d.i < len(d.b) && d.b[d.i] == '0':
		d.i++
	case d.digits() == 0:
		return nil, d.refuse(BadSyntax, "a number with no digits")
	}
	if d.i < len(d.b) && d.b[d.i] == '.' {
		d.i++
		if d.digits() == 0 {
			return nil, d.refuse(BadSyntax, "a point with no digits after it")
		}
	}
	mantissa := d.i // the end of the digits before any exponent
	if d.i < len(d.b) && (d.b[d.i] == 'e' || d.b[d.i] == 'E') {
		d.i++
		if d.i < len(d.b) && (d.b[d.i] == '+' || d.b[d.i] == '-') {
			d.i++
		}
		if d.digits() == 0 {
			return nil, d.refuse(BadSyntax, "an exponent with no digits")
		}
	}
	lit := string(d.b[start:d.i])
	f, err := strconv.ParseFloat(lit, 64)
	switch {
	case err != nil || math.Abs(f) > MaxSafe:
		// err is ErrRange alone here, a magnitude past a binary64's.
		return nil, &Error{Kind: TooLarge, Offset: start, Detail: lit + " is past 2^53 - 1, so a reader would round it"}
	case f == 0 && strings.ContainsAny(lit[:mantissa-start], "123456789"):
		return nil, &Error{Kind: Underflow, Offset: start, Detail: lit + " is too small for a binary64, so a reader would read 0"}
	case f == math.Trunc(f):
		return Int(int64(f)), nil
	}
	return Float(f), nil
}

// digits skips decimal digits and says how many.
func (d *decoder) digits() int {
	n := 0
	for d.i < len(d.b) && d.b[d.i] >= '0' && d.b[d.i] <= '9' {
		d.i++
		n++
	}
	return n
}
