package sarif_test

import (
	"math"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/sarif"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

const log = `{"version":"2.1.0","runs":[{
 "tool":{"driver":{"rules":[
  {"id":"R1","helpUri":"https://x.example/r1","defaultConfiguration":{"level":"error"}},
  {"id":"R2","defaultConfiguration":{"level":"note"}}]}},
 "results":[
  {"ruleId":"R1","message":{"text":"bad one"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"file:///tmp/s1/src/a%20b.go"},"region":{"startLine":3,"startColumn":7}}}]},
  {"ruleIndex":1,"message":{"markdown":"**note** two"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/c.go"}}}]},
  {"ruleId":"R3","level":"none","message":{"text":"three"}},
  {"ruleId":"R1","level":"warning","message":{"text":"four"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"d.go"},"region":{"startLine":0,"startColumn":4294967296}}}]}
 ]}]}`

func TestAResultBecomesAFindingByTheMappingInThePackageDoc(t *testing.T) {
	t.Parallel()
	l, err := sarif.Parse([]byte(log))
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.Findings("/tmp/s1")
	if err != nil {
		t.Fatal(err)
	}
	want := []wire.Finding{
		{Key: "R1", Message: "bad one", Path: "src/a b.go", Line: 3, Column: 7, Severity: wire.SeverityError, URL: "https://x.example/r1"},
		{Key: "R2", Message: "**note** two", Path: "src/c.go", Severity: wire.SeverityInfo},
		{Key: "R3", Message: "three", Severity: wire.SeverityInfo},
		{Key: "R1", Message: "four", Path: "d.go", Severity: wire.SeverityWarning, URL: "https://x.example/r1"},
	}
	if len(got) != len(want) {
		t.Fatalf("%d findings: %+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("finding %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestADefaultLevelIsWarningAndAResultWithNoRuleOrMessageIsNamed(t *testing.T) {
	t.Parallel()
	l, err := sarif.Parse([]byte(`{"version":"2.1.0","runs":[{"tool":{"driver":{}},"results":[{"ruleId":"X","message":{"text":"m"}}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l.Findings(""); err != nil || len(got) != 1 || got[0].Severity != wire.SeverityWarning {
		t.Errorf("findings = %+v, %v", got, err)
	}
	for name, c := range map[string]struct{ in, at string }{
		"no rule id": {`{"version":"2.1.0","runs":[{"results":[{"message":{"text":"m"}}]}]}`, "/runs/0/results/0 has no rule id"},
		"no message": {`{"version":"2.1.0","runs":[{"results":[{"ruleId":"a","message":{"text":"m"}},{"ruleId":"b"}]}]}`, "/runs/0/results/1 has no message"},
	} {
		l, err := sarif.Parse([]byte(c.in))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := l.Findings(""); err == nil || !strings.Contains(err.Error(), c.at) {
			t.Errorf("%s: %v, want it to say %q", name, err, c.at)
		}
	}
}

func TestADocumentThatIsNotSARIFIsAnError(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]string{
		"not json":   `{"version":`,
		"a list":     `[]`,
		"no version": `{"runs":[]}`,
		"an old one": `{"version":"2.0.0","runs":[]}`,
		"no runs":    `{"version":"2.1.0"}`,
		"null runs":  `{"version":"2.1.0","runs":null}`,
	} {
		if _, err := sarif.Parse([]byte(in)); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	if l, err := sarif.Parse([]byte(`{"version":"2.1.0","runs":[]}`)); err != nil || len(l.Runs) != 0 {
		t.Errorf("a log with no run: %+v, %v", l, err)
	}
}

func TestAUriBecomesAProjectPath(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ uri, scratch, want string }{
		{"file:///tmp/s1/a.go", "/tmp/s1", "a.go"},
		{"file:///tmp/s1/a%20b.go", "/tmp/s1", "a b.go"},
		{"file:///C:/s/a.go", "C:/s", "a.go"},
		{"file://host/a.go", "", "/a.go"},
		{"a/b.go", "/tmp/s1", "a/b.go"},
		{"a%2Fb.go", "", "a/b.go"},
		{"a%zz.go", "", "a%zz.go"},
		{"", "/tmp/s1", ""},
	} {
		if got := sarif.Path(c.uri, c.scratch); got != c.want {
			t.Errorf("Path(%q, %q) = %q, want %q", c.uri, c.scratch, got, c.want)
		}
	}
}

// FuzzParse: no document panics Parse or Findings, and each finding is as the contract holds it: a key of 1 to 512
// bytes, a message of 1 to 8192, a line and a column within range, a severity of the three words. The seeds are
// the log above, the smallest logs and the shapes that break a reader.
func FuzzParse(f *testing.F) {
	for _, s := range []string{log, `{"version":"2.1.0","runs":[]}`, `{"version":"2.1.0","runs":[{}]}`, `{"version":"2.1.0","runs":[{"results":[{"ruleId":"a","message":{"text":"m"},"ruleIndex":-1}]}]}`,
		`{"version":"2.1.0","runs":[{"tool":{"driver":{"rules":[{}]}},"results":[{"ruleIndex":0,"message":{"text":"m"},"locations":[{}]}]}]}`, `[]`, ``, `null`, "{\"version\":\"2.1.0\",\"runs\":[{\"results\":[{\"ruleId\":\"\\u00e9\",\"message\":{\"text\":\"\\ud800\"}}]}]}"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		l, err := sarif.Parse(data)
		if err != nil {
			return
		}
		got, err := l.Findings("/tmp/s1")
		if err != nil {
			return
		}
		for _, x := range got {
			switch {
			case x.Key == "" || len(x.Key) > 512, x.Message == "" || len(x.Message) > 8192:
				t.Fatalf("a finding with key %d bytes, message %d bytes", len(x.Key), len(x.Message))
			case x.Line < 0 || int64(x.Line) > math.MaxUint32 || x.Column < 0 || int64(x.Column) > math.MaxUint32:
				t.Fatalf("line %d, column %d are out of range", x.Line, x.Column)
			case !slicesContains(wire.Severities(), x.Severity):
				t.Fatalf("the severity %q", x.Severity)
			}
		}
	})
}

func slicesContains(s []wire.Severity, v wire.Severity) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestALineBetween2To31And2To32IsReadWhereAnIntHoldsItAndDroppedWhereItDoesNot: a startLine and a startColumn of 3000000000
// are within the contract's 4294967295, so they read as 3000000000 where an int holds them (64 bits) and as absent where it
// does not (32 bits), never as a negative number, which would refuse the whole answer of the plugin.
// Risk: on a 32-bit target int(n) wraps, finishFindings refuses the answer, and one odd line costs every finding.
func TestALineBetween2To31And2To32IsReadWhereAnIntHoldsItAndDroppedWhereItDoesNot(t *testing.T) {
	t.Parallel()
	n := int64(3000000000)
	l, err := sarif.Parse([]byte(`{"version":"2.1.0","runs":[{"results":[{"ruleId":"R","message":{"text":"m"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"a.go"},"region":{"startLine":3000000000,"startColumn":3000000000}}}]}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.Findings("")
	if err != nil || len(got) != 1 {
		t.Fatalf("findings = %+v, %v", got, err)
	}
	want := 0
	if int64(int(n)) == n {
		want = int(n)
	}
	if got[0].Line != want || got[0].Column != want {
		t.Errorf("line %d, column %d, want %d, and never a negative number", got[0].Line, got[0].Column, want)
	}
}
