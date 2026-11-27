package render

import (
	"errors"
	"strings"
)

// JSONC is v written as JSON with comments (what an editor's settings file or a tool's .jsonc config holds): keys
// in the order given, two spaces of indent, no trailing comma, and the header, when there is one, as // lines
// before it. The document is valid JSON apart from the header.
func JSONC(v any, opts ...Option) (string, error) {
	head, err := settings(opts).comment("//")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(head)
	if err := jsonNode(&b, v, 0); err != nil {
		return "", err
	}
	b.WriteString("\n")
	return b.String(), nil
}

func jsonNode(b *strings.Builder, v any, depth int) error {
	if m, ok, err := members(v); ok || err != nil {
		if err != nil {
			return err
		}
		if len(m) == 0 {
			b.WriteString("{}")
			return nil
		}
		b.WriteString("{\n")
		for i, mem := range m {
			k, err := quote(mem.Key)
			if err != nil {
				return err
			}
			b.WriteString(indentOf(depth+1) + k + ": ")
			if err := jsonNode(b, mem.Value, depth+1); err != nil {
				return err
			}
			if i < len(m)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indentOf(depth) + "}")
		return nil
	}
	if l, ok := items(v); ok {
		if len(l) == 0 {
			b.WriteString("[]")
			return nil
		}
		b.WriteString("[\n")
		for i, it := range l {
			b.WriteString(indentOf(depth + 1))
			if err := jsonNode(b, it, depth+1); err != nil {
				return err
			}
			if i < len(l)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(indentOf(depth) + "]")
		return nil
	}
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
		return nil
	case bool:
		if t {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		return nil
	case string:
		s, err := quote(t)
		b.WriteString(s)
		return err
	}
	if n, ok, err := number(v); ok || err != nil {
		b.WriteString(n)
		return err
	}
	return errors.New("a value of a kind JSON cannot write here")
}
