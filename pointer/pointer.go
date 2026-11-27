// Package pointer reads and writes JSON pointers (RFC 6901), which an error's path uses to name the field of the
// request at fault, for example /have/0/stdout or /entries/3/options.
package pointer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Escape is one reference token written for a pointer: ~ becomes ~0 and / becomes ~1.
func Escape(token string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(token)
}

// Unescape is one escaped token read back. A ~ that is not followed by 0 or 1 is an error.
func Unescape(token string) (string, error) {
	if !strings.Contains(token, "~") {
		return token, nil
	}
	var b strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] != '~' {
			b.WriteByte(token[i])
			continue
		}
		if i+1 >= len(token) || (token[i+1] != '0' && token[i+1] != '1') {
			return "", fmt.Errorf("the token %q holds a ~ not followed by 0 or 1", token)
		}
		if token[i+1] == '0' {
			b.WriteByte('~')
		} else {
			b.WriteByte('/')
		}
		i++
	}
	return b.String(), nil
}

// Join is the pointer of the tokens, each escaped, in order. No token is the pointer to the whole document, "".
func Join(tokens ...string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(Escape(t))
	}
	return b.String()
}

// Index is the pointer to item i of a list, as a token of Join.
func Index(i int) string { return strconv.Itoa(i) }

// Parse is the tokens of a pointer. "" is the whole document and has none; any other pointer starts with /.
func Parse(ptr string) ([]string, error) {
	if ptr == "" {
		return nil, nil
	}
	if ptr[0] != '/' {
		return nil, errors.New("a pointer is empty or starts with /")
	}
	var out []string
	for _, raw := range strings.Split(ptr[1:], "/") {
		t, err := Unescape(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// Get is the value ptr names inside doc, a document read by encoding/json into any: objects are
// map[string]any and lists are []any. It is false for a pointer that is not valid or names nothing. A list token
// is a decimal index with no leading zero, as the RFC has it.
func Get(doc any, ptr string) (any, bool) {
	tokens, err := Parse(ptr)
	if err != nil {
		return nil, false
	}
	cur := doc
	for _, t := range tokens {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[t]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			i, ok := listIndex(t)
			if !ok || i >= len(v) {
				return nil, false
			}
			cur = v[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// listIndex is the index a token names: digits only, no leading zero unless the token is "0".
func listIndex(t string) (int, bool) {
	if t == "" || (len(t) > 1 && t[0] == '0') {
		return 0, false
	}
	for _, c := range t {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	i, err := strconv.Atoi(t)
	return i, err == nil
}
