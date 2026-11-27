package declarative_test

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// The rounds of the plugin contract and the rows of gragas's contracttest (plan 01 to 05), asked of a plugin that reads a tool's config by its
// files: ast-grep's adapter, which prints no effective config and names its rule folders in sgconfig.yml.

const astGrep = "testdata/adapters/ast-grep/1.1.0.yaml"

func readResult(round, step int, files ...wire.File) wire.Result {
	return wire.Result{Round: round, Step: step, Kind: wire.KindRead, Files: append([]wire.File{}, files...)}
}

func textFile(path, text string) wire.File {
	return wire.File{Path: path, Data: wire.Bytes{Data: []byte(text)}}
}

func configPlan(t *testing.T, p *declarative.Plugin, round int, cfg []wire.Config, have ...wire.Result) (string, error) {
	t.Helper()
	got, err := p.Plan(wire.PlanRequest{Gathered: wire.Gathered{ToolVersion: "0.51.0", Config: append([]wire.Config{}, cfg...), Have: append([]wire.Result{}, have...)},
		For: wire.ForConfig, Target: "source", Round: round})
	b, _ := json.Marshal(got)
	return string(b), err
}

const sgConfig = "ruleDirs:\n  - rules\n  - more/rules\n"

// TestAFilesReadAsksForTheProjectConfigFirstAndThenForTheFoldersItNames: no config in the request, round 0, is a read of the config
// the adapter names, and not final (the contracttest row 01); with its file in have, round 1 reads each folder for each ending, final
// (row 02 for what has no file, and the folders for what has); the config given in the request skips the first round (row 03).
// Risk: a tool that keeps its entries in files is read as having none, or is read before its folders are known.
func TestAFilesReadAsksForTheProjectConfigFirstAndThenForTheFoldersItNames(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	for name, c := range map[string]struct {
		round int
		cfg   []wire.Config
		have  []wire.Result
		want  string
	}{
		"round 0, no config": {0, nil, nil, `{"steps":[{"kind":"read","glob":"sgconfig.yml"}],"final":false}`},
		"round 1, the config read": {1, nil, []wire.Result{readResult(0, 0, textFile("sgconfig.yml", sgConfig))},
			`{"steps":[{"kind":"read","glob":"rules/**/*.yml"},{"kind":"read","glob":"rules/**/*.yaml"},{"kind":"read","glob":"rules/**/.*ignore"},{"kind":"read","glob":"more/rules/**/*.yml"},{"kind":"read","glob":"more/rules/**/*.yaml"},{"kind":"read","glob":"more/rules/**/.*ignore"}],"final":true}`},
		"round 1, no config in the project": {1, nil, []wire.Result{readResult(0, 0)}, `{"steps":[],"final":true}`},
		"round 0, the config given": {0, []wire.Config{{Path: "sgconfig.yml", Data: &wire.Bytes{Data: []byte(sgConfig)}}}, nil,
			`{"steps":[{"kind":"read","glob":"rules/**/*.yml"},{"kind":"read","glob":"rules/**/*.yaml"},{"kind":"read","glob":"rules/**/.*ignore"},{"kind":"read","glob":"more/rules/**/*.yml"},{"kind":"read","glob":"more/rules/**/*.yaml"},{"kind":"read","glob":"more/rules/**/.*ignore"}],"final":true}`},
		"round 2, nothing more to read": {2, nil, []wire.Result{readResult(0, 0, textFile("sgconfig.yml", sgConfig)), readResult(1, 0)}, `{"steps":[],"final":true}`},
	} {
		got, err := configPlan(t, p, c.round, c.cfg, c.have...)
		if err != nil || got != c.want {
			t.Errorf("%s:\n got %s, %v\nwant %s", name, got, err, c.want)
		}
	}
}

// TestAFilesReadRefusesWhatLeavesTheProjectRootAndNamesWhereTheFaultIs: a folder that climbs out of the root is refused at the
// config's own pointer, in round 0 a refused (an unreadable is not valid there) and after it an unreadable.
// Risk: a project config names a path outside the project and the plugin asks the engine to read it.
func TestAFilesReadRefusesWhatLeavesTheProjectRootAndNamesWhereTheFaultIs(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	bad := "ruleDirs:\n  - ../outside\n"
	_, err := configPlan(t, p, 1, nil, readResult(0, 0, textFile("sgconfig.yml", bad)))
	if we, ok := err.(*wire.Error); !ok || we.Code != wire.CodeUnreadable || we.Path != "/have/0/files/0/data" {
		t.Errorf("a folder out of the root, after round 0: %v, want unreadable at /have/0/files/0/data", err)
	}
	_, err = configPlan(t, p, 0, []wire.Config{{Path: "sgconfig.yml", Data: &wire.Bytes{Data: []byte(bad)}}})
	if we, ok := err.(*wire.Error); !ok || we.Code != wire.CodeRefused || we.Path != "/config/0/data" {
		t.Errorf("a folder out of the root, in round 0: %v, want refused at /config/0/data", err)
	}
	_, err = configPlan(t, p, 1, nil, readResult(0, 0, textFile("sgconfig.yml", "ruleDirs: []\n")))
	if we, ok := err.(*wire.Error); !ok || we.Code != wire.CodeUnreadable {
		t.Errorf("no folders: %v, want unreadable", err)
	}
}

func filesEntries(t *testing.T, p *declarative.Plugin, have ...wire.Result) (string, error) {
	t.Helper()
	got, err := p.Entries(wire.EntriesRequest{Gathered: wire.Gathered{ToolVersion: "0.51.0", Config: []wire.Config{}, Have: have}, Target: "source"})
	b, _ := json.Marshal(got)
	return string(b), err
}

const ruleError = "id: no-eval\nlanguage: js\nseverity: error\nrule:\n  pattern: eval($A)\n"
const ruleNoSeverity = "id: no-var\nlanguage: js\nrule:\n  pattern: var $A\n"
const ruleOff = "id: quiet\nlanguage: js\nseverity: \"off\"\nrule:\n  pattern: x\n"

// TestAFilesReadReadsEachRuleFileAsOneEntry: the id is the key, the severity word the level it stands for (hint is info), a file with
// no severity stands at the adapter's default, off is off, the rule is the value as JSON; a file in a hidden folder is passed over as
// the tool passes it.
// Risk: a tool's rules are missing from an import, or stand at a severity the tool does not give them.
func TestAFilesReadReadsEachRuleFileAsOneEntry(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	got, err := filesEntries(t, p,
		readResult(0, 0, textFile("sgconfig.yml", sgConfig)),
		readResult(1, 0, textFile("rules/b.yml", ruleNoSeverity), textFile("rules/a.yaml", ruleError), textFile("rules/.hidden/h.yml", "id: hidden\nrule:\n  pattern: x\n")),
		readResult(1, 3, textFile("more/rules/deep/q.yml", ruleOff), textFile("more/rules/notes.txt", "not a rule")))
	// In the order of the files' paths, as the reader reads them; Serve sorts the answer by key.
	const want = `{"entries":[` +
		`{"key":"quiet","on":false,"value":{"pattern":"x"}},` +
		`{"key":"no-eval","on":true,"severity":"error","value":{"pattern":"eval($A)"}},` +
		`{"key":"no-var","on":true,"severity":"info","value":{"pattern":"var $A"}}]}`
	if err != nil || got != want {
		t.Errorf("entries =\n%s, %v\nwant\n%s", got, err, want)
	}
}

// TestAFilesReadRefusesWhatItCannotNameOrHold: a file that is not one mapping, two files with one id, a folder that holds an .ignore
// file and a severity word that is no level each answer unreadable at the file that holds the fault; no project config in have is no
// entries, not a fault.
// Risk: a tool's rule is read wrong and no one is told, or a read of nothing is taken for a read of a project with no rules.
func TestAFilesReadRefusesWhatItCannotNameOrHold(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	cfg := readResult(0, 0, textFile("sgconfig.yml", sgConfig))
	for name, files := range map[string][]wire.File{
		"not a mapping":               {textFile("rules/a.yml", "- 1\n- 2\n")},
		"one id twice":                {textFile("rules/a.yml", ruleError), textFile("rules/b.yml", ruleError)},
		"an ignore file":              {textFile("rules/a.yml", ruleError), textFile("rules/.ignore", "x\n")},
		"a severity that is no level": {textFile("rules/a.yml", "id: x\nseverity: fatal\nrule:\n  pattern: y\n")},
	} {
		_, err := filesEntries(t, p, cfg, readResult(1, 0, files...))
		if we, ok := err.(*wire.Error); !ok || we.Code != wire.CodeUnreadable || !strings.HasPrefix(we.Path, "/have/") {
			t.Errorf("%s: %v, want unreadable at a /have pointer", name, err)
		}
	}
	got, err := filesEntries(t, p)
	if err != nil || got != `{"entries":[]}` {
		t.Errorf("no config in have: %s, %v, want no entries", got, err)
	}
}

// TestTheFourShippedAdaptersReadAsTheyDidThroughLink: ast-grep 1.1.0, eslint 1.1.0, golangci-lint 1.0.0 and tsc 1.1.0, as release-1 ships
// them, each read with the moved reader, no refusal; ast-grep by its files and the others by their print.
// Risk: the re-sync of the reader refuses a tool the engine ships.
func TestTheFourShippedAdaptersReadAsTheyDidThroughLink(t *testing.T) {
	t.Parallel()
	for _, path := range []string{astGrep, "testdata/adapters/eslint/1.1.0.yaml", "testdata/adapters/golangci-lint/1.0.0.yaml", "testdata/adapters/tsc/1.1.0.yaml"} {
		if _, err := declarative.New(read(t, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// TestAFilesReadHoldsItsPlanToSixteenStepsAndAFileWithNoSeverityToTheAdaptersDefault: six folders, each at two endings and its ignore names, are eighteen reads,
// which a plan does not hold, and the plan says so at the config; and an entry file that names no severity where the adapter names no
// default is unreadable at that file, never an entry at a severity nobody gave it.
// Risk: a plan the engine refuses as ErrAnswer, or an entry at a made-up severity.
func TestAFilesReadHoldsItsPlanToSixteenStepsAndAFileWithNoSeverityToTheAdaptersDefault(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	six := "ruleDirs: [a, b, c, d, e, f]\n"
	_, err := configPlan(t, p, 1, nil, readResult(0, 0, textFile("sgconfig.yml", six)))
	if we, ok := err.(*wire.Error); !ok || we.Code != wire.CodeUnreadable || we.Path != "/have/0/files/0/data" {
		t.Errorf("eighteen reads: %v, want unreadable at the config", err)
	}
	five := "ruleDirs: [a, b, c, d, e]\n"
	if got, err := configPlan(t, p, 1, nil, readResult(0, 0, textFile("sgconfig.yml", five))); err != nil || strings.Count(got, `"kind":"read"`) != 15 {
		t.Errorf("sixteen reads: %s, %v", got, err)
	}
	noDefault, err := declarative.New([]byte(strings.Replace(string(read(t, astGrep)), "  default: hint\n", "", 1)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = filesEntries(t, noDefault, readResult(0, 0, textFile("sgconfig.yml", sgConfig)), readResult(1, 0, textFile("rules/b.yml", ruleNoSeverity)))
	if we, ok := err.(*wire.Error); !ok || we.Code != wire.CodeUnreadable || !strings.Contains(we.Message, "no default") {
		t.Errorf("no severity, no default: %v, want unreadable saying so", err)
	}
}

// segmentsMatch is the glob (gitignore and doublestar): a segment ** is a whole
// segment that matches zero or more folders, and * and ? stay inside a segment. It is the test's own, to hold the plan's globs to that
// meaning and not to the code's.
func segmentsMatch(glob, path string) bool {
	return matchSegs(strings.Split(glob, "/"), strings.Split(path, "/"))
}

func matchSegs(g, p []string) bool {
	if len(g) == 0 {
		return len(p) == 0
	}
	if g[0] == "**" {
		for skip := 0; skip <= len(p); skip++ {
			if matchSegs(g[1:], p[skip:]) {
				return true
			}
		}
		return false
	}
	if len(p) == 0 || !matchSeg(g[0], p[0]) {
		return false
	}
	return matchSegs(g[1:], p[1:])
}

func matchSeg(g, s string) bool {
	switch {
	case g == "":
		return s == ""
	case g[0] == '*':
		for i := 0; i <= len(s); i++ {
			if matchSeg(g[1:], s[i:]) {
				return true
			}
		}
		return false
	case s == "":
		return false
	case g[0] == '?' || g[0] == s[0]:
		return matchSeg(g[1:], s[1:])
	}
	return false
}

// TestTheFolderReadsHoldAFileDirectlyInTheFolderAndAtAnyDepth: with ** a whole segment matching zero or more folders,
// the plan's glob <dir>/**/*<ext> holds a file at <dir>/a<ext> and one at <dir>/x/y/b<ext>, and none of another ending or of another folder; and
// every file one of the plan's globs holds is a file the entry reader takes, and the reverse, so the plan asks for what Entries reads.
// Risk: a rule file directly in a rule folder is never asked for, and the project's rules are read as fewer than they are.
func TestTheFolderReadsHoldAFileDirectlyInTheFolderAndAtAnyDepth(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	got, err := p.Plan(wire.PlanRequest{Gathered: wire.Gathered{ToolVersion: "0.51.0", Config: []wire.Config{}, Have: []wire.Result{readResult(0, 0, textFile("sgconfig.yml", "ruleDirs:\n  - rules\n"))}},
		For: wire.ForConfig, Target: "source", Round: 1})
	if err != nil || len(got.Steps) != 3 {
		t.Fatalf("plan = %+v, %v, want two endings and the ignore names", got, err)
	}
	var globs []string
	for _, s := range got.Steps {
		globs = append(globs, s.Read.Glob)
	}
	heldBy := func(path string) bool {
		for _, g := range globs {
			if segmentsMatch(g, path) {
				return true
			}
		}
		return false
	}
	for path, want := range map[string]bool{
		"rules/a.yml": true, "rules/a.yaml": true, "rules/x/y/b.yml": true, "rules/x/b.yaml": true,
		"rules/a.txt": false, "other/a.yml": false, "rules.yml": false, "xrules/a.yml": false,
		// * matches a name that starts with a dot (gitignore and doublestar), so a hidden entry file is asked for as the tool reads it.
		"rules/.x.yml": true, "rules/deep/.y.yaml": true,
		// The ignore names, directly in the folder and below it; and no other dot file.
		"rules/.ignore": true, "rules/.gitignore": true, "rules/deep/.gitignore": true, "rules/.hidden": false, "rules/ignore": false,
	} {
		if heldBy(path) != want {
			t.Errorf("the plan's globs %q hold %s: %v, want %v", globs, path, !want, want)
		}
	}
	// What the plan asks for is what Entries reads: a file directly in the folder, a deep one, and one of another ending.
	have := []wire.Result{readResult(0, 0, textFile("sgconfig.yml", "ruleDirs:\n  - rules\n")),
		readResult(1, 0, textFile("rules/a.yml", ruleError), textFile("rules/deep/er/b.yaml", ruleNoSeverity), textFile("rules/c.txt", "id: txt\nrule:\n  pattern: x\n"))}
	out, err := filesEntries(t, p, have...)
	if err != nil || !strings.Contains(out, `"key":"no-eval"`) || !strings.Contains(out, `"key":"no-var"`) || strings.Contains(out, `"txt"`) {
		t.Errorf("entries = %s, %v, want the direct and the deep file and not the .txt", out, err)
	}
}

// projectAnswer is what the engine answers a read step of a project of these files: each file the glob holds (segmentsMatch), in path order.
func projectAnswer(project map[string]string, round, step int, glob string) wire.Result {
	var paths []string
	for path := range project {
		if segmentsMatch(glob, path) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	files := []wire.File{}
	for _, p := range paths {
		files = append(files, textFile(p, project[p]))
	}
	return wire.Result{Round: round, Step: step, Kind: wire.KindRead, Files: files}
}

// TestAnIgnoreFileInARuleFolderIsSeenThroughTheStepsThePlanAsksFor: the have of this test is built from the plan's own read steps over a
// project, as the engine answers them, so what the reader sees is what the plan asked for. A rule folder with a .ignore, a .gitignore, at its
// top or below it, is refused, never read as if all its files were rules; the same project without one reads every rule.
// Risk: the guard is dead code here, and a project's rules that the tool leaves out are imported.
func TestAnIgnoreFileInARuleFolderIsSeenThroughTheStepsThePlanAsksFor(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	run := func(project map[string]string) (string, error) {
		round0, err := p.Plan(wire.PlanRequest{Gathered: wire.Gathered{ToolVersion: "0.51.0", Config: []wire.Config{}, Have: []wire.Result{}}, For: wire.ForConfig, Target: "source", Round: 0})
		if err != nil || len(round0.Steps) != 1 {
			t.Fatalf("round 0 = %+v, %v", round0, err)
		}
		have := []wire.Result{projectAnswer(project, 0, 0, round0.Steps[0].Read.Glob)}
		round1, err := p.Plan(wire.PlanRequest{Gathered: wire.Gathered{ToolVersion: "0.51.0", Config: []wire.Config{}, Have: have}, For: wire.ForConfig, Target: "source", Round: 1})
		if err != nil {
			t.Fatalf("round 1: %v", err)
		}
		for i, s := range round1.Steps {
			have = append(have, projectAnswer(project, 1, i, s.Read.Glob))
		}
		return filesEntries(t, p, have...)
	}
	clean := map[string]string{"sgconfig.yml": "ruleDirs:\n  - rules\n", "rules/a.yml": ruleError, "rules/deep/b.yaml": ruleNoSeverity, "src/x.js": "x"}
	got, err := run(clean)
	if err != nil || !strings.Contains(got, `"no-eval"`) || !strings.Contains(got, `"no-var"`) {
		t.Fatalf("a project with no ignore file: %s, %v", got, err)
	}
	for name, ignore := range map[string]string{"at the top": "rules/.ignore", "a .gitignore": "rules/.gitignore", "below": "rules/deep/.gitignore"} {
		project := map[string]string{}
		for k, v := range clean {
			project[k] = v
		}
		project[ignore] = "a.yml\n"
		if _, err := run(project); err == nil || err.(*wire.Error).Code != wire.CodeUnreadable || !strings.HasPrefix(err.(*wire.Error).Path, "/have/") {
			t.Errorf("%s: %v, want unreadable at a /have pointer", name, err)
		}
	}
}

// TestAFolderNameWithAGlobCharacterIsRefused: * and ? in a folder name would make the read reach other folders, and the other glob
// characters are refused by the contract; the folder is read by its name or not at all.
// Risk: a read of rules* takes in the files of every folder that begins so, or the engine refuses the plan.
func TestAFolderNameWithAGlobCharacterIsRefused(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	for _, dir := range []string{"rules*", "rul?s", "rules[1]", "rules{a}"} {
		_, err := configPlan(t, p, 1, nil, readResult(0, 0, textFile("sgconfig.yml", "ruleDirs:\n  - \""+dir+"\"\n")))
		if we, ok := err.(*wire.Error); !ok || we.Code != wire.CodeUnreadable || we.Path != "/have/0/files/0/data" {
			t.Errorf("%s: %v, want unreadable at the config", dir, err)
		}
	}
}

// TestOnlyTheExactNamesIgnoreAndGitignoreAreRefused: the plan's glob <dir>/**/.*ignore reads wider than it means, so the engine hands back
// .dockerignore and .npmignore too, and the entries read acts on the two names of IgnoreFile alone: a project whose rule folder holds
// those two imports every rule and refuses nothing. Built through the plan's own steps, as the engine answers them.
// Risk: a check by the suffix "ignore" refuses a project for a file no tool of this reads as a list of files to leave out.
func TestOnlyTheExactNamesIgnoreAndGitignoreAreRefused(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	project := map[string]string{"sgconfig.yml": "ruleDirs:\n  - rules\n", "rules/a.yml": ruleError, "rules/.dockerignore": "x\n", "rules/.npmignore": "y\n", "rules/deep/.npmignore": "z\n"}
	round0 := projectAnswer(project, 0, 0, "sgconfig.yml")
	round1, err := p.Plan(wire.PlanRequest{Gathered: wire.Gathered{ToolVersion: "0.51.0", Config: []wire.Config{}, Have: []wire.Result{round0}}, For: wire.ForConfig, Target: "source", Round: 1})
	if err != nil {
		t.Fatal(err)
	}
	have := []wire.Result{round0}
	handed := 0
	for i, s := range round1.Steps {
		r := projectAnswer(project, 1, i, s.Read.Glob)
		handed += len(r.Files)
		have = append(have, r)
	}
	// The glob does hand them back: the test is of what the entries read does with them.
	if handed != 4 {
		t.Fatalf("the plan's steps handed back %d files, want the rule and the three ignore-shaped files", handed)
	}
	got, err := filesEntries(t, p, have...)
	if err != nil || !strings.Contains(got, `"no-eval"`) {
		t.Errorf("entries = %s, %v, want the rule read and no refusal", got, err)
	}
}

// TestTheFaultsPointerIsTheFileWhoseErrorItIs: two broken files of different endings, gathered in the order .yml then .yaml while the
// reader reports in path order (a.yaml before b.yml), are answered with the message of the one the reader names and the pointer of that
// same file, not the message of one and the pointer of the other.
// Risk: the unreadable names a file that has nothing wrong with the message it carries.
func TestTheFaultsPointerIsTheFileWhoseErrorItIs(t *testing.T) {
	t.Parallel()
	p := newPlugin(t, astGrep)
	_, err := filesEntries(t, p,
		readResult(0, 0, textFile("sgconfig.yml", sgConfig)),
		readResult(1, 0, textFile("rules/b.yml", "key: [unclosed\n")),
		readResult(1, 1, textFile("rules/a.yaml", "- 1\n- 2\n")))
	we, ok := err.(*wire.Error)
	if !ok || we.Code != wire.CodeUnreadable {
		t.Fatalf("two broken files: %v, want unreadable", err)
	}
	// The reader names a.yaml, which is first in path order; the pointer is a.yaml's, the second file of the second result.
	if !strings.Contains(we.Message, "rules/a.yaml") || we.Path != "/have/2/files/0/data" {
		t.Errorf("unreadable at %s saying %q, want a.yaml's message at a.yaml's pointer /have/2/files/0/data", we.Path, we.Message)
	}
}
