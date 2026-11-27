package plugin_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// TestASamplePluginAnswersEachOfTheFiveOperations: the plugin of sample_test.go, written on Serve, answers plan
// over two rounds, entries, findings, rank and preset. This is the sample plugin passing its own cases natively
// (the ticket's first done-when).
// Risk: the SDK cannot carry a plugin through a whole read.
func TestASamplePluginAnswersEachOfTheFiveOperations(t *testing.T) {
	t.Parallel()
	_, ok, e := call(t, tidy{}, req("plan", `{"for":"run","target":"source","round":0,`+gathered+`}`))
	if e != nil || !strings.Contains(string(ok), `"kind":"read"`) || !strings.Contains(string(ok), `"final":false`) {
		t.Errorf("plan round 0 = %s, %v", ok, e)
	}
	_, ok, e = call(t, tidy{}, req("plan", `{"for":"run","target":"source","round":1,`+gathered+`}`))
	if e != nil || !strings.Contains(string(ok), `"kind":"run"`) || !strings.Contains(string(ok), `"final":true`) {
		t.Errorf("plan round 1 = %s, %v", ok, e)
	}
	_, ok, e = call(t, tidy{}, req("plan", `{"for":"config","target":"source","round":0,`+gathered+`}`))
	if e != nil || string(ok) != `{"steps":[],"final":true}` {
		t.Errorf("a plan for config that needs no step = %s, %v", ok, e)
	}
	_, ok, e = call(t, tidy{}, req("entries", `{"target":"source",`+withStdout("max-len=300\nno-tabs=off\n")+`}`))
	if e != nil || string(ok) != `{"entries":[{"key":"max-len","on":true,"value":300},{"key":"no-tabs","on":false}]}` {
		t.Errorf("entries = %s, %v", ok, e)
	}
	_, ok, e = call(t, tidy{}, req("findings", `{"fixtures":["a.go"],`+withStdout("a.go:3:1: rule-a bad\n")+`}`))
	if e != nil || string(ok) != `{"ran":true,"findings":[{"key":"rule-a","message":"bad","path":"a.go","line":3,"column":1}]}` {
		t.Errorf("findings = %s, %v", ok, e)
	}
	_, ok, e = call(t, tidy{}, req("rank", `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"value","preset":300,"found":500},{"key":"k","aspect":"value","preset":300,"found":100},{"key":"k","aspect":"value","preset":300,"found":300},{"key":"k","aspect":"value","preset":300,"found":"x"}]}`))
	if e != nil || string(ok) != `{"words":["weaker","tighter","same","unknown"]}` {
		t.Errorf("rank = %s, %v", ok, e)
	}
	_, ok, e = call(t, tidy{}, req("preset", `{"tool_version":"1.2.3","mark":"gragas:mark","inputs":[],"entries":[{"key":"r","target":"source","trial":false,"rules":["go.a.1"]}]}`))
	if e != nil || !strings.Contains(string(ok), `"path":"a.tidy"`) {
		t.Errorf("preset = %s, %v", ok, e)
	}
}

// TestAnAnswerLeavesSortedWhateverOrderThePluginGaveIt: one row for each order rule of the plugin contract: entries by
// key as UTF-8 bytes, findings by path, line, column, key and message with an absent field first, and files by
// path. The sample plugin gives its answers out of order.
// Risk: an author gets an order rule wrong and the engine refuses the answer.
func TestAnAnswerLeavesSortedWhateverOrderThePluginGaveIt(t *testing.T) {
	t.Parallel()
	t.Run("entries by key, as bytes", func(t *testing.T) {
		t.Parallel()
		// B (0x42) sorts before a (0x61) as bytes, and e-acute (0xC3) after every ASCII key.
		_, ok, e := call(t, tidy{}, req("entries", `{"target":"source",`+withStdout("a=1\né=1\nB=1\nA=1\n")+`}`))
		if e != nil {
			t.Fatal(e)
		}
		var r wire.EntriesResponse
		if err := json.Unmarshal(ok, &r); err != nil {
			t.Fatal(err)
		}
		var keys []string
		for _, en := range r.Entries {
			keys = append(keys, en.Key)
		}
		if got := strings.Join(keys, ","); got != "A,B,a,é" {
			t.Errorf("keys = %s, want A,B,a,é", got)
		}
	})
	t.Run("findings by path, line, column, key, message, absent first", func(t *testing.T) {
		t.Parallel()
		lines := []string{
			"b.go:1:1: k m",
			"a.go:2:1: k m",
			"a.go:1:2: k m",
			"a.go:1:1: z m",
			"a.go:1:1: k z",
			"a.go:1:1: k a",
			"a.go: k nolinecol",
			": k nopath",
		}
		_, ok, e := call(t, tidy{}, req("findings", `{"fixtures":["a.go"],`+withStdout(strings.Join(lines, "\n"))+`}`))
		if e != nil {
			t.Fatal(e)
		}
		var r wire.FindingsResponse
		if err := json.Unmarshal(ok, &r); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, f := range r.Findings {
			got = append(got, f.Path+"|"+itoa(f.Line)+"|"+itoa(f.Column)+"|"+f.Key+"|"+f.Message)
		}
		want := []string{"|0|0|k|nopath", "a.go|0|0|k|nolinecol", "a.go|1|1|k|a", "a.go|1|1|k|z", "a.go|1|1|z|m", "a.go|1|2|k|m", "a.go|2|1|k|m", "b.go|1|1|k|m"}
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("findings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	})
	t.Run("files by path", func(t *testing.T) {
		t.Parallel()
		_, ok, e := call(t, tidy{}, req("preset", `{"tool_version":"1.2.3","mark":"m","inputs":[],"entries":[]}`))
		if e != nil {
			t.Fatal(e)
		}
		var r wire.PresetResponse
		if err := json.Unmarshal(ok, &r); err != nil || len(r.Files) != 2 || r.Files[0].Path != "a.tidy" || r.Files[1].Path != "z.tidy" {
			t.Errorf("files = %+v, %v", r.Files, err)
		}
	})
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// dup is a plugin whose answers hold a key or a path twice, or words that do not match, or no mark.
type dup struct {
	plugin.Unsupported
	entries  []wire.Entry
	findings []wire.Finding
	words    []wire.Word
	files    []wire.PresetFile
}

func (d dup) Entries(wire.EntriesRequest) (wire.EntriesResponse, error) {
	return wire.EntriesResponse{Entries: d.entries}, nil
}
func (d dup) Findings(wire.FindingsRequest) (wire.FindingsResponse, error) {
	return wire.FindingsResponse{Ran: true, Findings: d.findings}, nil
}
func (d dup) Rank(wire.RankRequest) (wire.RankResponse, error) {
	return wire.RankResponse{Words: d.words}, nil
}
func (d dup) Preset(wire.PresetRequest) (wire.PresetResponse, error) {
	return wire.PresetResponse{Files: d.files}, nil
}

// TestAnAnswerTheEngineWouldRefuseIsABugThatSaysWhy: a key or a path given twice, words that are not as many as
// the pairs, a word or a severity the contract does not have, a mark missing from a file, a plan with neither
// steps nor the last round, and a brace in an argv word each end in a bug that names the fault, never in an
// answer that is dropped or mended.
// Risk: the author learns of the fault as ErrAnswer in a project, or an answer is changed without a word.
func TestAnAnswerTheEngineWouldRefuseIsABugThatSaysWhy(t *testing.T) {
	t.Parallel()
	pairs := `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"value","preset":1,"found":2}]}`
	for name, c := range map[string]struct {
		impl plugin.Plugin
		req  string
		want string
	}{
		"a key twice":           {dup{entries: []wire.Entry{{Key: "k", On: true}, {Key: "k", On: false}}}, req("entries", `{"target":"source",`+gathered+`}`), `the key \"k\" is given twice`},
		"a path twice":          {dup{files: []wire.PresetFile{{Path: "a", Text: "m\n"}, {Path: "a", Text: "m\n"}}}, req("preset", `{"tool_version":"1.2.3","mark":"m","inputs":[],"entries":[]}`), `the path \"a\" is given twice`},
		"too few words":         {dup{}, req("rank", pairs), `0 words for 1 pairs`},
		"a word of its own":     {dup{words: []wire.Word{"better"}}, req("rank", pairs), `the word \"better\"`},
		"a severity of its own": {dup{entries: []wire.Entry{{Key: "k", On: true, Severity: "fatal"}}}, req("entries", `{"target":"source",`+gathered+`}`), `the severity \"fatal\"`},
		"a finding severity":    {dup{findings: []wire.Finding{{Key: "k", Message: "m", Severity: "fatal"}}}, req("findings", `{"fixtures":[],`+gathered+`}`), `the severity \"fatal\"`},
		"no mark":               {dup{files: []wire.PresetFile{{Path: "a", Text: "no mark here\n"}}}, req("preset", `{"tool_version":"1.2.3","mark":"gragas:m","inputs":[],"entries":[]}`), `does not hold the mark`},
		"no file":               {dup{}, req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[]}`), `at least one file`},
		"an empty message":      {dup{findings: []wire.Finding{{Key: "k"}}}, req("findings", `{"fixtures":[],`+gathered+`}`), `has a message`},
	} {
		out, _, e := call(t, c.impl, c.req)
		if e == nil || e.Code != wire.CodeBug || !strings.Contains(string(out), c.want) {
			t.Errorf("%s: answered %s, want a bug saying %s", name, out, c.want)
		}
	}
	// A plan that is neither final nor has a step, and an argv word with a brace of its own.
	for name, c := range map[string]struct {
		resp wire.PlanResponse
		want string
	}{
		"no step, not final":  {wire.PlanResponse{}, `must be final`},
		"a brace":             {wire.PlanResponse{Steps: []wire.Step{{Run: &wire.Run{Program: "p", Argv: []string{"--x={y}"}}}}, Final: true}, `the engine refuses it`},
		"the tokens as words": {wire.PlanResponse{Steps: []wire.Step{{Run: &wire.Run{Program: "p", Argv: []string{"{scratch}", "{null}", "{scratch}/a", "{scratch}/a/b", "--flag", "x}"}}}}, Final: true}, ``},
	} {
		out, _, e := call(t, planner{resp: c.resp}, req("plan", `{"for":"run","target":"source","round":0,`+gathered+`}`))
		switch {
		case c.want == "" && e != nil:
			t.Errorf("%s: %s", name, out)
		case c.want != "" && (e == nil || e.Code != wire.CodeBug || !strings.Contains(string(out), c.want)):
			t.Errorf("%s: answered %s, want a bug saying %s", name, out, c.want)
		}
	}
}

type planner struct {
	plugin.Unsupported
	resp wire.PlanResponse
}

func (p planner) Plan(wire.PlanRequest) (wire.PlanResponse, error) { return p.resp, nil }

// TestARequestTheSDKMeetsThatItDoesNotKnowIsUnsupported: an unknown op, purpose, target and aspect each answer
// unsupported, so does an operation the plugin does not provide, and so does another contract number (C14, C15).
// Risk: a newer engine's request ends in a bug or a trap and not in the answer the engine reads as "not served".
func TestARequestTheSDKMeetsThatItDoesNotKnowIsUnsupported(t *testing.T) {
	t.Parallel()
	for name, r := range map[string]string{
		"an unknown op":                 req("describe", `{}`),
		"an unknown purpose":            req("plan", `{"for":"audit","target":"source","round":0,`+gathered+`}`),
		"an unknown target":             req("plan", `{"for":"run","target":"quantum","round":0,`+gathered+`}`),
		"an unknown target in entries":  req("entries", `{"target":"quantum",`+gathered+`}`),
		"an unknown aspect":             req("rank", `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"colour","preset":1,"found":2}]}`),
		"an unknown target in a preset": req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[{"key":"k","target":"quantum","trial":false,"rules":["r"]}]}`),
		"another contract":              `{"contract":2,"features":[],"op":"plan","body":{}}`,
	} {
		out, _, e := call(t, tidy{}, r)
		if e == nil || e.Code != wire.CodeUnsupported {
			t.Errorf("%s: answered %s, want unsupported", name, out)
		}
	}
	// An operation the plugin does not provide answers unsupported, with no code of the author's.
	for _, op := range wire.Ops() {
		body := map[wire.Op]string{
			wire.OpPlan:     `{"for":"run","target":"source","round":0,` + gathered + `}`,
			wire.OpEntries:  `{"target":"source",` + gathered + `}`,
			wire.OpFindings: `{"fixtures":[],` + gathered + `}`,
			wire.OpRank:     `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"value","preset":1,"found":2}]}`,
			wire.OpPreset:   `{"tool_version":"1.2.3","inputs":[],"entries":[]}`,
		}[op]
		out, _, e := call(t, plugin.Unsupported{}, req(string(op), body))
		if e == nil || e.Code != wire.CodeUnsupported || e.Message == "" {
			t.Errorf("%s of a plugin that provides nothing: %s", op, out)
		}
	}
}

// TestARequestItCannotReadEndsInABugAndNeverInOK: a document that is not JSON, not an object, one with a required
// field missing or null, a body that is not an object, a round out of range and a tool version that is not
// major.minor.patch each end in a bug error, never in ok (C17).
// Risk: a malformed request is answered as if it were read, and the engine takes the answer for the tool's.
func TestARequestItCannotReadEndsInABugAndNeverInOK(t *testing.T) {
	t.Parallel()
	good := `"for":"run","target":"source","round":0,` + gathered
	for name, r := range map[string]string{
		"nothing":                 ``,
		"not json":                `{"contract":`,
		"an array":                `[]`,
		"null":                    `null`,
		"two documents":           req("plan", `{`+good+`}`) + req("plan", `{`+good+`}`),
		"no contract":             `{"features":[],"op":"plan","body":{}}`,
		"no features":             `{"contract":1,"op":"plan","body":{}}`,
		"no op":                   `{"contract":1,"features":[],"body":{}}`,
		"no body":                 `{"contract":1,"features":[],"op":"plan"}`,
		"a body that is a list":   req("plan", `[]`),
		"a null body":             req("plan", `null`),
		"features not strings":    `{"contract":1,"features":[1],"op":"plan","body":{}}`,
		"a contract that is text": `{"contract":"1","features":[],"op":"plan","body":{}}`,
		"no purpose":              req("plan", `{"target":"source","round":0,`+gathered+`}`),
		"no tool version":         req("plan", `{"for":"run","target":"source","round":0,"config":[],"have":[]}`),
		"a null have":             req("plan", `{"for":"run","target":"source","round":0,"tool_version":"1.2.3","config":[],"have":null}`),
		"round 3":                 req("plan", `{"for":"run","target":"source","round":3,`+gathered+`}`),
		"a round that is text":    req("plan", `{"for":"run","target":"source","round":"0",`+gathered+`}`),
		"a version of two parts":  req("entries", `{"target":"source","tool_version":"1.2","config":[],"have":[]}`),
		"no pairs":                req("rank", `{"tool_version":"1.2.3","pairs":[]}`),
		"a pair with no found":    req("rank", `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"value","preset":1}]}`),
		"an entry with no rules":  req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[{"key":"k","target":"source","trial":false,"rules":[]}]}`),
		"an entry with no trial":  req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[{"key":"k","target":"source","rules":["r"]}]}`),
	} {
		out, ok, e := call(t, tidy{}, r)
		if ok != nil || e == nil || e.Code != wire.CodeBug {
			t.Errorf("%s: answered %s, want a bug", name, out)
		}
	}
}

// TestAFieldItDoesNotKnowIsIgnored: the same request with one unknown field added, in the envelope, in the body
// and in an item, gives the same answer to the byte (C12).
// Risk: a newer engine's optional request field makes an older plugin refuse the call.
func TestAFieldItDoesNotKnowIsIgnored(t *testing.T) {
	t.Parallel()
	body := `{"target":"source",` + withStdout("max-len=300\n") + `}`
	base, _, _ := call(t, tidy{}, req("entries", body))
	for name, r := range map[string]string{
		"in the envelope": `{"contract":1,"features":[],"op":"entries","new":true,"body":` + body + `}`,
		"in the body":     req("entries", strings.Replace(body, `{"target"`, `{"newer":[1],"target"`, 1)),
		"in an item":      req("entries", strings.Replace(body, `"round":1`, `"round":1,"note":"x"`, 1)),
	} {
		out, _, e := call(t, tidy{}, r)
		if e != nil || !bytes.Equal(out, base) {
			t.Errorf("%s: answered %s, want %s", name, out, base)
		}
	}
}

// TestTheSameRequestGivesTheSameBytes: a second call gives the same answer to the byte, and an answer is one
// JSON document and one newline (C8, C11).
// Risk: an answer that depends on map order, a clock or an earlier call.
func TestTheSameRequestGivesTheSameBytes(t *testing.T) {
	t.Parallel()
	r := req("entries", `{"target":"source",`+withStdout("b=1\na=2\nc=off\n")+`}`)
	first, _, _ := call(t, tidy{}, r)
	for range 20 {
		again, _, _ := call(t, tidy{}, r)
		if !bytes.Equal(first, again) {
			t.Fatalf("a second call gave %s, the first %s", again, first)
		}
	}
	if !bytes.HasSuffix(first, []byte("}\n")) || bytes.Count(first, []byte("\n")) != 1 {
		t.Errorf("the answer %q is not one document and one newline", first)
	}
}

// TestAnErrorCodeIsKeptWhereItIsValidAndABugWhereItIsNot: unreadable stands in entries, in findings and in plan
// after round 0, refused in plan and in preset, and the other combinations are turned into a bug that says so,
// as the engine would refuse them.
// Risk: the engine reads an answer the plugin's author thought valid as ErrAnswer.
func TestAnErrorCodeIsKeptWhereItIsValidAndABugWhereItIsNot(t *testing.T) {
	t.Parallel()
	failing := func(code wire.Code) plugin.Plugin {
		return erring{err: &wire.Error{Code: code, Message: "no", Path: "/x"}}
	}
	ent := req("entries", `{"target":"source",`+gathered+`}`)
	fnd := req("findings", `{"fixtures":[],`+gathered+`}`)
	plan := func(round string) string {
		return req("plan", `{"for":"run","target":"source","round":`+round+`,`+gathered+`}`)
	}
	rnk := req("rank", `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"value","preset":1,"found":2}]}`)
	pre := req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[]}`)
	for name, c := range map[string]struct {
		code wire.Code
		req  string
		keep bool
	}{
		"unreadable in entries":       {wire.CodeUnreadable, ent, true},
		"unreadable in findings":      {wire.CodeUnreadable, fnd, true},
		"unreadable in plan, round 1": {wire.CodeUnreadable, plan("1"), true},
		"unreadable in plan, round 0": {wire.CodeUnreadable, plan("0"), false},
		"unreadable in rank":          {wire.CodeUnreadable, rnk, false},
		"unreadable in preset":        {wire.CodeUnreadable, pre, false},
		"refused in plan":             {wire.CodeRefused, plan("0"), true},
		"refused in preset":           {wire.CodeRefused, pre, true},
		"refused in entries":          {wire.CodeRefused, ent, false},
		"refused in rank":             {wire.CodeRefused, rnk, false},
		"unsupported in rank":         {wire.CodeUnsupported, rnk, true},
		"bug in preset":               {wire.CodeBug, pre, true},
		"a code of its own":           {"nope", ent, false},
	} {
		out, _, e := call(t, failing(c.code), c.req)
		switch {
		case e == nil:
			t.Errorf("%s: answered %s", name, out)
		case c.keep && (e.Code != c.code || e.Path != "/x"):
			t.Errorf("%s: answered %s, want the code and path kept", name, out)
		case !c.keep && (e.Code != wire.CodeBug || !strings.Contains(e.Message, "not valid there")):
			t.Errorf("%s: answered %s, want a bug that says it is not valid there", name, out)
		}
	}
	// An error of Go's own is a bug, and a panic is a bug that names the operation.
	out, _, e := call(t, erring{err: errors.New("the tool exploded")}, ent)
	if e == nil || e.Code != wire.CodeBug || !strings.Contains(e.Message, "the tool exploded") {
		t.Errorf("a plain error: %s", out)
	}
	out, _, e = call(t, erring{panics: true}, ent)
	if e == nil || e.Code != wire.CodeBug || !strings.Contains(e.Message, "panicked in entries") {
		t.Errorf("a panic: %s", out)
	}
}

// erring answers every operation with err, or panics.
type erring struct {
	err    error
	panics bool
}

func (e erring) fail() error {
	if e.panics {
		panic("boom")
	}
	return e.err
}
func (e erring) Plan(wire.PlanRequest) (wire.PlanResponse, error) {
	return wire.PlanResponse{}, e.fail()
}
func (e erring) Entries(wire.EntriesRequest) (wire.EntriesResponse, error) {
	return wire.EntriesResponse{}, e.fail()
}
func (e erring) Findings(wire.FindingsRequest) (wire.FindingsResponse, error) {
	return wire.FindingsResponse{}, e.fail()
}
func (e erring) Rank(wire.RankRequest) (wire.RankResponse, error) {
	return wire.RankResponse{}, e.fail()
}
func (e erring) Preset(wire.PresetRequest) (wire.PresetResponse, error) {
	return wire.PresetResponse{}, e.fail()
}

// TestANullValueAndAnAbsentValueReachThePluginApart: in a value, null differs from an absent value,
// so a pair whose preset is null reaches the plugin as "null" and one with no preset as nothing.
// Risk: a tool that holds null as a value is read as one that holds none.
func TestANullValueAndAnAbsentValueReachThePluginApart(t *testing.T) {
	t.Parallel()
	var seen []string
	p := ranker{seen: &seen}
	_, _, e := call(t, p, req("rank", `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"value","preset":null,"found":[1]}]}`))
	if e != nil || len(seen) != 1 || seen[0] != "null|[1]" {
		t.Errorf("a null preset reached the plugin as %v, %v", seen, e)
	}
	out, _, e := call(t, p, req("rank", `{"tool_version":"1.2.3","pairs":[{"key":"k","aspect":"value","found":[1]}]}`))
	if e == nil || e.Code != wire.CodeBug {
		t.Errorf("a pair with no preset was answered %s, want a bug", out)
	}
}

type ranker struct {
	plugin.Unsupported
	seen *[]string
}

func (r ranker) Rank(q wire.RankRequest) (wire.RankResponse, error) {
	var out wire.RankResponse
	for _, p := range q.Pairs {
		*r.seen = append(*r.seen, string(p.Preset)+"|"+string(p.Found))
		out.Words = append(out.Words, wire.Same)
	}
	return out, nil
}

// TestAnAnswerOverTheCapIsABug: a response over 16 MiB is answered as a bug, never written whole.
// Risk: the engine meets ErrLimit for a plugin's own fault.
func TestAnAnswerOverTheCapIsABug(t *testing.T) {
	t.Parallel()
	big := dup{files: []wire.PresetFile{{Path: "a", Text: "m\n" + strings.Repeat("x", 17<<20)}}}
	out, _, e := call(t, big, req("preset", `{"tool_version":"1.2.3","mark":"m","inputs":[],"entries":[]}`))
	if e == nil || e.Code != wire.CodeBug || !strings.Contains(string(out), "16 MiB") || len(out) > 4096 {
		t.Errorf("answered %d bytes: %.200s", len(out), out)
	}
}

// TestARequestThatCannotBeReadAtAllIsAnsweredAndAFailedWriteIsExitOne: ServeIO answers a reader that fails with a
// bug, and returns 1 when the answer cannot be written.
func TestARequestThatCannotBeReadAtAllIsAnsweredAndAFailedWriteIsExitOne(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if code := plugin.ServeIO(failingReader{}, &buf, tidy{}); code != 0 || !strings.Contains(buf.String(), `"bug"`) {
		t.Errorf("a reader that fails: exit %d, %s", code, buf.String())
	}
	if code := plugin.ServeIO(strings.NewReader(`{}`), failingWriter{}, tidy{}); code != 1 {
		t.Errorf("a writer that fails: exit %d, want 1", code)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestAPlaceholderTokenIsAWholeWordOrTheStartOfOne: the engine takes the word {scratch}, the word {null} and a word that
// starts with {scratch}/, and refuses any other {, so the SDK refuses each of these in the plugin's own test, as the
// engine's wordFault does.
// Risk: a plugin passes its tests on the SDK and ends in ErrAnswer in a project.
func TestAPlaceholderTokenIsAWholeWordOrTheStartOfOne(t *testing.T) {
	t.Parallel()
	for _, w := range []string{"--out={scratch}/a", "--null={null}", "{scratch}x", "{null}x", "{scratch}/{scratch}", "{scratch}/{null}", "a{scratch}", "{"} {
		plan := wire.PlanResponse{Steps: []wire.Step{{Run: &wire.Run{Program: "p", Argv: []string{w}}}}, Final: true}
		out, _, e := call(t, planner{resp: plan}, req("plan", `{"for":"run","target":"source","round":0,`+gathered+`}`))
		if e == nil || e.Code != wire.CodeBug || e.Path != "/steps/0/argv/0" {
			t.Errorf("%q: answered %s, want a bug at /steps/0/argv/0", w, out)
		}
	}
}

// TestAStringThatIsNotUTF8IsABugAndNeverAnotherStringWithOK: a key, a finding's path and a preset's text that are not
// UTF-8 each answer a bug at their pointer; JSON would write U+FFFD for each bad byte and answer ok, with two keys that
// are one, a key out of order, or a file's text not byte for byte.
// Risk: the answer the engine reads is not the answer the plugin gave, and no refusal says so.
func TestAStringThatIsNotUTF8IsABugAndNeverAnotherStringWithOK(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		impl plugin.Plugin
		req  string
		at   string
	}{
		"an entry key":      {dup{entries: []wire.Entry{{Key: "a\xff", On: true}}}, req("entries", `{"target":"source",`+gathered+`}`), "/entries/0/key"},
		"a finding path":    {dup{findings: []wire.Finding{{Key: "k", Message: "m", Path: "a\xffb"}}}, req("findings", `{"fixtures":[],`+gathered+`}`), "/findings/0/path"},
		"a preset text":     {dup{files: []wire.PresetFile{{Path: "a", Text: "a\xffb\n"}}}, req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[]}`), "/files/0/text"},
		"a finding message": {dup{findings: []wire.Finding{{Key: "k", Message: "m\xc3"}}}, req("findings", `{"fixtures":[],`+gathered+`}`), "/findings/0/message"},
	} {
		out, _, e := call(t, c.impl, c.req)
		if e == nil || e.Code != wire.CodeBug || e.Path != c.at {
			t.Errorf("%s: answered %s, want a bug at %s", name, out, c.at)
		}
	}
}

// TestAnEntrysValueAndOptionsAreHeldTo64KiBAnd32Levels: a value or options of 64 KiB and of 32 levels leave; one byte
// over and one level over are each a bug at the entry, as the engine's valueFault has it.
// Risk: a plugin whose entry the engine refuses passes its tests on the SDK.
func TestAnEntrysValueAndOptionsAreHeldTo64KiBAnd32Levels(t *testing.T) {
	t.Parallel()
	text := func(n int) wire.Value { return wire.Value(`"` + strings.Repeat("a", n-2) + `"`) }
	nest := func(n int) wire.Value { return wire.Value(strings.Repeat("[", n) + strings.Repeat("]", n)) }
	try := func(e wire.Entry) *wire.Error {
		_, _, err := call(t, dup{entries: []wire.Entry{e}}, req("entries", `{"target":"source",`+gathered+`}`))
		return err
	}
	for name, e := range map[string]wire.Entry{
		"a value of 64 KiB": {Key: "k", Value: text(65536)}, "options of 64 KiB": {Key: "k", Options: text(65536)},
		"a value of 32 levels": {Key: "k", Value: nest(32)}, "options of 32 levels": {Key: "k", Options: nest(32)},
		"brackets in a string": {Key: "k", Value: wire.Value(`"` + strings.Repeat("[", 100) + `"`)},
	} {
		if err := try(e); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
	for name, c := range map[string]struct {
		e  wire.Entry
		at string
	}{
		"a value of 64 KiB and a byte": {wire.Entry{Key: "k", Value: text(65537)}, "/entries/0/value"},
		"options of 64 KiB and a byte": {wire.Entry{Key: "k", Options: text(65537)}, "/entries/0/options"},
		"a value of 33 levels":         {wire.Entry{Key: "k", Value: nest(33)}, "/entries/0/value"},
		"options of 33 levels":         {wire.Entry{Key: "k", Options: nest(33)}, "/entries/0/options"},
	} {
		if err := try(c.e); err == nil || err.Code != wire.CodeBug || err.Path != c.at {
			t.Errorf("%s: answered %+v, want a bug at %s", name, err, c.at)
		}
	}
}

// TestAnErrorsPathIsAPointerAndTheDisplayFieldsAreCutOnACharacter: a path that is not a JSON pointer (entries/3, /a~2, /a~)
// is a bug that says so, a path over 1024 bytes is cut at a token and never inside ~1, and a finding's message, url and why,
// and a note, are cut at the schema's lengths on a character, so a tool's long line is not a bug.
// Risk: the author's own refusal is lost behind ErrAnswer in a project, or a long message makes the whole answer a bug.
func TestAnErrorsPathIsAPointerAndTheDisplayFieldsAreCutOnACharacter(t *testing.T) {
	t.Parallel()
	bad := func(path string) *wire.Error {
		_, _, e := call(t, errs{err: plugin.Refusedf(path, "no")}, req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[]}`))
		return e
	}
	for _, p := range []string{"entries/3", "/a~2", "/a~"} {
		if e := bad(p); e == nil || e.Code != wire.CodeBug || !strings.Contains(e.Message, "not a JSON pointer") {
			t.Errorf("path %q: %+v, want a bug naming the pointer", p, e)
		}
	}
	if e := bad("/entries/0/options"); e == nil || e.Code != wire.CodeRefused || e.Path != "/entries/0/options" {
		t.Errorf("a pointer: %+v, want refused at it", e)
	}
	long := strings.Repeat("/abcdefgh~1xy", 120) // a cut at 1024 bytes lands just after a ~
	if e := bad(long); e == nil || e.Code != wire.CodeRefused || len(e.Path) == 0 || len(e.Path) > 1024 || strings.HasSuffix(e.Path, "~") || !strings.HasPrefix(long, e.Path) {
		t.Errorf("a long path: %d bytes, %+v", len(long), e)
	}
	msg := strings.Repeat("é", 5000)
	out, ok, e := call(t, dup{findings: []wire.Finding{{Key: "k", Message: msg, URL: strings.Repeat("u", 3000)}}}, req("findings", `{"fixtures":[],`+gathered+`}`))
	var got wire.FindingsResponse
	if e != nil || json.Unmarshal(ok, &got) != nil || len(got.Findings[0].Message) > 8192 || !utf8.ValidString(got.Findings[0].Message) || len(got.Findings[0].URL) != 2048 {
		t.Errorf("long display fields: %s", out)
	}
}

type errs struct {
	plugin.Unsupported
	err error
}

func (e errs) Preset(wire.PresetRequest) (wire.PresetResponse, error) {
	return wire.PresetResponse{}, e.err
}

// TestTheSchemasLengthsAreHeldInTheCode: at the cap an answer leaves and one past it is a bug at its pointer, for the 300
// argv words, the 20 files a run places, the 20 paths it collects, a glob of 2000 bytes and a finding's line and column up to
// 4294967295; a finding's why (2048) and a preset's notes (1024) are cut on a character and leave ok.
// Risk: a plugin passes its tests on the SDK and ends in ErrAnswer in a project, which is what the SDK is there to prevent.
func TestTheSchemasLengthsAreHeldInTheCode(t *testing.T) {
	t.Parallel()
	plan := func(r wire.Run, g string) *wire.Error {
		steps := []wire.Step{{Run: &r}}
		if g != "" {
			steps = []wire.Step{{Read: &wire.Read{Glob: g}}}
		}
		_, _, e := call(t, planner{resp: wire.PlanResponse{Steps: steps, Final: true}}, req("plan", `{"for":"run","target":"source","round":0,`+gathered+`}`))
		return e
	}
	words := func(n int) []string { return slices.Repeat([]string{"x"}, n) }
	files := func(n int) []wire.File {
		return slices.Repeat([]wire.File{{Path: "a", Data: wire.Bytes{Data: []byte("x")}}}, n)
	}
	for name, c := range map[string]struct {
		ok, over *wire.Error
		at       string
	}{
		"argv words":   {plan(wire.Run{Program: "p", Argv: words(300)}, ""), plan(wire.Run{Program: "p", Argv: words(301)}, ""), "/steps/0/argv"},
		"placed files": {plan(wire.Run{Program: "p", Argv: words(1), Place: files(20)}, ""), plan(wire.Run{Program: "p", Argv: words(1), Place: files(21)}, ""), "/steps/0"},
		"collected":    {plan(wire.Run{Program: "p", Argv: words(1), Collect: words(20)}, ""), plan(wire.Run{Program: "p", Argv: words(1), Collect: words(21)}, ""), "/steps/0"},
		"a glob":       {plan(wire.Run{}, strings.Repeat("a", 2000)), plan(wire.Run{}, strings.Repeat("a", 2001)), "/steps/0/glob"},
	} {
		if c.ok != nil {
			t.Errorf("%s: at the cap, refused: %+v", name, c.ok)
		}
		if c.over == nil || c.over.Code != wire.CodeBug || c.over.Path != c.at {
			t.Errorf("%s: one past the cap, answered %+v, want a bug at %s", name, c.over, c.at)
		}
	}
	// The values are int64 and become an int only where an int holds them: on a 32-bit target none can be past the cap,
	// and the rows that need one are skipped, with no build tag.
	const top int64 = 4294967295
	fits := func(vs ...int64) bool {
		for _, v := range vs {
			if int64(int(v)) != v {
				return false
			}
		}
		return true
	}
	finding := func(line, col int64) *wire.Error {
		_, _, e := call(t, dup{findings: []wire.Finding{{Key: "k", Message: "m", Line: int(line), Column: int(col)}}}, req("findings", `{"fixtures":[],`+gathered+`}`))
		return e
	}
	if fits(top) {
		if e := finding(top, top); e != nil {
			t.Errorf("a line and column of 4294967295: %+v", e)
		}
	}
	if fits(top + 1) {
		for name, e := range map[string]*wire.Error{"a line": finding(top+1, 1), "a column": finding(1, top+1)} {
			if e == nil || e.Code != wire.CodeBug || e.Path != "/findings/0" {
				t.Errorf("%s past 4294967295: %+v, want a bug at /findings/0", name, e)
			}
		}
	}
	_, ok, e := call(t, whyNotes{}, req("findings", `{"fixtures":[],`+gathered+`}`))
	var f wire.FindingsResponse
	if e != nil || json.Unmarshal(ok, &f) != nil || len(f.Why) != 2048 || !utf8.ValidString(f.Why) {
		t.Errorf("a long why: %d bytes, %+v", len(f.Why), e)
	}
	_, ok, e = call(t, whyNotes{}, req("preset", `{"tool_version":"1.2.3","inputs":[],"entries":[]}`))
	var p wire.PresetResponse
	if e != nil || json.Unmarshal(ok, &p) != nil || len(p.Notes) != 1 || len(p.Notes[0]) != 1024 {
		t.Errorf("a long note: %+v, %+v", p.Notes, e)
	}
}

// whyNotes answers a why of 3000 bytes and a note of 3000 bytes, which the SDK cuts at the schema's lengths.
type whyNotes struct{ plugin.Unsupported }

func (whyNotes) Findings(wire.FindingsRequest) (wire.FindingsResponse, error) {
	return wire.FindingsResponse{Ran: false, Why: strings.Repeat("é", 1500), Findings: []wire.Finding{}}, nil
}

func (whyNotes) Preset(wire.PresetRequest) (wire.PresetResponse, error) {
	return wire.PresetResponse{Files: []wire.PresetFile{{Path: "a", Text: "x\n"}}, Notes: []string{strings.Repeat("a", 3000)}}, nil
}
