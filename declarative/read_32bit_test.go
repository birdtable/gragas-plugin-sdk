package declarative_test

import (
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// TestALineAtTheEdgeOfAnInt32IsReadAndOnePastItIsLeftOut: a line and a column of 2147483647 are read as they are, and
// 2147483648 is left out (0), in the lines form and in the JSON form that prints them as text, on every width of int. The
// comparison that holds this is written so that it compiles where int is 32 bits, which GOOS=linux GOARCH=386 go vet proves.
// Risk: a constant past 32 bits makes the whole package fail to build for a 32-bit target, a TinyGo plugin's among them.
func TestALineAtTheEdgeOfAnInt32IsReadAndOnePastItIsLeftOut(t *testing.T) {
	t.Parallel()
	lines := model.AdapterOutput{Format: model.OutputLines, OnlyForm: true, Pattern: `^(?P<path>[^:\n]+):(?P<line>\d+):(?P<column>\d+): (?P<key>[a-z]+)$`}
	jsonText := model.AdapterOutput{Format: model.OutputJSON, Each: "/i", Fields: &model.Fields{Key: "/k", Path: "/p", Line: "/l", Column: "/c"}}
	for name, c := range map[string]struct {
		out  model.AdapterOutput
		doc  string
		line int
	}{
		"lines at the edge":       {lines, "a.go:2147483647:2147483647: k\n", 2147483647},
		"lines one past":          {lines, "a.go:2147483648:2147483648: k\n", 0},
		"json text at the edge":   {jsonText, `{"i":[{"k":"k","p":"a.go","l":"2147483647","c":"2147483647"}]}`, 2147483647},
		"json text one past":      {jsonText, `{"i":[{"k":"k","p":"a.go","l":"2147483648","c":"2147483648"}]}`, 0},
		"json number at the edge": {jsonText, `{"i":[{"k":"k","p":"a.go","l":2147483647,"c":2147483647}]}`, 2147483647},
		"json number one past":    {jsonText, `{"i":[{"k":"k","p":"a.go","l":2147483648,"c":2147483648}]}`, 0},
	} {
		got, err := declarative.ReadDiagnostics(c.out, []byte(c.doc))
		if err != nil || len(got) != 1 {
			t.Errorf("%s: %+v, %v", name, got, err)
			continue
		}
		// 2147483647 is the largest int32, so it fits an int of either width; one past it is dropped on both.
		if got[0].Line != c.line || got[0].Column != c.line {
			t.Errorf("%s: line %d, column %d, want %d", name, got[0].Line, got[0].Column, c.line)
		}
	}
}
