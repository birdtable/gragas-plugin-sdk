package declarative_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// filesRead is a config read of entry files, as an adapter declares one: the
// project config lint.yaml names its folders at /dirs, and each file ending
// .yaml or .yml there is one entry, keyed by its id.
func filesRead() model.ConfigRead {
	return model.ConfigRead{
		Target: model.TargetSource, Format: model.ConfigYAML,
		Files:   &model.FileRead{Config: "lint.yaml", Dirs: "/dirs", Ext: []string{".yaml", ".yml"}},
		Entries: model.EntryMap{Each: "", Key: "/id", Value: "/rule", Severity: "/level"},
	}
}

// TestAFilesReadNamesItsFoldersByTheConfigAndReadsEachFileAsOneEntry: the
// folders are the list at Dirs, each read relative to the project config's
// own folder and each once; a file below one of them at any depth, in no
// hidden folder, ending in one of the endings as written, is an entry file,
// a hidden file among them, as the tool of the decision adapter-file-entries
// runs one; each file's mapping, read by the entry map, gives its entry, the
// files in path order. Planted failure: a folder read
// from the root, a folder walked only at its top, a hidden folder read, a
// hidden file left out, an ending folded to lower case, or an entry keyed by
// its name.
func TestAFilesReadNamesItsFoldersByTheConfigAndReadsEachFileAsOneEntry(t *testing.T) {
	t.Parallel()
	r := filesRead()
	for _, c := range []struct {
		name, config string
		want         []string
	}{
		{"lint.yaml", "dirs: [rules, more/rules]\n", []string{"rules", "more/rules"}},
		{"sub/lint.yaml", "dirs:\n  - rules\n  - rules\n", []string{"sub/rules"}},
	} {
		if got, err := declarative.EntryFolders(r, c.name, []byte(c.config)); err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: folders %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	for _, c := range []struct {
		p    string
		want bool
	}{
		{"rules/a.yml", true}, {"rules/a.json", false}, {"rules/deeper/a.yml", true}, {"rules/a/b/c/d.yml", true},
		{"rules/.hidden.yml", true}, {"rules/sub/.h.yaml", true}, {"rules/sub/.yml", true},
		{"rules/UPPER.YML", false}, {"rules/Mixed.Yaml", false}, {"rules/a.YAML", false},
		{"rules/.git/a.yml", false}, {"rules/deeper/.x/a.yml", false}, {"rulesx/a.yml", false}, {"other/a.yml", false},
	} {
		if got := declarative.EntryFile(r, "rules", c.p); got != c.want {
			t.Errorf("EntryFile(rules, %s) = %v, want %v", c.p, got, c.want)
		}
	}
	held, err := declarative.ReadFileEntries(r, []declarative.ConfigFile{
		{Path: "rules/b.yml", Data: []byte("id: second\nlevel: warn\nrule: {pattern: b}\n")},
		{Path: "rules/a.yaml", Data: []byte("id: first\nlevel: error\nrule: {pattern: a}\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range held {
		for _, e := range f.Entries {
			got = append(got, fmt.Sprintf("%s %s %s %s", f.Path, e.Key, e.Severity, e.Value))
		}
	}
	if want := []string{`rules/a.yaml first error {"pattern": "a"}`, `rules/b.yml second warn {"pattern": "b"}`}; !slices.Equal(got, want) {
		t.Errorf("entries %q, want %q", got, want)
	}
	if _, err := declarative.ReadFileEntries(model.ConfigRead{Target: model.TargetSource}, nil); err == nil {
		t.Error("a read with no files read its files")
	}
	if _, err := declarative.EntryFolders(model.ConfigRead{Target: model.TargetSource}, "lint.yaml", []byte("dirs: [rules]\n")); err == nil {
		t.Error("a read with no files named folders")
	}
}

// TestAFilesReadReadsUpToItsCapOfFilesAndRefusesOneMore: the entry files are
// a project's, so they are bounded before they are read.
// boundary: MaxEntryFiles
// Risk: a project folder of any size read whole into memory.
func TestAFilesReadReadsUpToItsCapOfFilesAndRefusesOneMore(t *testing.T) {
	t.Parallel()
	for _, n := range []int{declarative.MaxEntryFiles, declarative.MaxEntryFiles + 1} {
		files := make([]declarative.ConfigFile, n)
		for i := range files {
			files[i] = declarative.ConfigFile{Path: fmt.Sprintf("rules/r%05d.yml", i), Data: fmt.Appendf(nil, "id: r%d\nlevel: error\n", i)}
		}
		_, err := declarative.ReadFileEntries(filesRead(), files)
		over := err != nil && strings.Contains(err.Error(), fmt.Sprintf("over the %d read", declarative.MaxEntryFiles))
		if over != (n > declarative.MaxEntryFiles) || (err != nil && !over) {
			t.Errorf("%d files: err = %v, want the cap's refusal only past %d", n, err, declarative.MaxEntryFiles)
		}
	}
}

// TestAFilesReadReadsUpToItsByteCapAndRefusesOneMore: the entry files hold at
// most MaxPrinted bytes in all, as a printed config does, and one byte more
// is refused, not read as fewer entries.
// boundary: MaxPrinted
// Risk: a project's rule folder of any size held in memory.
func TestAFilesReadReadsUpToItsByteCapAndRefusesOneMore(t *testing.T) {
	t.Parallel()
	head := "id: big\nlevel: error\nnote: "
	for _, n := range []int{declarative.MaxPrinted, declarative.MaxPrinted + 1} {
		data := head + strings.Repeat("a", n-len(head)-1) + "\n"
		_, err := declarative.ReadFileEntries(filesRead(), []declarative.ConfigFile{{Path: "rules/big.yml", Data: []byte(data)}})
		over := err != nil && strings.Contains(err.Error(), fmt.Sprintf("more than the %d bytes read", declarative.MaxPrinted))
		if over != (n > declarative.MaxPrinted) || (err != nil && !over) {
			t.Errorf("%d bytes: err = %v, want the cap's refusal only past %d", n, err, declarative.MaxPrinted)
		}
	}
}

// TestAnIgnoreFileIsNamedWhereverItIsBelowANamedFolder: a .ignore or a
// .gitignore at any depth below a named folder is one the tool's walk reads to
// leave files out, in any case of its name, since a disk that folds case
// hands the tool .Ignore for .ignore, and this reader names it so the read
// can be refused. Planted failure: an
// ignore file below the top of the folder, or in another case, passed over.
func TestAnIgnoreFileIsNamedWhereverItIsBelowANamedFolder(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		p    string
		want bool
	}{{"rules/.ignore", true}, {"rules/.gitignore", true}, {"rules/ci/.Ignore", true}, {"rules/cg/.GITIGNORE", true}, {"rules/a/b/.gitignore", true}, {"rules/a.ignore", false}, {"rules/ignore", false}, {"other/.gitignore", false}, {".gitignore", false}} {
		if got := declarative.IgnoreFile("rules", c.p); got != c.want {
			t.Errorf("IgnoreFile(rules, %s) = %v, want %v", c.p, got, c.want)
		}
	}
}
