package ordered

import (
	"bytes"
	"encoding/json/jsontext"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// JSON encodes v with the given indent (spaces per level), keeping key order,
// leaving non-ASCII text unescaped, and writing floats in shortest round-trip
// form. With indent < 0 it writes one line with ", " and ": " separators.
// This is the byte layout of the committed rules.json and bindings.json.
// The text goes through encoding/json/jsontext:
// its string escaping is the layout (only the quote, the backslash and the
// control characters below U+0020 are escaped; invalid UTF-8 becomes U+FFFD),
// and floats go in as raw numbers because FloatRepr's layout ("1.0", "1e-05")
// is not jsontext's.
func JSON(v any, indent int) string {
	opts := []jsontext.Options{
		jsontext.AllowInvalidUTF8(true),
		jsontext.AllowDuplicateNames(true),
		jsontext.SpaceAfterColon(true),
	}
	if indent < 0 {
		opts = append(opts, jsontext.SpaceAfterComma(true))
	} else {
		opts = append(opts, jsontext.Multiline(true), jsontext.WithIndent(strings.Repeat(" ", indent)))
	}
	var b bytes.Buffer
	w := &jsonWriter{e: jsontext.NewEncoder(&b, opts...)}
	w.write(v)
	if w.err != nil {
		// Writing into a bytes.Buffer fails only on a malformed token
		// sequence, and write cannot produce one.
		panic("ordered.JSON: " + w.err.Error())
	}
	// The encoder ends each top-level value with a newline; JSON does not.
	return strings.TrimSuffix(b.String(), "\n")
}

// jsonWriter writes tokens to e and keeps the first error, so write reads as
// the layout and JSON checks once.
type jsonWriter struct {
	e   *jsontext.Encoder
	err error
}

func (w *jsonWriter) token(t jsontext.Token) {
	if w.err == nil {
		w.err = w.e.WriteToken(t)
	}
}

func (w *jsonWriter) write(v any) {
	switch x := v.(type) {
	case nil:
		w.token(jsontext.Null)
	case bool:
		w.token(jsontext.Bool(x))
	case int64:
		w.token(jsontext.Int(x))
	case int:
		w.token(jsontext.Int(int64(x)))
	case float64:
		w.float(x)
	case string:
		w.token(jsontext.String(x))
	case Timestamp:
		w.token(jsontext.String(string(x)))
	case []any:
		w.token(jsontext.BeginArray)
		for _, el := range x {
			w.write(el)
		}
		w.token(jsontext.EndArray)
	case *Map:
		w.token(jsontext.BeginObject)
		for _, k := range x.keys {
			w.token(jsontext.String(k))
			w.write(x.vals[k])
		}
		w.token(jsontext.EndObject)
	default:
		w.token(jsontext.Null)
	}
}

// float writes f. JSON has no NaN or infinity, so those are written the way
// Plain holds them: {"$yaml": "nan"}, "inf" or "-inf".
func (w *jsonWriter) float(f float64) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		m := NewMap(0)
		m.Set("$yaml", FloatRepr(f), 0)
		w.write(m)
		return
	}
	if w.err == nil {
		w.err = w.e.WriteValue(jsontext.Value(FloatRepr(f)))
	}
}

// FloatRepr writes f in shortest round-trip form with the original tooling's
// layout: fixed notation with at least one decimal while the exponent is in
// [-4, 16), else d.ddde±XX with at least two exponent digits.
// So 1e6 is "1000000.0", 1e16 is "1e+16" and 0.00001 is "1e-05".
func FloatRepr(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	e := strconv.FormatFloat(f, 'e', -1, 64) // like -1.2345e+06
	neg := strings.HasPrefix(e, "-")
	e = strings.TrimPrefix(e, "-")
	mant, expStr, _ := strings.Cut(e, "e")
	exp, err := strconv.Atoi(expStr)
	if err != nil {
		// FormatFloat always writes a decimal exponent; keep its own form if not.
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	digits := strings.Replace(mant, ".", "", 1)
	var out string
	if exp >= -4 && exp < 16 {
		if exp >= 0 {
			if len(digits) <= exp+1 {
				out = digits + strings.Repeat("0", exp+1-len(digits)) + ".0"
			} else {
				out = digits[:exp+1] + "." + digits[exp+1:]
			}
		} else {
			out = "0." + strings.Repeat("0", -exp-1) + digits
		}
	} else {
		m := digits[:1]
		if len(digits) > 1 {
			m += "." + digits[1:]
		}
		sign := "+"
		if exp < 0 {
			sign = "-"
			exp = -exp
		}
		es := strconv.Itoa(exp)
		if len(es) < 2 {
			es = "0" + es
		}
		out = m + "e" + sign + es
	}
	if neg {
		out = "-" + out
	}
	return out
}

// Repr writes v the way the original tooling quoted values in its messages:
// strings in single quotes (double when the text holds a single quote and no
// double one), lists in brackets, mappings in braces, and True, False, None.
func Repr(v any) string {
	var b strings.Builder
	writeRepr(&b, v)
	return b.String()
}

func writeRepr(b *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("None")
	case bool:
		if x {
			b.WriteString("True")
		} else {
			b.WriteString("False")
		}
	case int64:
		b.WriteString(strconv.FormatInt(x, 10))
	case int:
		b.WriteString(strconv.Itoa(x))
	case float64:
		b.WriteString(FloatRepr(x))
	case string:
		writeStrRepr(b, x)
	case Timestamp:
		b.WriteString(string(x))
	case []any:
		writeListRepr(b, x, writeRepr)
	case []string:
		writeListRepr(b, x, writeStrRepr)
	case *Map:
		writeMapRepr(b, x)
	default:
		b.WriteString("?")
	}
}

// writeListRepr writes xs in brackets, each element by each.
func writeListRepr[T any](b *strings.Builder, xs []T, each func(*strings.Builder, T)) {
	b.WriteByte('[')
	for i, e := range xs {
		if i > 0 {
			b.WriteString(", ")
		}
		each(b, e)
	}
	b.WriteByte(']')
}

// writeMapRepr writes m in braces, its keys quoted as strings.
func writeMapRepr(b *strings.Builder, m *Map) {
	b.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			b.WriteString(", ")
		}
		writeStrRepr(b, k)
		b.WriteString(": ")
		writeRepr(b, m.vals[k])
	}
	b.WriteByte('}')
}

func writeStrRepr(b *strings.Builder, s string) {
	q := byte('\'')
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		q = '"'
	}
	b.WriteByte(q)
	for _, r := range s {
		switch {
		case r == rune(q) || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == ' ' || unicode.IsPrint(r):
			b.WriteRune(r)
		case r < 0x100:
			b.WriteString(`\x`)
			b.WriteString(hex(int(r), 2))
		case r < 0x10000:
			b.WriteString(`\u`)
			b.WriteString(hex(int(r), 4))
		default:
			b.WriteString(`\U`)
			b.WriteString(hex(int(r), 8))
		}
	}
	b.WriteByte(q)
}

func hex(v, width int) string {
	s := strconv.FormatInt(int64(v), 16)
	return strings.Repeat("0", width-len(s)) + s
}

// Str writes v as text is interpolated into the part READMEs: a string as
// itself, anything else as Repr.
func Str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return Repr(v)
}
