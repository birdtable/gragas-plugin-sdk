package render

import (
	"errors"
	"fmt"
	"strings"
)

// TOML is v written as a TOML document. The top level must be a table. Within a table the plain keys come first,
// then its sub-tables, then its lists of tables, each in the order given, since TOML wants a table's own keys
// before any table below it. A list is written inline, except a list whose items are all tables, which is an array
// of tables ([[name]]). TOML has no null: nil is an error, and so is a list of tables mixed with other items.
func TOML(v any, opts ...Option) (string, error) {
	head, err := settings(opts).comment("#")
	if err != nil {
		return "", err
	}
	m, ok, err := members(v)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("the top level of a TOML document is a table")
	}
	var b strings.Builder
	b.WriteString(head)
	if err := tomlTable(&b, nil, m, head != ""); err != nil {
		return "", err
	}
	return b.String(), nil
}

// tableKind is how a member is written: with its table's own keys, as a table below it, or as an array of tables.
type tableKind int

const (
	own tableKind = iota
	sub
	arrayOf
)

func kindOf(v any) (tableKind, error) {
	if _, ok, err := members(v); ok || err != nil {
		return sub, err
	}
	l, ok := items(v)
	if !ok || len(l) == 0 {
		return own, nil
	}
	tables := 0
	for _, it := range l {
		if _, ok, err := members(it); err != nil {
			return own, err
		} else if ok {
			tables++
		}
	}
	switch tables {
	case 0:
		return own, nil
	case len(l):
		return arrayOf, nil
	}
	return own, errors.New("a list mixes tables with other items, which TOML cannot write inline here")
}

// tomlTable writes the members of the table at path: its own keys, then its sub-tables, then its arrays of tables.
// started is whether something has been written before, so that a blank line parts the tables.
func tomlTable(b *strings.Builder, path []string, m Map, started bool) error {
	kinds := make([]tableKind, len(m))
	for i, mem := range m {
		k, err := kindOf(mem.Value)
		if err != nil {
			return fmt.Errorf("%s: %w", dotted(append(path, mem.Key)), err)
		}
		kinds[i] = k
	}
	for i, mem := range m {
		if kinds[i] != own {
			continue
		}
		key, err := tomlKey(mem.Key)
		if err != nil {
			return err
		}
		val, err := tomlValue(mem.Value)
		if err != nil {
			return fmt.Errorf("%s: %w", dotted(append(path, mem.Key)), err)
		}
		b.WriteString(key + " = " + val + "\n")
		started = true
	}
	for _, want := range []tableKind{sub, arrayOf} {
		for i, mem := range m {
			if kinds[i] != want {
				continue
			}
			at := append(append([]string{}, path...), mem.Key)
			if want == sub {
				inner, _, err := members(mem.Value)
				if err != nil {
					return err
				}
				if err := tomlHeader(b, "[", "]", at, &started); err != nil {
					return err
				}
				if err := tomlTable(b, at, inner, true); err != nil {
					return err
				}
				continue
			}
			l, _ := items(mem.Value)
			for _, it := range l {
				inner, _, err := members(it)
				if err != nil {
					return err
				}
				if err := tomlHeader(b, "[[", "]]", at, &started); err != nil {
					return err
				}
				if err := tomlTable(b, at, inner, true); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// tomlHeader writes a table's header line, parted from what is before it by a blank line.
func tomlHeader(b *strings.Builder, open, closer string, path []string, started *bool) error {
	keys := make([]string, len(path))
	for i, p := range path {
		k, err := tomlKey(p)
		if err != nil {
			return err
		}
		keys[i] = k
	}
	if *started {
		b.WriteString("\n")
	}
	b.WriteString(open + strings.Join(keys, ".") + closer + "\n")
	*started = true
	return nil
}

func dotted(path []string) string { return strings.Join(path, ".") }

// tomlKey is a key as a bare key when it is only letters, digits, _ and -, and quoted when not.
func tomlKey(k string) (string, error) {
	if k != "" {
		bare := true
		for i := 0; i < len(k); i++ {
			c := k[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
				bare = false
			}
		}
		if bare {
			return k, nil
		}
	}
	return quote(k)
}

// tomlValue is a value written inline: a string, a boolean, a number, or a list of values.
func tomlValue(v any) (string, error) {
	if l, ok := items(v); ok {
		parts := make([]string, len(l))
		for i, it := range l {
			s, err := tomlValue(it)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	}
	if _, ok, _ := members(v); ok {
		return "", errors.New("a table inside a list of other items cannot be written inline here")
	}
	switch t := v.(type) {
	case nil:
		return "", errors.New("TOML has no null")
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case string:
		return quote(t)
	}
	n, ok, err := number(v)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errors.New("a value of a kind TOML cannot write here")
	}
	// A float keeps a point or an exponent: TOML reads 5 as an integer and 5.0 as a float. A value that came as an
	// integer stays one.
	if _, isFloat := v.(float64); isFloat && !strings.ContainsAny(n, ".eE") {
		n += ".0"
	}
	return n, nil
}
