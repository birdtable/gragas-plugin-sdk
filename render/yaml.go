package render

import (
	"errors"
	"fmt"
	"strings"
)

// yamlReserved are the plain words YAML reads as something other than a string.
var yamlReserved = map[string]bool{"true": true, "false": true, "null": true, "yes": true, "no": true, "on": true,
	"off": true, "y": true, "n": true, "~": true}

// plainYAML is whether s may stand unquoted: it starts with a letter or an underscore, holds only letters, digits
// and _ . / -, and is not a word YAML reads as a boolean or null. So no number, no indicator and no spare space can
// make a plain string a different value.
func plainYAML(s string) bool {
	if s == "" || yamlReserved[strings.ToLower(s)] {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_'
		if i == 0 && !letter {
			return false
		}
		if !letter && !(c >= '0' && c <= '9') && c != '.' && c != '/' && c != '-' {
			return false
		}
	}
	return true
}

func yamlString(s string) (string, error) {
	if plainYAML(s) {
		return s, nil
	}
	return quote(s)
}

// YAML is v written as a YAML document in block style. A table written as {} and a list as [] when empty. The
// top level is any value; a scalar is written alone.
func YAML(v any, opts ...Option) (string, error) {
	head, err := settings(opts).comment("#")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(head)
	if err := yamlNode(&b, v, 0, true); err != nil {
		return "", err
	}
	return b.String(), nil
}

// yamlNode writes v at depth. top is whether it stands where a block may start on its own line.
func yamlNode(b *strings.Builder, v any, depth int, top bool) error {
	if m, ok, err := members(v); ok || err != nil {
		if err != nil {
			return err
		}
		if len(m) == 0 {
			b.WriteString("{}\n")
			return nil
		}
		for _, mem := range m {
			if err := yamlMember(b, indentOf(depth), mem, depth); err != nil {
				return err
			}
		}
		return nil
	}
	if l, ok := items(v); ok {
		if len(l) == 0 {
			b.WriteString("[]\n")
			return nil
		}
		for _, it := range l {
			if err := yamlItem(b, indentOf(depth), it, depth); err != nil {
				return err
			}
		}
		return nil
	}
	s, err := yamlScalar(v)
	if err != nil {
		return err
	}
	b.WriteString(s + "\n")
	return nil
}

// yamlMember writes "key: value", the value on the next lines when it is a non-empty table or list.
func yamlMember(b *strings.Builder, pad string, m Member, depth int) error {
	k, err := yamlString(m.Key)
	if err != nil {
		return fmt.Errorf("the key %q: %w", m.Key, err)
	}
	b.WriteString(pad + k + ":")
	if nested(m.Value) {
		b.WriteString("\n")
		return yamlNode(b, m.Value, depth+1, false)
	}
	return yamlInline(b, m.Value)
}

// yamlItem writes "- value"; a table's first key stands on the dash's line, and the rest below it.
func yamlItem(b *strings.Builder, pad string, v any, depth int) error {
	if m, ok, err := members(v); ok || err != nil {
		if err != nil {
			return err
		}
		if len(m) == 0 {
			b.WriteString(pad + "- {}\n")
			return nil
		}
		for i, mem := range m {
			lead := pad + "  "
			if i == 0 {
				lead = pad + "- "
			}
			if err := yamlMember(b, lead, mem, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if l, ok := items(v); ok {
		if len(l) == 0 {
			b.WriteString(pad + "- []\n")
			return nil
		}
		b.WriteString(pad + "-\n")
		return yamlNode(b, v, depth+1, false)
	}
	b.WriteString(pad + "-")
	return yamlInline(b, v)
}

// yamlInline writes a scalar, or an empty table or list, after what stands before it on its line.
func yamlInline(b *strings.Builder, v any) error {
	if _, ok, err := members(v); ok || err != nil {
		if err != nil {
			return err
		}
		b.WriteString(" {}\n")
		return nil
	}
	if _, ok := items(v); ok {
		b.WriteString(" []\n")
		return nil
	}
	s, err := yamlScalar(v)
	if err != nil {
		return err
	}
	b.WriteString(" " + s + "\n")
	return nil
}

// nested is whether v is a table or a list with something in it, which YAML writes below its key.
func nested(v any) bool {
	if m, ok, _ := members(v); ok {
		return len(m) > 0
	}
	if l, ok := items(v); ok {
		return len(l) > 0
	}
	return false
}

func yamlScalar(v any) (string, error) {
	switch t := v.(type) {
	case nil:
		return "null", nil
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case string:
		return yamlString(t)
	}
	if n, ok, err := number(v); ok || err != nil {
		return n, err
	}
	return "", errors.New("a value of a kind YAML cannot write here")
}
