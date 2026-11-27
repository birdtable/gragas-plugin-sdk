package ordered

import (
	"strconv"
	"strings"
	"unicode"
)

// YAML writes v as a block-style YAML document, two spaces to a level, keeping
// key order. v is what ParseYAML and JSON take: nil, bool, int, int64,
// float64, string, Timestamp, []any and *Map. It is the writer of a generated
// preset, and it keeps a string plain only where the YAML 1.1
// and the YAML 1.2 readings of the text both give that same string, so a
// loader of either generation reads back what was written; any other string
// is written double-quoted. A value of another type is written as null. The
// output ends in a newline and holds no date and no clock reading.
func YAML(v any) string {
	var b strings.Builder
	switch x := v.(type) {
	case []any:
		if len(x) > 0 {
			writeYAMLSeq(&b, x, 0)
			return b.String()
		}
	case *Map:
		if x.Len() > 0 {
			writeYAMLMap(&b, x, 0)
			return b.String()
		}
	}
	b.WriteString(yamlInline(v))
	b.WriteByte('\n')
	return b.String()
}

// plainSafe says whether s may be written with no quotes: a conservative
// shape (a letter, digit, underscore, slash or dot first, then those, a dash
// and single inner spaces) whose 1.1 and 1.2 readings are both s itself.
func plainSafe(s string) bool {
	if s == "" || s != strings.TrimSpace(s) {
		return false
	}
	for i, r := range s {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_./", r)
		if i > 0 {
			ok = ok || r == '-' || r == ' '
		}
		if !ok || r > unicode.MaxASCII {
			return false
		}
	}
	if strings.Contains(s, "  ") || strings.HasPrefix(s, "...") || strings.Contains("yYnN", s) {
		// "y" and "n" are booleans to a YAML 1.1 loader that keeps the
		// whole table, and "..." at a line start ends a document.
		return false
	}
	r11, err := resolve(s, 0)
	if err != nil {
		return false
	}
	t11, ok11 := r11.(string)
	t12, ok12 := resolve12(s).(string)
	return ok11 && ok12 && t11 == s && t12 == s
}

// yamlString is s as a YAML scalar: plain where both readings agree, else
// double-quoted by writeYAMLQuoted.
func yamlString(s string) string {
	if plainSafe(s) {
		return s
	}
	var b strings.Builder
	writeYAMLQuoted(&b, s)
	return b.String()
}

// writeYAMLQuoted writes s double-quoted. JSON's escapes cover the quote, the
// backslash and the C0 controls; YAML also needs an escape for every rune that
// is a line break to it or outside its printable set, which a loader would
// otherwise drop, fold to a space or refuse: DEL and the C1 controls (NEL is
// \N), LS (\L), PS (\P), the byte order mark and the two non-characters at
// the top of the plane. A byte that is not UTF-8 is written as U+FFFD.
func writeYAMLQuoted(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case 0x85:
			b.WriteString(`\N`)
		case 0x2028:
			b.WriteString(`\L`)
		case 0x2029:
			b.WriteString(`\P`)
		default:
			switch {
			case r < 0x20 || (r >= 0x7f && r <= 0x9f):
				b.WriteString(`\x`)
				b.WriteByte("0123456789abcdef"[r>>4])
				b.WriteByte("0123456789abcdef"[r&0xf])
			case r == 0xfeff || r == 0xfffe || r == 0xffff:
				b.WriteString(`\u`)
				b.WriteString(strconv.FormatInt(int64(r)+0x10000, 16)[1:])
			default:
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
}

// maxImplicitKey is the longest key YAML lets stand before a colon on one
// line; a longer key is written in the explicit form, `? key` then `: value`.
const maxImplicitKey = 1024

// longKey says whether key, as written (quotes and escapes counted), is over
// maxImplicitKey characters.
func longKey(key string) bool {
	n := 0
	for range key {
		n++
		if n > maxImplicitKey {
			return true
		}
	}
	return false
}

// yamlInline is a scalar, or an empty collection, on one line.
func yamlInline(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return yamlFloat(x)
	case string:
		return yamlString(x)
	case Timestamp:
		return yamlString(string(x))
	case []any:
		return "[]"
	case *Map:
		return "{}"
	default:
		return "null"
	}
}

func yamlFloat(f float64) string {
	switch {
	case f != f:
		return ".nan"
	case f > 1.7976931348623157e308:
		return ".inf"
	case f < -1.7976931348623157e308:
		return "-.inf"
	}
	s := FloatRepr(f)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func yamlBlock(v any) bool {
	switch x := v.(type) {
	case []any:
		return len(x) > 0
	case *Map:
		return x.Len() > 0
	}
	return false
}

func pad(b *strings.Builder, depth int) {
	for range depth {
		b.WriteString("  ")
	}
}

func writeYAMLMap(b *strings.Builder, m *Map, depth int) {
	for _, k := range m.keys {
		pad(b, depth)
		key := yamlString(k)
		if longKey(key) {
			b.WriteString("? ")
			b.WriteString(key)
			b.WriteByte('\n')
			pad(b, depth)
		} else {
			b.WriteString(key)
		}
		b.WriteByte(':')
		v := m.vals[k]
		if !yamlBlock(v) {
			b.WriteByte(' ')
			b.WriteString(yamlInline(v))
			b.WriteByte('\n')
			continue
		}
		b.WriteByte('\n')
		writeYAMLNode(b, v, depth+1)
	}
}

func writeYAMLSeq(b *strings.Builder, s []any, depth int) {
	for _, e := range s {
		pad(b, depth)
		b.WriteByte('-')
		if !yamlBlock(e) {
			b.WriteByte(' ')
			b.WriteString(yamlInline(e))
			b.WriteByte('\n')
			continue
		}
		b.WriteByte('\n')
		writeYAMLNode(b, e, depth+1)
	}
}

func writeYAMLNode(b *strings.Builder, v any, depth int) {
	switch x := v.(type) {
	case []any:
		writeYAMLSeq(b, x, depth)
	case *Map:
		writeYAMLMap(b, x, depth)
	}
}
