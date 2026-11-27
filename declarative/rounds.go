package declarative

import (
	"fmt"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/plugin"
	ptr "github.com/birdtable/gragas-plugin-sdk/pointer"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// This file is the plan rounds for a config read of files: a tool that prints no effective config and keeps
// each entry in a file of its own, in the folders its project config names. The reading is the moved code's (EntryFolders, EntryFile,
// IgnoreFile and readFileEntries of files.go); what is new is the asking. The plugin never reads a file: it asks the engine to, as a
// read step, and the engine answers in the next round, at most three plan calls (rounds 0 to 2) and then one call that reads what
// was gathered.
//
//	round 0, the project config not in the request's config set:  read it, not final.
//	round 1, it has been read:                                    read each folder for each ending, final.
//	round 0, the project config in the config set:                read each folder for each ending, final.
//
// A project that has no such config is a fact about the project, and the plan is no steps and final: Entries then answers no entries.
//
// What the engine holds of this: the entries request carries every file gathered, so a project whose rule files add up to more than 64 KiB is
// refused at the engine's cap on an entries request before this plugin is asked, and a user of ast-grep meets that refusal and not an
// answer. A glob's * matches a name that starts with a dot (gitignore and doublestar), so *.yml holds .x.yml.

// maxRoundSteps is the most steps one plan holds.
const maxRoundSteps = 16

// globChars are the characters a folder name may not hold, since the folder is put in a glob: * and ? have a meaning there, and [ ] { } and \ are
// refused by the contract.
const globChars = "*?[]{}\\"

// projectConfig is the project config of a files read as the request has it: the file of the config set, or, failing that, the first
// file the results of the read steps hold by that path. at is the pointer of its bytes in the request.
func projectConfig(c model.ConfigRead, cfg []wire.Config, have []wire.Result) (name string, data []byte, at string, found bool) {
	name = c.Files.Config
	for i, f := range cfg {
		if f.Path == name && f.Data != nil {
			return name, f.Data.Data, ptr.Join("config", ptr.Index(i), "data"), true
		}
	}
	for r, res := range have {
		if res.Kind != wire.KindRead {
			continue
		}
		for i, f := range res.Files {
			if f.Path == name {
				return name, f.Data.Data, ptr.Join("have", ptr.Index(r), "files", ptr.Index(i), "data"), true
			}
		}
	}
	return name, nil, "", false
}

// planFiles is Plan for a config read of files, in the rounds above.
func (p *Plugin) planFiles(c model.ConfigRead, req wire.PlanRequest) (wire.PlanResponse, error) {
	if req.Round > 2 {
		return wire.PlanResponse{}, plugin.Unsupportedf("a plan has rounds 0 to 2, and round %d was asked", req.Round)
	}
	name, data, at, found := projectConfig(c, req.Config, req.Have)
	if !found {
		if req.Round == 0 {
			return wire.PlanResponse{Steps: []wire.Step{{Read: &wire.Read{Glob: name}}}, Final: false}, nil
		}
		return wire.PlanResponse{Steps: []wire.Step{}, Final: true}, nil
	}
	// The folders are read in the round after the config's own, or in round 0 where the config was given: a later round has
	// nothing more to ask for, and the plan is final.
	if req.Round >= 2 || (req.Round >= 1 && strings.HasPrefix(at, "/config/")) {
		return wire.PlanResponse{Steps: []wire.Step{}, Final: true}, nil
	}
	fault := func(err error) error {
		// An unreadable is not valid in round 0, where the config was given: that input is refused.
		if req.Round == 0 {
			return plugin.Refusedf(at, "%v", err)
		}
		return plugin.Unreadablef(at, "%v", err)
	}
	folders, err := EntryFolders(c, name, data)
	if err != nil {
		return wire.PlanResponse{}, fault(err)
	}
	steps := []wire.Step{}
	for _, dir := range folders {
		if bad := strings.IndexAny(dir, globChars); bad >= 0 {
			return wire.PlanResponse{}, fault(fmt.Errorf("%s names the folder %q, which holds the glob character %q: a folder is read by its name", name, dir, dir[bad]))
		}
		for _, ext := range c.Files.Ext {
			steps = append(steps, wire.Step{Read: &wire.Read{Glob: dir + "/**/*" + ext}})
		}
		// The names a tool reads to leave files out (IgnoreFile). A read of the entry files never returns them, and a folder that holds one is
		// refused, never guessed, so they are asked for as well: .ignore and .gitignore, and the other names of that shape.
		// The glob is case exact, so a case-folded name (.Ignore) on a disk that keeps case is outside this guard.
		steps = append(steps, wire.Step{Read: &wire.Read{Glob: dir + "/**/.*ignore"}})
	}
	if len(steps) > maxRoundSteps {
		return wire.PlanResponse{}, fault(fmt.Errorf("%s names %d folders, which with %d endings is %d reads, and a plan holds %d", name, len(folders), len(c.Files.Ext), len(steps), maxRoundSteps))
	}
	return wire.PlanResponse{Steps: steps, Final: true}, nil
}

// entriesFromFiles is Entries for a config read of files: the entry files the read steps gathered, each one entry by the read's entry map,
// its key the file's own. A file in a hidden folder is passed over as the tool passes it; a folder that holds an ignore file, a file
// that is not one mapping, an entry that names no severity where the adapter has no default, a severity that is no level and one key
// in two files are each unreadable at the file that holds the fault. No project config in the request is no entries.
func (p *Plugin) entriesFromFiles(c model.ConfigRead, req wire.EntriesRequest) (wire.EntriesResponse, error) {
	out := wire.EntriesResponse{Entries: []wire.Entry{}}
	name, data, at, found := projectConfig(c, req.Config, req.Have)
	if !found {
		return out, nil
	}
	folders, err := EntryFolders(c, name, data)
	if err != nil {
		return wire.EntriesResponse{}, plugin.Unreadablef(at, "%v", err)
	}
	for _, dir := range folders {
		if bad := strings.IndexAny(dir, globChars); bad >= 0 {
			return wire.EntriesResponse{}, plugin.Unreadablef(at, "%s names the folder %q, which holds the glob character %q: a folder is read by its name", name, dir, dir[bad])
		}
	}
	var files []ConfigFile
	where := map[string]string{}
	for r, res := range req.Have {
		if res.Kind != wire.KindRead {
			continue
		}
		for i, f := range res.Files {
			here := ptr.Join("have", ptr.Index(r), "files", ptr.Index(i), "data")
			for _, dir := range folders {
				if IgnoreFile(dir, f.Path) {
					return wire.EntriesResponse{}, plugin.Unreadablef(here, "%s holds the file %s, which a tool reads to leave files out and this reader does not read: the folder is refused, never guessed", dir, f.Path)
				}
			}
			if _, seen := where[f.Path]; seen {
				continue
			}
			for _, dir := range folders {
				if EntryFile(c, dir, f.Path) {
					files = append(files, ConfigFile{Path: f.Path, Data: f.Data.Data})
					where[f.Path] = here
					break
				}
			}
		}
	}
	typed, err := readFileEntries(c, files)
	if err != nil {
		// The file at fault is the one whose own error is the reader's: each is read alone and its error compared, so that where two files are
		// broken (the reader reports in path order and they were gathered in another) the message and the pointer are one file's. The error's
		// text is never parsed. A fault of the set (too many files, too many bytes) is no file's, and is at /have.
		pointer := ptr.Join("have")
		for _, f := range files {
			if _, one := readFileEntries(c, []ConfigFile{f}); one != nil && one.Error() == err.Error() {
				pointer = where[f.Path]
				break
			}
		}
		return wire.EntriesResponse{}, plugin.Unreadablef(pointer, "%v", err)
	}
	owner := map[string]string{}
	for _, f := range typed {
		for _, e := range f.Entries {
			if first, twice := owner[e.Key]; twice {
				return wire.EntriesResponse{}, plugin.Unreadablef(where[f.Path], "the key %q is in %s and in %s: each key stands once", e.Key, first, f.Path)
			}
			owner[e.Key] = f.Path
			if e.Severity == "" && p.ad.Severities.Default == "" {
				return wire.EntriesResponse{}, plugin.Unreadablef(where[f.Path], "%s names no severity, and the adapter has no default for a file that names none", f.Path)
			}
			w, err := p.wireEntry(e, where[f.Path], p.ad.Severities.Default)
			if err != nil {
				return wire.EntriesResponse{}, err
			}
			out.Entries = append(out.Entries, w)
		}
	}
	return out, nil
}
