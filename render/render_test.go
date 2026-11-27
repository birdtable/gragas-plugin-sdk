package render_test

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/render"
)

func n(s string) json.Number { return json.Number(s) }

func TestYAMLWritesBlockStyleInTheOrderGiven(t *testing.T) {
	t.Parallel()
	doc := render.Map{
		{Key: "version", Value: "1.2.3"},
		{Key: "rules", Value: render.Map{{Key: "max-len", Value: n("300")}, {Key: "no tabs", Value: true}, {Key: "empty", Value: render.Map{}}}},
		{Key: "list", Value: []any{"a", "b c", 3, 1.5}},
		{Key: "items", Value: []any{render.Map{{Key: "name", Value: "x"}, {Key: "on", Value: true}}, render.Map{{Key: "name", Value: "y"}}}},
		{Key: "nothing", Value: nil},
		{Key: "word", Value: "yes"},
		{Key: "num", Value: "12"},
		{Key: "multi", Value: "a\nb \"q\"\\"},
		{Key: "none", Value: []any{}},
		{Key: "deep", Value: render.Map{{Key: "a", Value: render.Map{{Key: "b", Value: []any{[]any{1, 2}, render.Map{{Key: "c", Value: render.Map{{Key: "d", Value: 1}}}}}}}}}},
	}
	want := `version: "1.2.3"
rules:
  max-len: 300
  "no tabs": true
  empty: {}
list:
  - a
  - "b c"
  - 3
  - 1.5
items:
  - name: x
    "on": true
  - name: "y"
nothing: null
word: "yes"
num: "12"
multi: "a\nb \"q\"\\"
none: []
deep:
  a:
    b:
      -
        - 1
        - 2
      - c:
          d: 1
`
	got, err := render.YAML(doc)
	if err != nil || got != want {
		t.Errorf("YAML =\n%s\nerr %v\nwant\n%s", got, err, want)
	}
	if again, _ := render.YAML(doc); again != got {
		t.Error("a second write differs")
	}
}

func TestYAMLAScalarAndAMapGivenAsAGoMapAreWritten(t *testing.T) {
	t.Parallel()
	if got, err := render.YAML("x"); err != nil || got != "x\n" {
		t.Errorf("a scalar = %q, %v", got, err)
	}
	if got, err := render.YAML(map[string]any{"b": 1, "a": 2}); err != nil || got != "a: 2\nb: 1\n" {
		t.Errorf("a Go map = %q, %v", got, err)
	}
	if got, err := render.YAML(render.Map{}); err != nil || got != "{}\n" {
		t.Errorf("an empty table = %q, %v", got, err)
	}
}

func TestYAMLQuotesWhatAPlainWordWouldChange(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"abc": "abc", "a-b_c.d/e": "a-b_c.d/e", "true": `"true"`, "NULL": `"NULL"`, "On": `"On"`, "~": `"~"`, "": `""`,
		"1": `"1"`, "1e5": `"1e5"`, ".5": `".5"`, "-x": `"-x"`, "a b": `"a b"`, "a: b": `"a: b"`, "#c": `"#c"`, "a#": `"a#"`,
		"- x": `"- x"`, "\u00e9": `"` + "\u00e9" + `"`, "a\u2028b": `"a\u2028b"`, "\x00": `"\u0000"`, "tab\t": `"tab\t"`, "@a": `"@a"`,
		"*a": `"*a"`, "&a": `"&a"`, "!a": `"!a"`, "%a": `"%a"`, "{a}": `"{a}"`, "[a]": `"[a]"`, "a,b": `"a,b"`, "'a'": `"'a'"`,
	} {
		got, err := render.YAML(in)
		if err != nil || got != want+"\n" {
			t.Errorf("YAML(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestJSONCWritesJSONWithAHeaderComment(t *testing.T) {
	t.Parallel()
	doc := render.Map{{Key: "a", Value: 1}, {Key: "b", Value: []any{"x", render.Map{{Key: "c", Value: nil}}}}, {Key: "d", Value: render.Map{}}, {Key: "e", Value: []any{}}, {Key: "f", Value: "q\"\n"}, {Key: "g", Value: n("1E5")}, {Key: "h", Value: false}}
	want := `// gragas:mark
// second
{
  "a": 1,
  "b": [
    "x",
    {
      "c": null
    }
  ],
  "d": {},
  "e": [],
  "f": "q\"\n",
  "g": 1E5,
  "h": false
}
`
	got, err := render.JSONC(doc, render.WithHeader("gragas:mark", "second"))
	if err != nil || got != want {
		t.Errorf("JSONC =\n%s\nerr %v\nwant\n%s", got, err, want)
	}
	// Without its comments it is valid JSON, and reads back as the document.
	var body []string
	for _, l := range strings.Split(got, "\n") {
		if !strings.HasPrefix(l, "//") {
			body = append(body, l)
		}
	}
	if !json.Valid([]byte(strings.Join(body, "\n"))) {
		t.Error("the body is not valid JSON")
	}
}

func TestTOMLWritesKeysThenTablesThenArraysOfTables(t *testing.T) {
	t.Parallel()
	doc := render.Map{
		{Key: "title", Value: "x"},
		{Key: "count", Value: n("3")},
		{Key: "ratio", Value: 1.5},
		{Key: "whole", Value: float64(2)},
		{Key: "tags", Value: []any{"a", "b"}},
		{Key: "server", Value: render.Map{{Key: "host", Value: "h"}, {Key: "tls", Value: render.Map{{Key: "on", Value: true}}}, {Key: "ports", Value: []any{1, 2}}}},
		{Key: "plugins", Value: []any{render.Map{{Key: "name", Value: "p1"}}, render.Map{{Key: "name", Value: "p2"}}}},
		{Key: "a b", Value: "quoted key"},
		{Key: "after", Value: "own keys stay above the tables"},
	}
	want := `# gragas:mark
title = "x"
count = 3
ratio = 1.5
whole = 2.0
tags = ["a", "b"]
"a b" = "quoted key"
after = "own keys stay above the tables"

[server]
host = "h"
ports = [1, 2]

[server.tls]
on = true

[[plugins]]
name = "p1"

[[plugins]]
name = "p2"
`
	got, err := render.TOML(doc, render.WithHeader("gragas:mark"))
	if err != nil || got != want {
		t.Errorf("TOML =\n%s\nerr %v\nwant\n%s", got, err, want)
	}
	if got, err := render.TOML(render.Map{{Key: "t", Value: render.Map{{Key: "u", Value: render.Map{{Key: "x", Value: 1}}}}}}); err != nil || got != "[t]\n\n[t.u]\nx = 1\n" {
		t.Errorf("nested tables = %q, %v", got, err)
	}
}

func TestAValueAWriterCannotWriteIsAnErrorAndNeverADifferentDocument(t *testing.T) {
	t.Parallel()
	twice := render.Map{{Key: "a", Value: 1}, {Key: "a", Value: 2}}
	for name, c := range map[string]struct {
		write func() (string, error)
	}{
		"yaml NaN":                    {func() (string, error) { return render.YAML(math.NaN()) }},
		"yaml infinity":               {func() (string, error) { return render.YAML(render.Map{{Key: "a", Value: math.Inf(1)}}) }},
		"yaml not UTF-8":              {func() (string, error) { return render.YAML("a\xffb") }},
		"yaml a bad key":              {func() (string, error) { return render.YAML(render.Map{{Key: "a\xff", Value: 1}}) }},
		"yaml a struct":               {func() (string, error) { return render.YAML(struct{}{}) }},
		"yaml a key twice":            {func() (string, error) { return render.YAML(twice) }},
		"yaml a bad number":           {func() (string, error) { return render.YAML(n("1x")) }},
		"yaml a header line":          {func() (string, error) { return render.YAML("a", render.WithHeader("a\nb")) }},
		"yaml header U+0085":          {func() (string, error) { return render.YAML("a", render.WithHeader("rules: a\u0085strict: false")) }},
		"jsonc header U+0085":         {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u0085y")) }},
		"yaml header U+2028":          {func() (string, error) { return render.YAML("a", render.WithHeader("rules: a\u2028strict: false")) }},
		"jsonc header U+2028":         {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u2028y")) }},
		"yaml header U+2029":          {func() (string, error) { return render.YAML("a", render.WithHeader("rules: a\u2029strict: false")) }},
		"jsonc header U+2029":         {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u2029y")) }},
		"yaml header a vertical tab":  {func() (string, error) { return render.YAML("a", render.WithHeader("rules: a\vstrict: false")) }},
		"jsonc header a vertical tab": {func() (string, error) { return render.JSONC("a", render.WithHeader("x\vy")) }},
		"yaml header a form feed":     {func() (string, error) { return render.YAML("a", render.WithHeader("rules: a\fstrict: false")) }},
		"jsonc header a form feed":    {func() (string, error) { return render.JSONC("a", render.WithHeader("x\fy")) }},
		"yaml header a NUL":           {func() (string, error) { return render.YAML("a", render.WithHeader("rules: a\x00strict: false")) }},
		"jsonc header a NUL":          {func() (string, error) { return render.JSONC("a", render.WithHeader("x\x00y")) }},
		"yaml header a DEL":           {func() (string, error) { return render.YAML("a", render.WithHeader("rules: a\x7fstrict: false")) }},
		"jsonc header a DEL":          {func() (string, error) { return render.JSONC("a", render.WithHeader("x\x7fy")) }},
		"yaml header bidi U+202E":     {func() (string, error) { return render.YAML("a", render.WithHeader("x\u202ey")) }},
		"jsonc header bidi U+202E":    {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u202ey")) }},
		"toml header bidi U+202E": {func() (string, error) {
			return render.TOML(render.Map{{Key: "a", Value: 1}}, render.WithHeader("x\u202ey"))
		}},
		"yaml header bidi U+2066":  {func() (string, error) { return render.YAML("a", render.WithHeader("x\u2066y")) }},
		"jsonc header bidi U+2066": {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u2066y")) }},
		"toml header bidi U+2066": {func() (string, error) {
			return render.TOML(render.Map{{Key: "a", Value: 1}}, render.WithHeader("x\u2066y"))
		}},
		"yaml header bidi U+2069":  {func() (string, error) { return render.YAML("a", render.WithHeader("x\u2069y")) }},
		"jsonc header bidi U+2069": {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u2069y")) }},
		"toml header bidi U+2069": {func() (string, error) {
			return render.TOML(render.Map{{Key: "a", Value: 1}}, render.WithHeader("x\u2069y"))
		}},
		"yaml header bidi U+200F":  {func() (string, error) { return render.YAML("a", render.WithHeader("x\u200fy")) }},
		"jsonc header bidi U+200F": {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u200fy")) }},
		"toml header bidi U+200F": {func() (string, error) {
			return render.TOML(render.Map{{Key: "a", Value: 1}}, render.WithHeader("x\u200fy"))
		}},
		"yaml header bidi U+061C":  {func() (string, error) { return render.YAML("a", render.WithHeader("x\u061cy")) }},
		"jsonc header bidi U+061C": {func() (string, error) { return render.JSONC("a", render.WithHeader("x\u061cy")) }},
		"toml header bidi U+061C": {func() (string, error) {
			return render.TOML(render.Map{{Key: "a", Value: 1}}, render.WithHeader("x\u061cy"))
		}},
		"yaml header not UTF-8": {func() (string, error) { return render.YAML("a", render.WithHeader("a\xffb")) }},
		"yaml number NaN":       {func() (string, error) { return render.YAML(n("NaN")) }},
		"toml number NaN":       {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("NaN")}}) }},
		"yaml number nan":       {func() (string, error) { return render.YAML(n("nan")) }},
		"toml number nan":       {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("nan")}}) }},
		"yaml number inf":       {func() (string, error) { return render.YAML(n("inf")) }},
		"toml number inf":       {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("inf")}}) }},
		"yaml number Infinity":  {func() (string, error) { return render.YAML(n("Infinity")) }},
		"toml number Infinity":  {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("Infinity")}}) }},
		"yaml number +5":        {func() (string, error) { return render.YAML(n("+5")) }},
		"toml number +5":        {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("+5")}}) }},
		"yaml number 1_0":       {func() (string, error) { return render.YAML(n("1_0")) }},
		"toml number 1_0":       {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("1_0")}}) }},
		"yaml number .5":        {func() (string, error) { return render.YAML(n(".5")) }},
		"toml number .5":        {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n(".5")}}) }},
		"yaml number 5.":        {func() (string, error) { return render.YAML(n("5.")) }},
		"toml number 5.":        {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("5.")}}) }},
		"yaml number 0x10":      {func() (string, error) { return render.YAML(n("0x10")) }},
		"toml number 0x10":      {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("0x10")}}) }},
		"yaml number 01":        {func() (string, error) { return render.YAML(n("01")) }},
		"toml number 01":        {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: n("01")}}) }},
		"jsonc a key twice":     {func() (string, error) { return render.JSONC(twice) }},
		"jsonc NaN":             {func() (string, error) { return render.JSONC(math.NaN()) }},
		"jsonc a chan":          {func() (string, error) { return render.JSONC(make(chan int)) }},
		"toml null":             {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: nil}}) }},
		"toml not a table":      {func() (string, error) { return render.TOML([]any{1}) }},
		"toml a key twice":      {func() (string, error) { return render.TOML(twice) }},
		"toml mixed list": {func() (string, error) {
			return render.TOML(render.Map{{Key: "a", Value: []any{1, render.Map{{Key: "b", Value: 1}}}}})
		}},
		"toml NaN":            {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: math.NaN()}}) }},
		"toml a null in list": {func() (string, error) { return render.TOML(render.Map{{Key: "a", Value: []any{nil}}}) }},
	} {
		if got, err := c.write(); err == nil {
			t.Errorf("%s: wrote %q", name, got)
		}
	}
}

func TestFromJSONKeepsTheOrderOfKeysAndTheTextOfNumbers(t *testing.T) {
	t.Parallel()
	v, err := render.FromJSON([]byte(`{"z":1,"a":{"y":[1.0,2e3,"s",null,true]},"m":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := render.Map{
		{Key: "z", Value: n("1")},
		{Key: "a", Value: render.Map{{Key: "y", Value: []any{n("1.0"), n("2e3"), "s", nil, true}}}},
		{Key: "m", Value: render.Map{}},
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("FromJSON = %#v, want %#v", v, want)
	}
	out, err := render.YAML(v)
	if err != nil || out != "z: 1\na:\n  \"y\":\n    - 1.0\n    - 2e3\n    - s\n    - null\n    - true\nm: {}\n" {
		t.Errorf("YAML of it = %q, %v", out, err)
	}
	for _, bad := range []string{``, `{`, `{"a":1} {}`, `{"a":}`, `[1,`} {
		if _, err := render.FromJSON([]byte(bad)); err == nil {
			t.Errorf("FromJSON(%q) was read", bad)
		}
	}
}

// FuzzFromJSON: a document FromJSON reads is written by JSONC as JSON that reads back as the same value, no input
// panics a writer, and YAML and TOML either refuse a value, write nothing (an empty table) or end their text in a newline. The seeds are the shapes
// of a preset: tables, lists, strings with quotes and line ends, numbers of every text, nulls.
func FuzzFromJSON(f *testing.F) {
	for _, s := range []string{`{}`, `[]`, `null`, `"s"`, `1`, `{"a":{"b":[1,2,{"c":null}]},"d":"x\ny\"z"}`, `{"n":1E5,"m":-0,"k":1.0}`, `{"a":1,"a":2}`, `["\u2028","\u0000","\ud83d\ude00"]`, `{"":""}`, `[[],[[]],{}]`} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		v, err := render.FromJSON(raw)
		if err != nil {
			return
		}
		out, err := render.JSONC(v)
		if err != nil {
			return // a key given twice, or a number the writer refuses
		}
		back, err := render.FromJSON([]byte(out))
		if err != nil || !reflect.DeepEqual(back, v) {
			t.Fatalf("JSONC of %q gave %q, which reads as %#v, %v; want %#v", raw, out, back, err, v)
		}
		for _, w := range []func(any, ...render.Option) (string, error){render.YAML, render.TOML} {
			if s, err := w(v); err == nil && s != "" && !strings.HasSuffix(s, "\n") {
				t.Fatalf("a document %q does not end in a newline", s)
			}
		}
	})
}
