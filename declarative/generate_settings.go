package declarative

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/canon"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/rendered"
)

// ErrNoSettings is the answer of Generate when an adapter names a settings
// input and the settings it was given hold none for the tool.
var ErrNoSettings = errors.New("the adapter names a settings input and none was given")

// ErrNoLinter is the answer of Generate when a tool's settings input is read
// and no active or trial rule binds a linter of the tool: an empty list would
// turn every linter off without a word.
var ErrNoLinter = errors.New("no active or trial rule binds a linter of the tool")

// ErrBetweenMarkers is the answer of Generate when the settings input holds
// something between its two marker lines: the list there is the engine's
// alone, and one file has one owner.
var ErrBetweenMarkers = errors.New("the settings input holds content between its markers")

// linterName is the shape of a key the list takes: the key is written as plain
// text, so nothing that YAML could read as more than one scalar on one line
// (a line break of any kind, a space, a colon, a quote) gets in.
var linterName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// generateSettings adds the preset file of a tool whose adapter names a
// settings input: the generated mark, then the settings text with the list of
// the linters its bindings name spliced between the markers. The list is every
// linter bound by an active or trial rule, sorted, each once, and each entry
// names the rules that bind it. A binding with no key binds no linter and is
// left out of the list. The tool's other settings are the input's own.
func generateSettings(out *Generated, ad model.Adapter, cs []candidate, text []byte) error {
	st := ad.Preset.Settings
	name := ad.Tool + ": the settings input " + st.File
	empty, err := rendered.Splice(text, st.Begin, st.End, nil)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if again, err := rendered.Splice(empty, st.Begin, st.End, nil); err != nil || !bytes.Equal(again, empty) {
		return fmt.Errorf("%s: %w", name, ErrBetweenMarkers)
	}
	if !bytes.Equal(empty, text) {
		return fmt.Errorf("%s: %w", name, ErrBetweenMarkers)
	}
	byKey := map[string][]string{}
	for _, c := range cs {
		if c.b.Key == "" {
			continue
		}
		if !linterName.MatchString(c.b.Key) {
			out.Notes = append(out.Notes, fmt.Sprintf("%s: the binding of %s at %q is not written: the key is no linter name", ad.Tool, c.rule, c.b.Key))
			continue
		}
		if !slices.Contains(byKey[c.b.Key], string(c.rule)) {
			byKey[c.b.Key] = append(byKey[c.b.Key], string(c.rule))
		}
		out.Shipped = append(out.Shipped, Shipped{Rule: c.rule, Binding: c.b, File: ad.Preset.Path})
	}
	if len(byKey) == 0 {
		return fmt.Errorf("%s: %w", ad.Tool, ErrNoLinter)
	}
	var list strings.Builder
	for _, k := range canon.SortedKeys(byKey) {
		ids := byKey[k]
		slices.SortFunc(ids, canon.Compare)
		list.WriteString("- " + k + " # " + strings.Join(ids, ", ") + "\n")
	}
	body, err := rendered.Splice(text, st.Begin, st.End, []byte(list.String()))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	p := ad.Preset
	mark := markGenerated
	if out.Mark != "" {
		mark = out.Mark
	}
	data := append([]byte(markLine(p.Comment, mark)), body...)
	out.Files = append(out.Files, PresetFile{File: model.File{Path: p.Path, Data: data}, Tool: ad.Tool})
	if w := p.Wiring; w != nil {
		out.Wiring = append(out.Wiring, Wire{Tool: ad.Tool, Import: w.Import, Install: slices.Clone(w.Install)})
	}
	return nil
}

// markLine is the generated mark as a comment of the syntax, with its line end.
func markLine(c model.CommentSyntax, mark string) string {
	switch c {
	case model.CommentHash:
		return "# " + mark + "\n"
	case model.CommentBlock:
		return "/* " + mark + " */\n"
	default:
		return "// " + mark + "\n"
	}
}
