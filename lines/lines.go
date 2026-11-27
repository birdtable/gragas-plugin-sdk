// Package lines reads the lines a tool prints by a line pattern: literal text with fields in braces, for example
//
//	{path}:{line}:{column}: {severity}: {message} ({key})
//
// It turns what a tool printed into matches a plugin maps to findings or entries, and holds no tool's name. The
// fields are {path}, {line}, {column}, {severity}, {key} and {message}; each stands at most once. {{ and }} are a
// literal brace. A line that the pattern does not match is not a match, and is left out, never an error: a tool's
// output holds lines of its own, a summary, a banner.
package lines

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// fields are what a pattern may name, each with what it matches. A path or a message may hold anything but a line
// end; the others are narrower, so the literal text around a field decides where it ends.
var fields = map[string]string{
	"path":     `.+?`,
	"line":     `[0-9]+`,
	"column":   `[0-9]+`,
	"severity": `[A-Za-z]+`,
	"key":      `[^\s()\[\]]+`,
	"message":  `.+?`,
}

// Pattern is a compiled line pattern.
type Pattern struct {
	re    *regexp.Regexp
	names []string
}

// Match is the fields one line gave. A field the pattern does not name, and a line or column of 0 or beyond
// 4294967295 (which no contract line may hold), are zero, and Raw is the line as it was, without its line end.
type Match struct {
	Raw      string
	Path     string
	Line     int
	Column   int
	Severity string
	Key      string
	Message  string
}

// Compile is the pattern of text, or an error for a brace that is not a field of the list above, an unclosed
// one, a field named twice, or a pattern that names no field.
func Compile(text string) (*Pattern, error) {
	var re strings.Builder
	var names []string
	seen := map[string]bool{}
	re.WriteString("^")
	for i := 0; i < len(text); {
		switch c := text[i]; {
		case c == '{' && strings.HasPrefix(text[i:], "{{"):
			re.WriteString(regexp.QuoteMeta("{"))
			i += 2
		case c == '}' && strings.HasPrefix(text[i:], "}}"):
			re.WriteString(regexp.QuoteMeta("}"))
			i += 2
		case c == '{':
			end := strings.IndexByte(text[i:], '}')
			if end < 0 {
				return nil, errors.New("a { is not closed")
			}
			name := text[i+1 : i+end]
			expr, ok := fields[name]
			if !ok {
				return nil, fmt.Errorf("the field {%s} is not one of path, line, column, severity, key and message", name)
			}
			if seen[name] {
				return nil, fmt.Errorf("the field {%s} is named twice", name)
			}
			seen[name] = true
			names = append(names, name)
			re.WriteString("(" + expr + ")")
			i += end + 1
		case c == '}':
			return nil, errors.New("a } closes nothing")
		default:
			// A run of literal bytes, up to the next brace.
			j := i
			for j < len(text) && text[j] != '{' && text[j] != '}' {
				j++
			}
			re.WriteString(regexp.QuoteMeta(text[i:j]))
			i = j
		}
	}
	re.WriteString("$")
	if len(names) == 0 {
		return nil, errors.New("a pattern names no field")
	}
	rx, err := regexp.Compile(re.String())
	if err != nil {
		return nil, fmt.Errorf("the pattern does not compile: %w", err)
	}
	return &Pattern{re: rx, names: names}, nil
}

// MustCompile is Compile for a pattern written in the plugin's own source: it panics on one that does not
// compile, which a plugin's first test finds.
func MustCompile(text string) *Pattern {
	p, err := Compile(text)
	if err != nil {
		panic("lines: " + err.Error())
	}
	return p
}

// Parse is the match of each line of text the pattern matches, in the order of the lines. A line may end \n or \r\n.
func (p *Pattern) Parse(text string) []Match {
	var out []Match
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		got := p.re.FindStringSubmatch(line)
		if got == nil {
			continue
		}
		m := Match{Raw: line}
		for i, name := range p.names {
			v := got[i+1]
			switch name {
			case "path":
				m.Path = v
			case "line":
				m.Line = number(v)
			case "column":
				m.Column = number(v)
			case "severity":
				m.Severity = v
			case "key":
				m.Key = v
			case "message":
				m.Message = v
			}
		}
		out = append(out, m)
	}
	return out
}

// number is v as a line or a column, or 0 for a number the contract cannot hold. A 32-bit target also drops what its int
// cannot hold (from 2147483648 to the contract's 4294967295), as sarif's within does: int(n) would wrap negative there.
func number(v string) int {
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil || n > math.MaxInt {
		return 0
	}
	return int(n)
}
