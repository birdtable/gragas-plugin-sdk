package declarative

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
)

// MaxEntryFiles caps the entry files one files read reads: they are a
// project's, so they are bounded before they are read.
const MaxEntryFiles = 4096

// The reader of a config read's files: a tool that prints no
// effective config and keeps each entry in a file of its own, in folders its
// project config names. Nothing here is per tool: the adapter's
// FileRead says where the folders are named and which files are entries,
// and its entry map reads the files' mappings, a list in path order, as it
// reads a printed document. The files themselves are read through the Tree
// port by the caller, which reads only what EntryFolders and EntryFile allow.

// EntryFolders is the folders, each slash-separated and relative to the
// project root, that the project config at name, holding data, names at the
// read's Dirs pointer. A folder is read relative to the project config's own
// folder, as the tool reads it. A folder that is not a list of text, that is
// empty, absolute, or has a step that is empty, . or .., is refused, naming
// it: a folder that leaves the project root is never read, and one that
// reaches out through a link is refused by the Tree port when it is read.
func EntryFolders(c model.ConfigRead, name string, data []byte) ([]string, error) {
	if c.Files == nil {
		return nil, fmt.Errorf("the config read of %s reads no entry files", c.Target)
	}
	doc, err := configDocument(c.Format, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	at, ok := pointer(doc, c.Files.Dirs)
	if !ok {
		return nil, fmt.Errorf("%s names no entry folders at %q", name, c.Files.Dirs)
	}
	list, ok := at.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("%s holds at %q no list of folders", name, c.Files.Dirs)
	}
	var out []string
	for _, v := range list {
		dir, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%s names a folder at %q that is not text: %v", name, c.Files.Dirs, v)
		}
		if !below(dir) {
			return nil, fmt.Errorf("%s names the folder %q, which is not a path below the project root: slash-separated, no step empty, . or .., not absolute, so it is not read", name, dir)
		}
		at := path.Join(path.Dir(name), dir)
		if !slices.Contains(out, at) {
			out = append(out, at)
		}
	}
	return out, nil
}

// below is whether p is a relative path whose every step is a name: no
// leading slash, no drive, no backslash, no step empty, . or ..
func below(p string) bool {
	if p == "" || strings.ContainsAny(p, `\:`) {
		return false
	}
	for step := range strings.SplitSeq(p, "/") {
		if step == "" || step == "." || step == ".." {
			return false
		}
	}
	return true
}

// EntryFile is whether the file at p, as the Tree port lists it, is one of
// the read's entry files in the folder dir: below it at any depth, as the tool
// walks its rule folders to the bottom; in no folder below dir with a hidden
// name (one that starts with a dot), which the tool passes over, though a
// hidden file is read as the tool reads it; and ending in one of the read's
// endings as written, since the tool compares them so. This is the walk of
// the one tool the decision adapter-file-entries names, read against its
// pinned release: the next adapter of a files read checks its
// own tool's walk against it.
func EntryFile(c model.ConfigRead, dir, p string) bool {
	rest, ok := strings.CutPrefix(p, dir+"/")
	if c.Files == nil || !ok || !slices.Contains(c.Files.Ext, path.Ext(p)) {
		return false
	}
	folders := strings.Split(rest, "/")
	for _, step := range folders[:len(folders)-1] {
		if strings.HasPrefix(step, ".") {
			return false
		}
	}
	return true
}

// IgnoreFile is whether the file at p, below the folder dir, is a file of
// the names a tool's walk reads to leave files out (.ignore, and .gitignore in
// a repository), which this reader does not read: a read of a folder that
// holds one is refused, never guessed.
func IgnoreFile(dir, p string) bool {
	base := path.Base(p)
	// in any case: a disk that folds case hands the tool .Ignore for .ignore,
	// and refusing one a disk keeps apart refuses on the closed side
	return strings.HasPrefix(p, dir+"/") && (strings.EqualFold(base, ".ignore") || strings.EqualFold(base, ".gitignore"))
}

// ReadFileEntries reads the entries of the entry files, as the read's entry
// map says of a list holding each file's mapping, one file at a time, in path
// order, so each entry is the one file's it is read from. Each file is read
// by the read's format, as a printed config is; a file that is not one
// mapping, more than MaxEntryFiles files, and more than MaxPrinted bytes in all
// are refused, never read as no entry.
func ReadFileEntries(c model.ConfigRead, files []ConfigFile) ([]FileEntries, error) {
	typed, err := readFileEntries(c, files)
	if err != nil {
		return nil, err
	}
	out := make([]FileEntries, 0, len(typed))
	for _, t := range typed {
		fe := FileEntries{Path: t.Path}
		for _, e := range t.Entries {
			fe.Entries = append(fe.Entries, e.ToolEntry)
		}
		out = append(out, fe)
	}
	return out, nil
}

// typedFile is the entries one entry file holds, each with its value and options as the document holds them, typed.
type typedFile struct {
	Path    string
	Entries []entry
}

// readFileEntries is ReadFileEntries with each entry typed, as readEntries is to ReadEntries: a plugin answers an
// entry's value and options as JSON, not as the text of a one-line form.
func readFileEntries(c model.ConfigRead, files []ConfigFile) ([]typedFile, error) {
	if c.Files == nil {
		return nil, fmt.Errorf("the config read of %s reads no entry files", c.Target)
	}
	if len(files) > MaxEntryFiles {
		return nil, fmt.Errorf("%d entry files, over the %d read", len(files), MaxEntryFiles)
	}
	sorted := slices.SortedFunc(slices.Values(files), func(a, b ConfigFile) int { return strings.Compare(a.Path, b.Path) })
	out, size, count := make([]typedFile, 0, len(sorted)), 0, 0
	for _, f := range sorted {
		if size += len(f.Data); size > MaxPrinted {
			return nil, fmt.Errorf("the entry files hold more than the %d bytes read", MaxPrinted)
		}
		v, err := configDocument(c.Format, f.Data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		if _, ok := v.(*ordered.Map); !ok {
			return nil, fmt.Errorf("%s is not one mapping, so it holds no entry the read can name", f.Path)
		}
		typed, err := entriesOf(c, []any{v})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		if count += len(typed); count > MaxFindings {
			return nil, fmt.Errorf("the entry files hold more than %d entries", MaxFindings)
		}
		out = append(out, typedFile{Path: f.Path, Entries: typed})
	}
	return out, nil
}

// FileEntries is the entries one entry file holds.
type FileEntries struct {
	Path    string
	Entries []model.ToolEntry
}
