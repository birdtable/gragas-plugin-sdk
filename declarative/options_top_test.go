package declarative_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// A files preset that writes an entry's options at the top of its file: an ast-grep rule file holds its
// language beside id, rule and severity, and a placement that names options_top writes each option there by its name.
// The rows hold what the files shape writes and what it refuses: an option on a place the placement writes, two
// bindings of one key that state it otherwise, and two keys that are one file where the file system folds case.

// adapterText is the shipped ast-grep adapter with each passage of olds replaced by its new, in order.
func adapterText(t *testing.T, olds ...string) []byte {
	t.Helper()
	text := string(read(t, astGrep))
	for i := 0; i+1 < len(olds); i += 2 {
		if !strings.Contains(text, olds[i]) {
			t.Fatalf("the ast-grep adapter has no %q", olds[i])
		}
		text = strings.Replace(text, olds[i], olds[i+1], 1)
	}
	return []byte(text)
}

// The passages of the ast-grep adapter where options_top goes: the preset's placement and the files read's entry map.
const (
	placementEnd = "    severity: /severity\n  wiring:"
	entriesEnd   = "      severity: /severity\nrun:"
)

// astGrepTop is ast-grep's adapter with options_top in its placement and in its files read's entry map.
func astGrepTop(t *testing.T) *declarative.Plugin {
	t.Helper()
	p, err := declarative.New(adapterText(t,
		placementEnd, "    severity: /severity\n    options_top: true\n  wiring:",
		entriesEnd, "      severity: /severity\n      options_top: true\nrun:"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func ruleEntry(key, rule, options string) wire.PresetEntry {
	e := wire.PresetEntry{Key: key, Target: "source", Severity: "error", Value: json.RawMessage(`{"pattern":"eval($A)"}`), Rules: []string{rule}}
	if options != "" {
		e.Options = json.RawMessage(options)
	}
	return e
}

func presetOf(t *testing.T, p *declarative.Plugin, entries ...wire.PresetEntry) wire.PresetResponse {
	t.Helper()
	resp, err := p.Preset(wire.PresetRequest{ToolVersion: "0.51.0", Mark: "gragas:preset", Inputs: []wire.File{}, Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// fileText is the text of the preset's file at path, and whether it has one.
func fileText(resp wire.PresetResponse, path string) (string, bool) {
	for _, f := range resp.Files {
		if f.Path == path {
			return f.Text, true
		}
	}
	return "", false
}

// TestAFilesPresetWithOptionsTopWritesEachOptionAtTheTopOfTheEntrysFile: language stands beside id, rule and severity,
// each option under its own name; without options_top the options are left out, as before.
// Risk: an ast-grep rule file is written without its language, which ast-grep does not accept as a rule.
func TestAFilesPresetWithOptionsTopWritesEachOptionAtTheTopOfTheEntrysFile(t *testing.T) {
	t.Parallel()
	entry := ruleEntry("no-eval", "js.a.1", `{"language":"javascript","note":"no eval"}`)
	got, ok := fileText(presetOf(t, astGrepTop(t), entry), "rules/no-eval.yml")
	const want = "# gragas:preset\nid: no-eval\nrule:\n  pattern: \"eval($A)\"\nlanguage: javascript\nnote: no eval\nseverity: error\n"
	if !ok || got != want {
		t.Errorf("rules/no-eval.yml =\n%s\nwant\n%s", got, want)
	}
	got, ok = fileText(presetOf(t, newPlugin(t, astGrep), entry), "rules/no-eval.yml")
	if !ok || strings.Contains(got, "language") || strings.Contains(got, "note") {
		t.Errorf("without options_top, rules/no-eval.yml =\n%s\nwant no options", got)
	}
}

// TestAnOptionWhereThePlacementWritesIsNotWritten: an option named as the first step of the key, value or severity
// pointer would be written over the entry's own part, so the binding is not written and the note names the rule, the
// option and the place, whether or not a severity is written for it.
// Risk: an option replaces the rule's id or pattern, and the file holds a rule the rulebook does not.
func TestAnOptionWhereThePlacementWritesIsNotWritten(t *testing.T) {
	t.Parallel()
	for option, place := range map[string]string{"id": "key", "rule": "value", "severity": "severity"} {
		entry := ruleEntry("no-eval", "js.a.1", `{"language":"javascript","`+option+`":"x"}`)
		for _, sev := range []wire.Severity{"error", ""} {
			entry.Severity = sev
			resp := presetOf(t, astGrepTop(t), entry)
			if _, ok := fileText(resp, "rules/no-eval.yml"); ok {
				t.Errorf("%s, severity %q: the file is written", option, sev)
			}
			want := `js.a.1: ast-grep source no-eval is not written: its option "` + option + `" is where placement.` + place + ` writes`
			if len(resp.Notes) != 1 || resp.Notes[0] != want {
				t.Errorf("%s, severity %q: notes = %q, want %q", option, sev, resp.Notes, want)
			}
		}
	}
}

// TestTwoBindingsOfOneKeyThatStateItOtherwiseAreRefusedNamingBoth: one file per key, so two bindings of a key are one
// file; the first, in rule order, stays written and the second is refused with a note naming both rules and the part
// they state otherwise. Two bindings that state it alike are one entry.
// Risk: one rule's language is written in a file another rule set, and which one wins depends on the order of the request.
func TestTwoBindingsOfOneKeyThatStateItOtherwiseAreRefusedNamingBoth(t *testing.T) {
	t.Parallel()
	resp := presetOf(t, astGrepTop(t),
		ruleEntry("no-eval", "js.b.1", `{"language":"typescript"}`),
		ruleEntry("no-eval", "js.a.1", `{"language":"javascript"}`))
	got, _ := fileText(resp, "rules/no-eval.yml")
	if !strings.Contains(got, "language: javascript\n") || strings.Contains(got, "typescript") {
		t.Errorf("rules/no-eval.yml =\n%s\nwant js.a.1's language alone", got)
	}
	const want = "js.b.1: ast-grep source no-eval is not written: js.a.1 already sets the entry's options otherwise"
	if len(resp.Notes) != 1 || resp.Notes[0] != want {
		t.Errorf("notes = %q, want %q", resp.Notes, want)
	}
	resp = presetOf(t, astGrepTop(t),
		ruleEntry("no-eval", "js.b.1", `{"language":"javascript"}`),
		ruleEntry("no-eval", "js.a.1", `{"language":"javascript"}`))
	if len(resp.Notes) != 0 {
		t.Errorf("two bindings stated alike: notes = %q, want none", resp.Notes)
	}
}

// TestTwoKeysThatAreOneFileWhereCaseFoldsAreRefusedNamingBoth: No-Eval and no-eval are two keys and two paths, and one
// file on a file system that folds case, where the second write would replace the first entry's file whole; the first
// stays written and the second is refused, naming both rules and both keys.
// Risk: on a file system that folds case, one rule's file silently holds another rule, options and all.
func TestTwoKeysThatAreOneFileWhereCaseFoldsAreRefusedNamingBoth(t *testing.T) {
	t.Parallel()
	for name, p := range map[string]*declarative.Plugin{"options_top": astGrepTop(t), "shipped": newPlugin(t, astGrep)} {
		resp := presetOf(t, p,
			ruleEntry("no-eval", "js.a.1", `{"language":"javascript"}`),
			ruleEntry("No-Eval", "js.b.1", `{"language":"typescript"}`))
		var paths []string
		for _, f := range resp.Files {
			paths = append(paths, f.Path)
		}
		if strings.Join(paths, " ") != "sgconfig.yml rules/No-Eval.yml" {
			t.Errorf("%s: files = %v, want sgconfig.yml and the first key's file alone", name, paths)
		}
		const want = "js.a.1: ast-grep source no-eval is not written: its key no-eval and js.b.1's key No-Eval are one file, rules/no-eval.yml, on a file system that folds case"
		if len(resp.Notes) != 1 || resp.Notes[0] != want {
			t.Errorf("%s: notes = %q, want %q", name, resp.Notes, want)
		}
	}
}

// TestAnEntryFilesOptionsAreReadBackFromItsTop: with options_top in the files read's entry map, an entry's options are
// its file's top-level members that the key, value and severity pointers do not name, as the preset wrote them; a file
// with none has no options; without options_top nothing is read as options, as before.
// Risk: a rule's language is written and never read back, so an import or a drift read sees a binding whose options are gone.
func TestAnEntryFilesOptionsAreReadBackFromItsTop(t *testing.T) {
	t.Parallel()
	have := []wire.Result{
		readResult(0, 0, textFile("sgconfig.yml", "ruleDirs:\n  - rules\n")),
		readResult(1, 0, textFile("rules/a.yml", ruleError), textFile("rules/b.yml", "id: bare\nrule:\n  pattern: x\n")),
	}
	got, err := filesEntries(t, astGrepTop(t), have...)
	const want = `{"entries":[` +
		`{"key":"no-eval","on":true,"severity":"error","value":{"pattern":"eval($A)"},"options":{"language":"js"}},` +
		`{"key":"bare","on":true,"severity":"info","value":{"pattern":"x"}}]}`
	if err != nil || got != want {
		t.Errorf("entries =\n%s, %v\nwant\n%s", got, err, want)
	}
	got, err = filesEntries(t, newPlugin(t, astGrep), have...)
	if err != nil || strings.Contains(got, "options") {
		t.Errorf("without options_top: %s, %v, want no options", got, err)
	}
}

// TestOptionsTopIsRefusedWhereNoEntryHasATopOfItsOwn: in a placement, on a list or an object, beside options or
// options_at, or under a key, value or severity pointer whose first step is digits; in an entry map, on a printed
// config's read, beside options, or with an each that is not the whole file.
// Risk: an adapter names options_top where the writer would put the options over another entry, or read another
// entry's members as this one's options.
func TestOptionsTopIsRefusedWhereNoEntryHasATopOfItsOwn(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		olds []string
		want string
	}{
		"beside options": {[]string{placementEnd, "    severity: /severity\n    options: /opts\n    options_top: true\n  wiring:"},
			"preset.placement holds options_top and options or options_at"},
		"a key of digits": {[]string{"    key: /id\n    value: /rule", "    key: /0\n    value: /rule", placementEnd, "    severity: /severity\n    options_top: true\n  wiring:"},
			`preset.placement.key is "/0", whose first step is digits`},
		"not true or false": {[]string{placementEnd, "    severity: /severity\n    options_top: \"yes\"\n  wiring:"},
			"preset.placement.options_top must be true or false"},
		"an entry map beside options": {[]string{entriesEnd, "      severity: /severity\n      options: /opts\n      options_top: true\nrun:"},
			"config[0].entries holds options_top and options, also or as"},
		"an entry map with an each": {[]string{"      each: \"\"\n      key: /id", "      each: /rules\n      key: /id", entriesEnd, "      severity: /severity\n      options_top: true\nrun:"},
			`config[0].entries.options_top is set, and each is "/rules"`},
	} {
		_, err := declarative.New(adapterText(t, c.olds...))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want a refusal saying %q", name, err, c.want)
		}
	}
	for name, c := range map[string]struct{ old, new, want string }{
		"on a list": {"placement:\n", "placement:\n    options_top: true\n",
			"options_top is set, and the shape is list, whose entries share one document"},
		"on a printed config's read": {"    entries:\n", "    entries:\n      options_top: true\n",
			"options_top is set, and the read is of a printed config"},
	} {
		text := string(read(t, "testdata/clean.yaml"))
		if !strings.Contains(text, c.old) {
			t.Fatalf("%s: the clean adapter has no %q", name, c.old)
		}
		_, err := declarative.New([]byte(strings.Replace(text, c.old, c.new, 1)))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want a refusal saying %q", name, err, c.want)
		}
	}
}
