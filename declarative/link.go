// Package link is the enforcement link's context: the one Go reader of the
// enforcer adapters, which are data and never code (docs/architecture.md
// section 4.1). A new tool arrives as a new document, with no change here,
// so nothing in this package names a tool.
//
// An adapter may come from a rulebook or a project, which the engine does not
// control, so the reader fails closed: a document is read whole
// or not at all, every key it does not name is refused, every value must fit
// its shape, an argument is a literal or one placeholder of a closed set, and
// size is capped before anything is parsed. The YAML is read by
// ordered.ParseStrict, so an anchor, an alias or a tag is refused before it
// can expand. The document is held to the Schema port as well, so the Go
// reader and the schema each refuse what the other would miss. Each refusal
// names the file, the line and the check that refused it.
//
// The reader reads no file. The entry point hands it the bytes of each layer:
// the engine's embedded adapters, a rulebook's adapters/ folder and a
// project's, and the more specific layer's adapter wins.
package declarative

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/port"
)

// SchemaName is the adapter's schema as the Schema port names it.
const SchemaName = "v3/adapter.schema.json"

// The caps. A document over MaxDocument bytes is refused unread, and a layer
// of more than MaxDocuments files is refused whole.
const (
	MaxDocument  = 64 << 10
	MaxDocuments = 256
	maxArgv      = 64
	maxList      = 32
	maxText      = 256
	maxPattern   = 1024
	maxRefusals  = 20
)

// Check names the check that refused a document. The set is closed.
type Check string

// The checks.
const (
	// CheckSize is a document or a layer over its cap.
	CheckSize Check = "size"
	// CheckPlace is a file whose path is not adapters/<tool>/<version>.yaml,
	// or whose tool or version differs from its path.
	CheckPlace Check = "place"
	// CheckYAML is a document that is not plain YAML: unreadable, or holding
	// an anchor, an alias, a tag or a key written twice.
	CheckYAML Check = "yaml"
	// CheckFormat is a document of another format version.
	CheckFormat Check = "format"
	// CheckKey is a key the adapter's shape does not name.
	CheckKey Check = "key"
	// CheckConditional is a key that would make a value depend on something.
	CheckConditional Check = "conditional"
	// CheckReference is a key that would make one place stand for another.
	CheckReference Check = "reference"
	// CheckExpression is a value holding the characters an expression is
	// written with.
	CheckExpression Check = "expression"
	// CheckPlaceholder is an argument that names no placeholder of the
	// closed set, or a placeholder used where it does not belong.
	CheckPlaceholder Check = "placeholder"
	// CheckShape is a value of the wrong type, outside its set, or not in
	// its form.
	CheckShape Check = "shape"
	// CheckRange is a tool version range that does not parse or holds no
	// version.
	CheckRange Check = "version-range"
	// CheckOutput is an output whose fields do not fit its format.
	CheckOutput Check = "output"
	// CheckSchema is a document the Schema port refuses.
	CheckSchema Check = "schema"
	// CheckLayer is a layer handed out of order or twice, or a file handed
	// twice.
	CheckLayer Check = "layer"
	// CheckOverride is a rulebook's or a project's adapter that is more than
	// an embedded adapter with other paths.
	CheckOverride Check = "override"
)

// Refusal is one reason an adapter document is not read: the file, slash
// separated from its layer's root, the line (0 when the whole file is meant),
// the check and a sentence for a person.
type Refusal struct {
	Path  string
	Line  int
	Check Check
	Msg   string
}

// String is the refusal as a person reads it: path:line: check: sentence.
func (r Refusal) String() string {
	if r.Line > 0 {
		return fmt.Sprintf("%s:%d: %s: %s", r.Path, r.Line, r.Check, r.Msg)
	}
	return fmt.Sprintf("%s: %s: %s", r.Path, r.Check, r.Msg)
}

// placeRE is the path an adapter stands at: adapters/<tool>/<version>.yaml.
var placeRE = regexp.MustCompile(`^adapters/([a-z0-9]+(?:-[a-z0-9]+)*)/((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))\.yaml$`)

// Read reads the one adapter document data, which stands at path in layer,
// and checks it against the schema the Schema port s holds. It answers the
// adapter, with its Range and Origin filled, or every reason it is refused, at
// most twenty, and never both.
func Read(s port.Schema, layer model.AdapterLayer, path string, data []byte) (model.Adapter, []Refusal) {
	r := &reader{path: path}
	if len(data) > MaxDocument {
		r.refuse(0, CheckSize, "the document is %d bytes, over the cap of %d", len(data), MaxDocument)
		return model.Adapter{}, r.out
	}
	place := placeRE.FindStringSubmatch(path)
	if place == nil {
		r.refuse(0, CheckPlace, "an adapter stands at adapters/<tool>/<adapter-version>.yaml, the tool a lowercase name and the version major.minor.patch")
		return model.Adapter{}, r.out
	}
	v, err := ordered.ParseStrict(data)
	if err != nil {
		r.yaml(err)
		return model.Adapter{}, r.out
	}
	m, ok := v.(*ordered.Map)
	if !ok {
		r.refuse(1, CheckShape, "the document must be a mapping of the adapter's fields")
		return model.Adapter{}, r.out
	}
	a := r.adapter(m)
	if len(r.out) == 0 {
		r.place(m, a, place[1], place[2])
	}
	if len(r.out) == 0 {
		r.schema(s, SchemaName, v)
	}
	if len(r.out) > 0 {
		return model.Adapter{}, r.out
	}
	a.Origin = model.AdapterOrigin{Layer: layer, Path: path}
	return a, nil
}

// yaml refuses a document ParseStrict could not read, at the line it names.
func (r *reader) yaml(err error) {
	var se *ordered.SyntaxError
	if !errors.As(err, &se) {
		r.refuse(0, CheckYAML, "the document could not be read: %v", err)
		return
	}
	r.refuse(se.Line, CheckYAML, "%s", strings.TrimPrefix(se.Detail, fmt.Sprintf("line %d: ", se.Line)))
}

// reader gathers the refusals of one document. kind names the document in
// its refusals, "an adapter" when it is "" (an adapter's reader), and kinds
// names its kind in the plural.
type reader struct {
	path        string
	out         []Refusal
	kind, kinds string
}

// an is the document as a refusal names it.
func (r *reader) an() string {
	if r.kind == "" {
		return "an adapter"
	}
	return r.kind
}

// many is the document's kind in the plural.
func (r *reader) many() string {
	if r.kinds == "" {
		return "adapters"
	}
	return r.kinds
}

// refuse records a refusal, up to the cap.
func (r *reader) refuse(line int, c Check, format string, args ...any) {
	if len(r.out) >= maxRefusals {
		return
	}
	r.out = append(r.out, Refusal{Path: r.path, Line: line, Check: c, Msg: fmt.Sprintf(format, args...)})
}

// place refuses a document whose tool or version is not the one its path
// names.
func (r *reader) place(m *ordered.Map, a model.Adapter, tool, version string) {
	if a.Tool != tool {
		r.refuse(m.KeyLine("tool"), CheckPlace, "the tool is %s, and the file stands in the folder %s", a.Tool, tool)
	}
	if a.Version != version {
		r.refuse(m.KeyLine("version"), CheckPlace, "the version is %s, and the file is named %s.yaml", a.Version, version)
	}
}

// schema holds the document to the adapter's schema through the port. The Go
// reader has passed it, so a problem here is a drift between the two, and it
// refuses the document all the same. Each problem's line, which is the
// line of the JSON the port was handed, is mapped back to the YAML's.
func (r *reader) schema(s port.Schema, name string, v any) {
	doc := ordered.JSON(v, 1)
	problems, err := s.Check(name, []byte(doc))
	if err != nil {
		r.refuse(0, CheckSchema, "the schema could not check it: %v", err)
		return
	}
	if len(problems) == 0 {
		return
	}
	back := lineMap(v, doc)
	for _, p := range problems {
		r.refuse(back[p.Line], CheckSchema, "%s", p.Msg)
	}
}

// lineMap maps each line of doc, the JSON encoding of v, that holds a value
// of v to the line of that value in v's YAML.
func lineMap(v any, doc string) map[int]int {
	out := map[int]int{}
	j, err := ordered.ParseYAML([]byte(doc))
	if err != nil {
		return out
	}
	var walk func(x any, at []string)
	walk = func(x any, at []string) {
		out[ordered.LineAt(j, at)] = ordered.LineAt(v, at)
		switch n := x.(type) {
		case *ordered.Map:
			for _, k := range n.Keys() {
				walk(n.Val(k), append(slices.Clip(at), k))
			}
		case []any:
			for i, e := range n {
				walk(e, append(slices.Clip(at), fmt.Sprint(i)))
			}
		}
	}
	walk(v, nil)
	return out
}
