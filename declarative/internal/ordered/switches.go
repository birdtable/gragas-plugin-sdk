package ordered

import (
	"slices"

	"go.yaml.in/yaml/v3"
)

// A Switch is a value under a named key that ParseYAML reads as a boolean
// and that is not the plain word true or false: the line it is on, its key,
// and the word as written.
type Switch struct {
	Line       int
	Key, Value string
}

// LooseSwitches is each value under a key named in keys, in any mapping of
// the one YAML document in data, that ParseYAML reads as a boolean though it
// is not the plain, untagged word true or false: YAML 1.1's yes, no, on and
// off in each of their cases, True and FALSE, a !!bool tag, or an alias of
// one. A YAML 1.2 reader, as tier's is, takes none of them as a boolean, so a
// file that holds one is read one way here and another there. A value that
// ParseYAML reads as no boolean is a schema's to refuse, not this.
func LooseSwitches(data []byte, keys ...string) ([]Switch, error) {
	n, err := decodeOne(data)
	if n == nil || err != nil {
		return nil, err
	}
	var out []Switch
	looseSwitches(n, keys, 0, &out)
	return out, nil
}

// looseSwitches adds below n each loose switch under a key named in keys. It
// follows no alias down, so a document of aliases costs no more than its
// text; the alias that is a key's value is read through to its scalar.
func looseSwitches(n *yaml.Node, keys []string, depth int, out *[]Switch) {
	if depth > maxDepth || n.Kind == yaml.AliasNode {
		return
	}
	if n.Kind == yaml.MappingNode {
		loosePairs(n, keys, out)
	}
	for _, c := range n.Content {
		looseSwitches(c, keys, depth+1, out)
	}
}

// loosePairs adds each loose switch among the pairs of the mapping n, read
// through an alias that is a pair's value.
func loosePairs(n *yaml.Node, keys []string, out *[]Switch) {
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if !slices.Contains(keys, k.Value) {
			continue
		}
		at := v.Line
		if v.Kind == yaml.AliasNode && v.Alias != nil {
			v = v.Alias
		}
		if v.Kind != yaml.ScalarNode {
			continue
		}
		if b, isBool := scalarOf(v); isBool && !plainWord(v, b) {
			*out = append(*out, Switch{Line: at, Key: k.Value, Value: v.Value})
		}
	}
}

// scalarOf is the boolean ParseYAML reads from the scalar n, and whether it
// reads one.
func scalarOf(n *yaml.Node) (bool, bool) {
	v, err := (&converter{}).convert(n, 0)
	if err != nil {
		return false, false
	}
	b, isBool := v.(bool)
	return b, isBool
}

// plainWord is whether n is written as the plain, untagged word for b.
func plainWord(n *yaml.Node, b bool) bool {
	word := "false"
	if b {
		word = "true"
	}
	return n.Style == 0 && n.Value == word
}
