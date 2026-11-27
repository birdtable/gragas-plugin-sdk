package declarative

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/port"
	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// This file is the step from a wire preset request to what the moved writer takes, with Plugin and New. The package's
// other new files are plan.go (Plan), entries_read.go (Entries and Findings) and rank.go (Rank); everything else is moved
// from gragas, and SOURCE lists it. gragas's Generate reads a rulebook's bindings; a plugin is handed entries
// (wire.PresetEntry), one for each key and target, each naming the rules that hold it. candidates turns each into the
// candidate generateTool writes, and Preset writes the files.

// Plugin is a plugin whose operations are read from one adapter document, a tool's rules as a flat list: the
// declarative plugin of the plugin contract. It provides all five operations: plan, entries, findings, rank and preset.
type Plugin struct {
	plugin.Unsupported
	ad model.Adapter
}

// noSchema is the Schema port of a running plugin: every document is valid. The adapter files' schema is checked by
// the plugins repository's gate, where the files live, and the reader's own Go checks run here.
type noSchema struct{}

func (noSchema) Check(string, []byte) ([]model.Problem, error) { return nil, nil }

var _ port.Schema = noSchema{}

// New reads the adapter document data: the tool's name and version from the document, then the moved reader. It is an
// error, naming every refusal, for a document the reader refuses.
func New(data []byte) (*Plugin, error) {
	v, err := ordered.ParseStrict(data)
	if err != nil {
		return nil, fmt.Errorf("the adapter does not read: %w", err)
	}
	doc, ok := v.(*ordered.Map)
	if !ok {
		return nil, fmt.Errorf("the adapter is not a mapping")
	}
	tool, _ := doc.Get("tool")
	version, _ := doc.Get("version")
	ts, _ := tool.(string)
	vs, _ := version.(string)
	ad, refused := Read(noSchema{}, model.LayerEmbedded, "adapters/"+ts+"/"+vs+".yaml", data)
	if len(refused) > 0 {
		out := make([]string, len(refused))
		for i, r := range refused {
			out[i] = r.String()
		}
		return nil, fmt.Errorf("the adapter is refused: %s", strings.Join(out, "; "))
	}
	return &Plugin{ad: ad}, nil
}

// Preset writes the tool's preset from the request's entries: each entry is a binding of the tool, written as
// gragas writes a rulebook's, and the request's mark stands in place of the generated mark in the first line of each
// file (render takes it as a parameter). An adapter that names a settings input (preset.settings) is written from that input, which the request
// carries in inputs; a request without it is refused.
func (p *Plugin) Preset(req wire.PresetRequest) (wire.PresetResponse, error) {
	if why := p.markFault(req.Mark); why != "" {
		return wire.PresetResponse{}, plugin.Refusedf("/mark", "the mark %s", why)
	}
	cs, notes, err := p.candidates(req.Entries)
	if err != nil {
		return wire.PresetResponse{}, err
	}
	out := Generated{Mark: req.Mark}
	if st := p.ad.Preset.Settings; st != nil {
		i := slices.IndexFunc(req.Inputs, func(f wire.File) bool { return f.Path == st.File })
		if i < 0 {
			return wire.PresetResponse{}, plugin.Refusedf("/inputs", "the preset is written from the input %s, and the request carries none by that path", st.File)
		}
		if err := generateSettings(&out, p.ad, cs, req.Inputs[i].Data.Data); err != nil {
			return wire.PresetResponse{}, plugin.Refusedf(fmt.Sprintf("/inputs/%d", i), "%v", err)
		}
	} else {
		generateTool(&out, p.ad, cs)
	}
	resp := wire.PresetResponse{Notes: append(notes, out.Notes...)}
	slices.Sort(resp.Notes)
	for _, f := range out.Files {
		resp.Files = append(resp.Files, wire.PresetFile{Path: f.File.Path, Text: string(f.File.Data)})
	}
	return resp, nil
}

// candidates are the entries as the candidates the moved writer takes, one for each, and the notes of those not
// shipped. An entry of a target the tool does not cover is not shipped, as in gragas's collect. A value that is not
// JSON and options that are not an object are refused at their pointer.
func (p *Plugin) candidates(entries []wire.PresetEntry) ([]candidate, []string, error) {
	var cs []candidate
	var notes []string
	for i, e := range entries {
		at := fmt.Sprintf("/entries/%d", i)
		rule := model.RuleID(e.Rules[0])
		target := model.TargetType(e.Target)
		if !slices.Contains(p.ad.Targets, target) {
			notes = append(notes, fmt.Sprintf("%s: %s %s %s is not shipped: the tool does not cover that target", rule, p.ad.Tool, target, e.Key))
			continue
		}
		value, err := decode(e.Value)
		if err != nil {
			return nil, nil, plugin.Refusedf(at+"/value", "the value does not read: %v", err)
		}
		var options map[string]any
		if len(e.Options) > 0 {
			o, err := decode(e.Options)
			if err != nil {
				return nil, nil, plugin.Refusedf(at+"/options", "the options do not read: %v", err)
			}
			switch m := o.(type) {
			case nil:
			case map[string]any:
				options = m
			default:
				return nil, nil, plugin.Refusedf(at+"/options", "the options of %s are not an object, which this plugin writes", e.Key)
			}
		}
		b := model.Binding{Tool: p.ad.Tool, Target: target, Key: e.Key, Value: value, Options: options, Severity: model.Severity(e.Severity)}
		cs = append(cs, candidate{rule: rule, trial: e.Trial, b: b})
	}
	return cs, notes, nil
}

// decode is a wire value in the forms the writers take: nil, bool, string, int64 where the number is whole and
// float64 where it is not, []any and map[string]any. An absent value is nil.
func decode(raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("a second JSON value follows the first")
	}
	return numbers(v), nil
}

func numbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return n
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = numbers(e)
		}
	case []any:
		for i, e := range x {
			x[i] = numbers(e)
		}
	}
	return v
}

// markFault is why mark cannot stand in a comment of the adapter's syntax, "" when it can. The engine sets the mark, but the
// writer does not trust its shape: a control character or a line end (a CR, which YAML reads as a line break, among them)
// would end the comment and begin a line of the config, and the closer */ would end a block comment early.
func (p *Plugin) markFault(mark string) string {
	for _, r := range mark {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return fmt.Sprintf("holds the control or line-end character U+%04X, which would end its comment", r)
		}
	}
	if p.ad.Preset.Comment == model.CommentBlock && strings.Contains(mark, "*/") {
		return "holds */, which would close the block comment it stands in"
	}
	return ""
}
