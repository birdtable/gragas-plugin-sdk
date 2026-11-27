package declarative

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/canon"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
	"github.com/birdtable/gragas-plugin-sdk/plugin"
	ptr "github.com/birdtable/gragas-plugin-sdk/pointer"
	"github.com/birdtable/gragas-plugin-sdk/scratch"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// This file is the other direction of the new pieces: what a tool printed, read by the moved reader, becomes the
// wire's entries and findings. The reading is the moved code's (ReadEntries' typed form and ReadDiagnostics); what is
// new is the step from the tool's words (its severity word, its value) to the engine's.

// lastRun is the last result of a run in have, which the plan's last step of a run gave, and its index.
func lastRun(have []wire.Result) (int, *wire.Result) {
	for i := len(have) - 1; i >= 0; i-- {
		if have[i].Kind == wire.KindRun {
			return i, &have[i]
		}
	}
	return -1, nil
}

// Entries reads the entries of the effective config the tool printed for the target, as the adapter's entry map says,
// from the last run in have. Each is on unless its severity is one of the adapter's off words or its value one of
// its off values (the key's own where the adapter names them); its severity is the engine's, through the adapter's
// levels; its value and options are the config's own, as JSON. A target the adapter reads no config for answers
// unsupported; a run that failed, was cut, or printed what the reader cannot read, or a severity word that is no level and
// no off word, answers unreadable at the result.
func (p *Plugin) Entries(req wire.EntriesRequest) (wire.EntriesResponse, error) {
	i := slices.IndexFunc(p.ad.Config, func(c model.ConfigRead) bool { return c.Target == model.TargetType(req.Target) })
	if i < 0 {
		return wire.EntriesResponse{}, plugin.Unsupportedf("the adapter of %s reads no config for the target %s", p.ad.Tool, req.Target)
	}
	read := p.ad.Config[i]
	if read.Files != nil {
		return p.entriesFromFiles(read, req)
	}
	at, res := lastRun(req.Have)
	if res == nil {
		// The plan had no steps (no config in the project): nothing was printed, and nothing is entered.
		return wire.EntriesResponse{Entries: []wire.Entry{}}, nil
	}
	where := ptr.Join("have", ptr.Index(at), "stdout")
	if res.Exit == nil || *res.Exit != 0 {
		return wire.EntriesResponse{}, plugin.Unreadablef(where, "the tool exited %s printing its config", exitOf(res))
	}
	if res.Cut != nil && *res.Cut {
		return wire.EntriesResponse{}, plugin.Unreadablef(where, "the tool's output was cut, so the config is not whole")
	}
	typed, err := readEntries(read, res.Stdout.Raw())
	if err != nil {
		return wire.EntriesResponse{}, plugin.Unreadablef(where, "%v", err)
	}
	out := wire.EntriesResponse{Entries: []wire.Entry{}}
	for _, e := range typed {
		w, err := p.wireEntry(e, where, "")
		if err != nil {
			return wire.EntriesResponse{}, err
		}
		out.Entries = append(out.Entries, w)
	}
	return out, nil
}

// wireEntry is the entry the wire holds for one the reader read, as Entries describes: on unless its severity is an off word or its
// value an off value, its severity the engine's through the adapter's levels, its value and options as JSON. An entry file that names no
// severity (def is the adapter's default level, "" where it has none, and then the file is refused) stands at def. Where is the pointer an
// unreadable names.
func (p *Plugin) wireEntry(e entry, where, def string) (wire.Entry, error) {
	w := wire.Entry{Key: e.Key, On: true}
	word := e.Severity
	if word == "" {
		word = def
	}
	sev, isOff, ok := p.severity(word)
	if !ok {
		return wire.Entry{}, plugin.Unreadablef(where, "the severity %q of %s is no level and no off word of the adapter", word, e.Key)
	}
	w.Severity = sev
	if isOff {
		w.On = false
	}
	if off(scaleOf(p.ad.Values, e.Key), e.value) {
		w.On = false
	}
	var err error
	if e.value != nil || e.null {
		if w.Value, err = jsonOf(e.value); err != nil {
			return wire.Entry{}, plugin.Unreadablef(where, "the value of %s does not write as JSON: %v", e.Key, err)
		}
	}
	if e.options != nil {
		if w.Options, err = jsonOf(e.options); err != nil {
			return wire.Entry{}, plugin.Unreadablef(where, "the options of %s do not write as JSON: %v", e.Key, err)
		}
	}
	return w, nil
}

// severity is the engine's word for a tool's: the level's engine word for a level; off for one of the adapter's off
// words; no word and not off for no word at all, which the adapter names none for. ok is false for a word that is
// none of these.
func (p *Plugin) severity(word string) (sev wire.Severity, off, ok bool) {
	if word == "" {
		return "", false, true
	}
	if slices.Contains(p.ad.Severities.Off, word) {
		return "", true, true
	}
	for _, l := range p.ad.Severities.Levels {
		if l.Name == word {
			return wire.Severity(l.Is), false, true
		}
	}
	return "", false, false
}

// exitOf is the exit code of a result as text.
func exitOf(r *wire.Result) string {
	if r.Exit == nil {
		return "with no code"
	}
	return fmt.Sprint(*r.Exit)
}

// Findings reads the findings of the run the plan made, from the last run in have, by the adapter's own output form
// (sarif, json or lines). The output is the first file the run collected where the adapter's argv names {output}, and
// the run's stdout where it does not. A run whose exit code is none of the adapter's ran codes (or, where it names none,
// is not 0), or one that printed a finding of an error key of the adapter's, is ran: false with the reason, and no
// findings. Paths are cut of the scratch folder, and a finding's severity is the engine's through the adapter's levels.
func (p *Plugin) Findings(req wire.FindingsRequest) (wire.FindingsResponse, error) {
	at, res := lastRun(req.Have)
	if res == nil {
		return wire.FindingsResponse{}, plugin.Unreadablef("/have", "no run of the tool is in have")
	}
	run := p.ad.Run
	codes := run.Output.RanCodes
	if res.Exit == nil || (len(codes) == 0 && *res.Exit != 0) || (len(codes) > 0 && !slices.Contains(codes, *res.Exit)) {
		return wire.FindingsResponse{Ran: false, Why: fmt.Sprintf("the tool exited %s, which is none of the codes that mean it ran", exitOf(res)), Findings: []wire.Finding{}}, nil
	}
	where := ptr.Join("have", ptr.Index(at), "stdout")
	body := res.Stdout.Raw()
	if slices.Contains(run.Argv, "{output}") {
		if len(res.Files) == 0 {
			return wire.FindingsResponse{}, plugin.Unreadablef(ptr.Join("have", ptr.Index(at), "files"), "the run collected no output file")
		}
		body = res.Files[0].Data.Raw()
		where = ptr.Join("have", ptr.Index(at), "files", "0")
	}
	if res.Cut != nil && *res.Cut {
		return wire.FindingsResponse{Ran: false, Why: "the tool's output was cut, so the findings are not whole", Findings: []wire.Finding{}}, nil
	}
	diags, err := ReadDiagnostics(run.Output, body)
	if err != nil {
		return wire.FindingsResponse{}, plugin.Unreadablef(where, "%v", err)
	}
	out := wire.FindingsResponse{Ran: true, Findings: []wire.Finding{}}
	for _, d := range diags {
		if slices.Contains(run.Output.ErrorKeys, d.Key) {
			return wire.FindingsResponse{Ran: false, Why: fmt.Sprintf("the tool printed its own failure, %s", d.Key), Findings: []wire.Finding{}}, nil
		}
		f := wire.Finding{Key: d.Key, Message: d.Message, Path: scratch.Path(d.Path, req.Scratch), Line: d.Line, Column: d.Column, URL: d.URL}
		if f.Message == "" {
			f.Message = d.Key
		}
		if sev, isOff, ok := p.severity(d.Severity); ok && !isOff {
			f.Severity = sev
		}
		out.Findings = append(out.Findings, f)
	}
	return out, nil
}

// jsonOf is a typed value of a printed config, in any of the forms the moved reader gives (canon's for JSON, ordered's
// for YAML), as JSON.
func jsonOf(v any) (json.RawMessage, error) {
	b, err := json.Marshal(plainOf(v))
	return b, err
}

// plainOf is v as the plain Go values encoding/json writes: a map, a list, a string, a number, a bool or nil.
func plainOf(v any) any {
	switch x := v.(type) {
	case nil, canon.Null:
		return nil
	case canon.Bool:
		return bool(x)
	case canon.Int:
		return int64(x)
	case canon.Float:
		return float64(x)
	case canon.String:
		return string(x)
	case canon.Array:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plainOf(e)
		}
		return out
	case canon.Object:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = plainOf(e)
		}
		return out
	case *ordered.Map:
		out := make(map[string]any, x.Len())
		for _, k := range x.Keys() {
			e, _ := x.Get(k)
			out[k] = plainOf(e)
		}
		return out
	case ordered.Timestamp:
		return string(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plainOf(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = plainOf(e)
		}
		return out
	}
	return v
}
