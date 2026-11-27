package lines_test

import (
	"math"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/lines"
)

func TestAPatternReadsTheFieldsOfEachLineItMatches(t *testing.T) {
	t.Parallel()
	p, err := lines.Compile("{path}:{line}:{column}: {severity}: {message} ({key})")
	if err != nil {
		t.Fatal(err)
	}
	text := "a/b.go:12:3: error: unused variable x (no-unused)\r\nsummary: 1 problem\n\nc.go:1:1: warning: a (b) c (rule-2)\nd.go:4294967296:0: error: huge (k)\n"
	got := p.Parse(text)
	if len(got) != 3 {
		t.Fatalf("matched %d lines: %+v", len(got), got)
	}
	want := []lines.Match{
		{Raw: "a/b.go:12:3: error: unused variable x (no-unused)", Path: "a/b.go", Line: 12, Column: 3, Severity: "error", Message: "unused variable x", Key: "no-unused"},
		{Raw: "c.go:1:1: warning: a (b) c (rule-2)", Path: "c.go", Line: 1, Column: 1, Severity: "warning", Message: "a (b) c", Key: "rule-2"},
		{Raw: "d.go:4294967296:0: error: huge (k)", Path: "d.go", Severity: "error", Message: "huge", Key: "k"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("match %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestAPatternWithFewerFieldsAndBraceEscapes(t *testing.T) {
	t.Parallel()
	p := lines.MustCompile("{{{key}}}: {message}")
	got := p.Parse("{no-x}: bad thing\n{}: nope\nplain: no\n")
	if len(got) != 1 || got[0].Key != "no-x" || got[0].Message != "bad thing" {
		t.Errorf("matches = %+v", got)
	}
	if got := lines.MustCompile("{path}").Parse("a\nb\n"); len(got) != 2 || got[0].Path != "a" || got[1].Path != "b" {
		t.Errorf("a path alone = %+v", got)
	}
}

func TestAPatternThatIsNotOneIsAnError(t *testing.T) {
	t.Parallel()
	for name, bad := range map[string]string{
		"an unknown field": "{file}: {message}",
		"an unclosed {":    "{path",
		"a lone }":         "{path} }",
		"a field twice":    "{path}:{path}",
		"no field":         "just text",
		"empty":            "",
	} {
		if _, err := lines.Compile(bad); err == nil {
			t.Errorf("%s: %q compiled", name, bad)
		}
	}
	defer func() {
		if recover() == nil {
			t.Error("MustCompile of a bad pattern did not panic")
		}
	}()
	lines.MustCompile("{nope}")
}

// FuzzParse: no pattern and no text panic, each match is a line of the text, and a line or column is within the
// contract's range. The seeds are the patterns and outputs of the common shapes of a linter's lines.
func FuzzParse(f *testing.F) {
	pats := []string{"{path}:{line}:{column}: {message} ({key})", "{path}:{line}: {severity}: {message}", "{key}: {message}", "{{{key}}} {path}", "{path}"}
	outs := []string{"", "a.go:1:2: bad (k)", "a.go:1: warning: bad\r\nb", "k: m\n\n\n", "{k} p", "\x00:1:1: x (y)", "a.go:99999999999:1: m (k)", "a:1:1: é (\U0001F600)"}
	for _, p := range pats {
		for _, o := range outs {
			f.Add(p, o)
		}
	}
	f.Fuzz(func(t *testing.T, pattern, text string) {
		p, err := lines.Compile(pattern)
		if err != nil {
			return
		}
		for _, m := range p.Parse(text) {
			if !strings.Contains(text, m.Raw) {
				t.Fatalf("the match %q is not in the text", m.Raw)
			}
			if m.Line < 0 || int64(m.Line) > math.MaxUint32 || m.Column < 0 || int64(m.Column) > math.MaxUint32 {
				t.Fatalf("line %d, column %d are out of range", m.Line, m.Column)
			}
		}
	})
}

// TestANumberBetween2To31And2To32IsReadWhereAnIntHoldsItAndDroppedWhereItDoesNot: a line and a column of 3000000000 are within the
// contract's 4294967295, so they read as 3000000000 where an int holds them (64 bits) and as 0, which leaves them out,
// where it does not (32 bits), never as a negative number, which would refuse the whole answer of the plugin.
// Risk: on a 32-bit target int(n) wraps, finishFindings refuses the answer, and one odd line costs every finding.
func TestANumberBetween2To31And2To32IsReadWhereAnIntHoldsItAndDroppedWhereItDoesNot(t *testing.T) {
	t.Parallel()
	p, err := lines.Compile("{path}:{line}:{column}: {message} ({key})")
	if err != nil {
		t.Fatal(err)
	}
	got := p.Parse("a.go:3000000000:3000000000: m (k)\n")
	if len(got) != 1 {
		t.Fatalf("matched %d lines: %+v", len(got), got)
	}
	n := int64(3000000000)
	want := 0
	if int64(int(n)) == n {
		want = int(n)
	}
	if got[0].Line != want || got[0].Column != want {
		t.Errorf("line %d, column %d, want %d, and never a negative number", got[0].Line, got[0].Column, want)
	}
}
