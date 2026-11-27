package model

// The adapter's types: one enforcer adapter as the link context reads it
// from adapters/<tool>/<adapter-version>.yaml (docs/architecture.md section
// 4.1). An adapter is data the engine interprets and never runs. Each
// field's JSON name is its name in
// internal/host/assets/format/schema/v3/adapter.schema.json, and a test holds
// the two to each other in both directions.
//
// An adapter may come from a rulebook or a project, so it is data from
// elsewhere: every field says "decides" and the one decision it
// drives, or "display only" (arch.ports.19), and a test holds that each does.

// Adapter is one enforcer adapter.
type Adapter struct {
	// Format is the format version the document is written in. Decides:
	// whether the reader reads it at all.
	Format int `json:"format"`
	// Tool is the tool's name, equal to the adapter's folder. Decides: which
	// tool a binding's adapter is.
	Tool string `json:"tool"`
	// Version is the adapter's own version, equal to its file name without
	// .yaml. Decides: which adapter a tested set pins.
	Version string `json:"version"`
	// ToolVersions is the tool version range as written: one comparator, or a
	// lower and an upper bound. Decides: which tool versions the adapter
	// describes, through Range, which the reader fills from it.
	ToolVersions string `json:"tool_versions"`
	// Targets are the target types the tool covers. Decides: which bindings
	// the adapter can serve.
	Targets []TargetType `json:"targets"`
	// Extends is whether the tool's config can extend another. Decides:
	// whether a preset is extended or copied once.
	Extends bool `json:"extends"`
	// Config is how to read the effective config, one invocation per target
	// type, none when the adapter reads no config. Decides: what is run to
	// read a project's effective config, and how its output becomes entries.
	Config []ConfigRead `json:"config,omitempty"`
	// ConfigLoadsCode is whether the custom-linter check of a JSON or YAML
	// config applies to this tool: true where the tool loads linters its
	// config names, false where it does not; nil where the adapter does not
	// say, which is read as true. False skips only golangci-lint's fold and the
	// custom-linter check; the keys extends and plugins, a control character
	// in a key and a config that is not one mapping stay refused for every
	// tool, and it says nothing of what a tool may load. Decides: whether import's print-config read asks the
	// config for a custom linter before the tool is run.
	ConfigLoadsCode *bool `json:"config_loads_code,omitempty"`
	// Run is how to run the tool over fixtures. Decides: the argv of a proof
	// run and how its output becomes diagnostics.
	Run Invocation `json:"run"`
	// VersionArgv is the argv, literals only, after which the tool prints its
	// version. Decides: the version a found tool is taken to be, read as the
	// first major.minor.patch on the first line it prints, which picks the
	// adapter whose ToolVersions holds it.
	VersionArgv []string `json:"version_argv"`
	// Severities are the tool's severities. Decides: the severity a binding
	// ships at, and a trial binding's.
	Severities SeverityScale `json:"severities"`
	// Values are the values an entry of the effective config takes that read
	// as off, and the order of the others, empty where the adapter names
	// neither. Decides: whether an entry whose value differs from
	// the preset's is tighter or weaker, or has no word and a reason.
	Values ValueScale `json:"values,omitzero"`
	// DocsURL is the option that sets a diagnostic's docs URL, nil where the
	// tool's URL is fixed. Decides: whether a preset sets the link, and
	// with which option.
	DocsURL *DocsOption `json:"docs_url,omitempty"`
	// ResolveFrom is the folder, relative to the config set, a run links
	// read-only into its scratch folder so the tool finds its plugins, "" for
	// none. Decides: which folder is linked.
	ResolveFrom string `json:"resolve_from,omitempty"`
	// Preset is how a preset of the tool is written: its file, its format, the
	// comment of the generated mark, where each part of an entry goes and the
	// wiring that makes the project use it. Decides: what the
	// generated preset is called, its form, and where it is placed.
	Preset Preset `json:"preset"`
	// Paths are where a project's own tool lives relative to its config set,
	// per ecosystem, tried before the tools file. Decides: where the tool is
	// looked for first.
	Paths []ToolPath `json:"paths,omitempty"`

	// Range is ToolVersions parsed. Decides: whether a found tool's version
	// is one the adapter describes.
	Range VersionRange `json:"-"`
	// Origin is the layer and the file the adapter was read from, filled by
	// the reader. Decides: which layer's adapter wins, and the place a
	// refusal names.
	Origin AdapterOrigin `json:"-"`
}

// Placeholder is an argv element that the engine fills at run time. It
// stands alone as one element; a literal holds none of its characters.
type Placeholder string

// The three placeholders.
const (
	// PlaceFixtures is the fixture paths, one element each.
	PlaceFixtures Placeholder = "{fixtures}"
	// PlaceConfig is the config path.
	PlaceConfig Placeholder = "{config}"
	// PlaceOutput is the path the tool writes its output to.
	PlaceOutput Placeholder = "{output}"
)

// Placeholders is the closed set.
func Placeholders() []Placeholder { return []Placeholder{PlaceFixtures, PlaceConfig, PlaceOutput} }

// ConfigRead is how one target type's effective config is read.
type ConfigRead struct {
	// Target is the target type. Decides: which target this invocation reads.
	Target TargetType `json:"target"`
	// Argv is the arguments after the tool's path, each a literal or a
	// placeholder; nil where Files reads the config instead. Decides: what
	// is run.
	Argv []string `json:"argv,omitempty"`
	// Files is where the entries are files the project's own config names,
	// read through the Tree port and never by running the tool; nil where
	// Argv prints the config. A read has one of the two.
	// Decides: which files are read as entries.
	Files *FileRead `json:"files,omitempty"`
	// Format is the form the tool prints its config in, or, with Files, the
	// form of the project config and of each entry file. Decides: which
	// parser reads it.
	Format ConfigFormat `json:"format"`
	// Entries is where the tool's entries are in the printed config. With
	// Files the document is a list holding each entry file's mapping, in path
	// order. Decides: which entries a project's effective config holds.
	Entries EntryMap `json:"entries"`
}

// PrintedReads is a's config reads that run the tool and read its print: a
// read of entry files runs nothing. Decides: which config reads
// a recording is made of.
func (a Adapter) PrintedReads() []ConfigRead {
	var out []ConfigRead
	for _, c := range a.Config {
		if c.Files == nil {
			out = append(out, c)
		}
	}
	return out
}

// FileRead is a config read of a tool that prints no effective config and
// keeps each entry in a file of its own, in folders its project config names:
// ast-grep's rule files, which ruleDirs of sgconfig.yml names.
type FileRead struct {
	// Config is the project config's path, slash-separated and relative to
	// the project root. Decides: which file names the folders.
	Config string `json:"config"`
	// Dirs is the JSON pointer, in the project config, of the list of
	// folders, each relative to the project root. Decides: which folders
	// hold the entry files.
	Dirs string `json:"dirs"`
	// Ext is the file name endings, each with its dot, of the files in those
	// folders that are entries. Decides: which files of a folder are read.
	Ext []string `json:"ext"`
}

// ConfigFormat is the form a tool prints its config in.
type ConfigFormat string

// The config forms. ConfigYAML names no YAML version, so a plain scalar the
// two versions read apart is refused; ConfigYAML12 is for a tool that reads
// YAML 1.2's core schema, where such a scalar is the value that schema gives.
const (
	ConfigJSON   ConfigFormat = "json"
	ConfigYAML   ConfigFormat = "yaml"
	ConfigYAML12 ConfigFormat = "yaml-1.2"
)

// ConfigFormats is the closed set.
func ConfigFormats() []ConfigFormat {
	return []ConfigFormat{ConfigJSON, ConfigYAML, ConfigYAML12}
}

// EntryMap is where a tool's entries are in its printed config, each a JSON
// pointer (RFC 6901). The empty pointer is the whole value it is read from.
type EntryMap struct {
	// Each is the object or list of entries, from the document's root. In an
	// object, each member's name is the entry's key. Decides: which values
	// are entries.
	Each string `json:"each"`
	// Key is, in a list, where an entry's key is, from the entry; "" in an
	// object. Decides: the entry's key.
	Key string `json:"key,omitempty"`
	// Value is where an entry's value is, from the entry. In an object whose
	// entry map names nothing but Each, the value is the member's whole
	// value. Decides: the entry's value.
	Value string `json:"value,omitempty"`
	// Options is where an entry's options are, from the entry. Decides: the
	// entry's options.
	Options string `json:"options,omitempty"`
	// OptionsTop is, for a read of entry files, whether an entry's options
	// are the members at the top of its file that Key, Value and Severity do
	// not name, as a files preset with options_top writes them.
	// A map names this or Options, never both. Decides: the
	// entry's options.
	OptionsTop bool `json:"options_top,omitempty"`
	// Severity is where an entry's severity is, from the entry. Decides: the
	// entry's severity.
	Severity string `json:"severity,omitempty"`
	// As is the value, as JSON text, of each entry read at Each, where the
	// printed config gives none: a tool that lists its linters on and off in
	// two lists gives each by the list it is in. Decides: the
	// entry's value.
	As string `json:"as,omitempty"`
	// Also is further lists of entries, each with its own value. Decides:
	// which more values are entries.
	Also []EntrySource `json:"also,omitempty"`
}

// EntrySource is one more list of entries in a printed config, each taking
// the value As gives.
type EntrySource struct {
	// Each is the object or list, from the document's root. Decides: which
	// values are entries.
	Each string `json:"each"`
	// Key is, in a list, where an entry's key is; "" in an object. Decides:
	// the entry's key.
	Key string `json:"key,omitempty"`
	// As is the value of every entry of the list, as JSON text. Decides: the
	// entry's value.
	As string `json:"as"`
}

// Invocation is how the tool is run over fixtures.
type Invocation struct {
	// Argv is the arguments after the tool's path, each a literal or a
	// placeholder, {fixtures} among them. Decides: what is run.
	Argv []string `json:"argv"`
	// Output is what the tool prints and how a diagnostic is read from it.
	// Decides: which parser reads it.
	Output AdapterOutput `json:"output"`
}

// OutputFormat is what a tool prints for its diagnostics.
type OutputFormat string

// The three output formats.
const (
	OutputSARIF OutputFormat = "sarif"
	OutputJSON  OutputFormat = "json"
	OutputLines OutputFormat = "lines"
)

// OutputFormats is the closed set.
func OutputFormats() []OutputFormat { return []OutputFormat{OutputSARIF, OutputJSON, OutputLines} }

// AdapterOutput is how a diagnostic is read from what a tool prints.
type AdapterOutput struct {
	// Format is what the tool prints. Decides: which parser reads it.
	Format OutputFormat `json:"format"`
	// Each is, for json, the list of diagnostics in the document, a JSON
	// pointer. Decides: which values are diagnostics.
	Each string `json:"each,omitempty"`
	// Fields is, for json, where each named field is in one diagnostic.
	// Decides: which value is a diagnostic's entry, place and severity.
	Fields *Fields `json:"fields,omitempty"`
	// Pattern is, for lines, the expression one line matches, its named
	// capture groups named as Fields' fields are. Decides: which lines are
	// diagnostics, and which part of one is its entry, place and severity.
	Pattern string `json:"pattern,omitempty"`
	// OnlyForm is, for lines, the adapter's statement that the tool prints
	// no machine form (arch.ports.20). Decides: whether lines may be read.
	OnlyForm bool `json:"only_form,omitempty"`
	// RanCodes are the exit codes that mean the tool ran, for example 0 for no
	// findings and 1 for findings. Decides: whether a side of a proof run is
	// counted; a code outside the list refuses the batch. Empty names none, and
	// a non-zero code with nothing readable is refused.
	RanCodes []int `json:"ran_codes,omitempty"`
	// ErrorKeys are the keys the tool gives its own failures (a parse, a type
	// check, a config). Decides: a finding of one on either side of a proof
	// run refuses the batch instead of counting.
	ErrorKeys []string `json:"error_keys,omitempty"`
}

// Field is a named field of a diagnostic, or of a config entry.
type Field string

// The fields.
const (
	FieldKey      Field = "key"
	FieldMessage  Field = "message"
	FieldPath     Field = "path"
	FieldLine     Field = "line"
	FieldColumn   Field = "column"
	FieldSeverity Field = "severity"
	FieldURL      Field = "url"
)

// FieldNames is the closed set.
func FieldNames() []Field {
	return []Field{FieldKey, FieldMessage, FieldPath, FieldLine, FieldColumn, FieldSeverity, FieldURL}
}

// Fields is where each named field is in one diagnostic of a JSON document,
// each a JSON pointer from the diagnostic, "" for a field it does not have.
// Key is always set.
type Fields struct {
	// Key is the tool's entry the diagnostic belongs to. Decides: which
	// binding the diagnostic proves.
	Key string `json:"key,omitempty"`
	// Message is the diagnostic's text. Decides: where the text is read
	// from; the text itself is display only.
	Message string `json:"message,omitempty"`
	// Path is the file the diagnostic is in. Decides: which fixture it is in.
	Path string `json:"path,omitempty"`
	// Line is the diagnostic's line. Decides: which line of the fixture.
	Line string `json:"line,omitempty"`
	// Column is the diagnostic's column. Decides: where the column is read
	// from; the column is display only.
	Column string `json:"column,omitempty"`
	// Severity is the diagnostic's severity. Decides: the severity observed.
	Severity string `json:"severity,omitempty"`
	// URL is the diagnostic's docs URL. Decides: where the link is read from;
	// the link itself is display only.
	URL string `json:"url,omitempty"`
}

// SeverityForm is how a tool keeps an entry's severity.
type SeverityForm string

// The two severity forms.
const (
	// SeverityLevel is a severity on each entry.
	SeverityLevel SeverityForm = "level"
	// SeverityTable is a table of overrides keyed by entry.
	SeverityTable SeverityForm = "table"
)

// SeverityForms is the closed set.
func SeverityForms() []SeverityForm { return []SeverityForm{SeverityLevel, SeverityTable} }

// SeverityScale is a tool's severities.
type SeverityScale struct {
	// Form is how the tool keeps an entry's severity. Decides: how a preset
	// writes a trial binding at warning.
	Form SeverityForm `json:"form"`
	// Levels are the tool's own severity words, each with the engine's
	// severity it stands for. Decides: the word a preset writes, and the
	// severity a diagnostic is read as.
	Levels []SeverityLevelName `json:"levels"`
	// Warning is the name of the level a trial binding ships at, one of
	// Levels' names; "" where the tool has no warning level. Decides:
	// whether a trial binding ships at warning or at the tool's only
	// severity.
	Warning string `json:"warning,omitempty"`
	// Default is the name of the level an entry file of a files read stands
	// at when it names no severity, one of Levels' names, as the tool reads
	// such a file; "" where the tool has none, and such a file is then
	// refused. Decides: the severity of an entry file that
	// names none.
	Default string `json:"default,omitempty"`
	// Off are the tool's own words for an entry that is off (written off_words,
	// since a bare off is a boolean in YAML), none where the
	// tool has no off word (an entry then reads as off for no value). Decides:
	// which severity an observed entry carries is off, so weaker than any
	// severity a preset sets.
	Off []string `json:"off_words,omitempty"`
}

// ValueScale is how a tool's entry values stand against one another: each a
// scalar as the adapter writes it (a bool, a number or a text), none twice,
// and none both off and in the order.
type ValueScale struct {
	// Off are the values that read as off (written off_values, since a bare
	// off is a boolean in YAML), below every value of the order. Decides:
	// that an entry at one of them is weaker than a preset's value in the
	// order.
	Off []any `json:"off_values,omitempty"`
	// Order are the values the tool reads as on, from the loosest to the
	// tightest. Decides: whether an entry's value is tighter or weaker than
	// the preset's; a value in neither list has no word.
	Order []any `json:"order,omitempty"`
	// Keys are the keys whose values stand otherwise: each has its own off
	// values and order, used for that key in place of the two above, so a tool
	// whose options run in different directions (one is tighter on, another
	// tighter off) says so for each. Decides: the direction of
	// that key's values; a key not here is ranked by the adapter's own.
	Keys []KeyScale `json:"keys,omitempty"`
}

// KeyScale is how one entry key's values stand against one another, as the
// ValueScale does for the adapter.
type KeyScale struct {
	// Key is the entry's key. Decides: which entry the scale ranks.
	Key string `json:"key"`
	// Off are the key's values that read as off. Decides: that value is
	// weaker than one of the order.
	Off []any `json:"off_values,omitempty"`
	// Order are the key's values from the loosest to the tightest. Decides:
	// whether a value is tighter or weaker than the preset's.
	Order []any `json:"order,omitempty"`
}

// SeverityLevelName is one of a tool's severity words.
type SeverityLevelName struct {
	// Name is the tool's word. Decides: what a preset writes and what a
	// diagnostic's severity is matched against.
	Name string `json:"name"`
	// Is is the engine's severity it stands for. Decides: the severity
	// observed.
	Is Severity `json:"is"`
}

// DocsOption is the option of a tool's config that sets a diagnostic's docs
// URL.
type DocsOption struct {
	// Option is the option's name. Decides: which option a preset sets.
	Option string `json:"option"`
}

// Ecosystem is a language's package world, where a project's own tool can
// live.
type Ecosystem string

// The three ecosystems.
const (
	EcosystemNode   Ecosystem = "node"
	EcosystemGo     Ecosystem = "go"
	EcosystemPython Ecosystem = "python"
)

// Ecosystems is the closed set.
func Ecosystems() []Ecosystem { return []Ecosystem{EcosystemNode, EcosystemGo, EcosystemPython} }

// ToolPath is where a project's own tool lives in one ecosystem.
type ToolPath struct {
	// Ecosystem is the ecosystem. Decides: which projects the path is tried
	// for.
	Ecosystem Ecosystem `json:"ecosystem"`
	// Path is the tool's path, slash-separated and relative to the config
	// set. Decides: where the tool is looked for. Exclusive with Package: an
	// entry holds one of the two.
	Path string `json:"path,omitempty"`
	// Package is the npm package whose package.json bin names the script the
	// tool runs as node <script>. Decides: where the script is
	// looked for, below node_modules.
	Package string `json:"package,omitempty"`
	// Bin is the key of a package.json bin that is a map. It is named, never
	// guessed, and only with Package.
	// Decides: which script of the package runs.
	Bin string `json:"bin,omitempty"`
}

// VersionRange is a tool version range parsed: a lower bound, an upper bound,
// or both. A bound's Version is "" when the range has none on that side.
type VersionRange struct {
	// Lower is the lowest version the range holds. Decides: with Upper,
	// whether a version is in the range.
	Lower VersionBound
	// Upper is the highest. Decides: with Lower, whether a version is in the
	// range.
	Upper VersionBound
}

// VersionBound is one side of a VersionRange.
type VersionBound struct {
	// Version is the bound, major.minor.patch, "" for none. Decides: the
	// bound.
	Version string
	// Included is whether Version itself is in the range. Decides: whether
	// the bound is closed.
	Included bool
}

// AdapterLayer is where an adapter was read from. A more specific layer wins:
// a project's over a rulebook's over the engine's own.
type AdapterLayer string

// The three layers, from the least specific.
const (
	// LayerEmbedded is the adapters the engine embeds.
	LayerEmbedded AdapterLayer = "embedded"
	// LayerRulebook is a rulebook's adapters/ folder.
	LayerRulebook AdapterLayer = "rulebook"
	// LayerProject is a project's adapters/ folder.
	LayerProject AdapterLayer = "project"
)

// AdapterLayers is the closed set, from the least specific.
func AdapterLayers() []AdapterLayer {
	return []AdapterLayer{LayerEmbedded, LayerRulebook, LayerProject}
}

// AdapterOrigin is where an adapter was read from.
type AdapterOrigin struct {
	// Layer is the layer. Decides: which layer's adapter wins.
	Layer AdapterLayer
	// Path is the file, adapters/<tool>/<version>.yaml, slash-separated from
	// the layer's root. Display only.
	Path string
}

// Preset is how one tool's preset file is written. It is data the engine
// reads when it writes the file; the engine writes every word of the file
// itself, so the adapter holds no text of it (arch.ports.2).
type Preset struct {
	// Path is the preset file, slash-separated and relative to the config
	// set. Decides: which file the generated preset is written to.
	Path string `json:"path"`
	// Format is the form the file is written in. Decides: how its entries
	// are encoded, and which comment syntax it can hold.
	Format PresetFormat `json:"format"`
	// Comment is the syntax of a comment in the file. Decides: how the
	// generated mark is written; the reader holds that the format can hold it.
	Comment CommentSyntax `json:"comment"`
	// Placement is where an entry, its value, its options and its severity go.
	// Decides: where each part of a binding is written in the file.
	Placement Placement `json:"placement"`
	// Lines are the fixed top-level lines the file carries before anything
	// the engine places, none where the tool needs none.
	// Decides: which keys the generated file starts with, and their values.
	Lines []FixedLine `json:"lines,omitempty"`
	// Settings is the tool's own settings input, nil where the adapter names
	// none: a file of the preset folder that holds the tool's hand-kept
	// settings and, between two marker lines, the place for the list the
	// engine writes. Decides: that the preset's main file is that file with
	// the generated list spliced in, and not a file built from the bindings
	// alone.
	Settings *PresetSettings `json:"settings,omitempty"`
	// Wiring is how a project comes to use the preset, nil where the
	// tool needs none. Decides: the import the project's config gets and the
	// install the tool has.
	Wiring *Wiring `json:"wiring,omitempty"`
}

// PresetSettings names a tool's settings input and the two marker lines that
// hold the generated list in it.
type PresetSettings struct {
	// File is the settings file, slash-separated and relative to the tool's
	// preset folder. Decides: which file the engine reads for the settings.
	File string `json:"file"`
	// Begin is the whole line that opens the generated list, without its
	// leading blanks. Decides: where the list starts.
	Begin string `json:"begin"`
	// End is the whole line that closes the generated list, unlike Begin.
	// Decides: where the list ends.
	End string `json:"end"`
}

// PresetFormat is the form a preset file is written in. Strict JSON is jsonc
// here: every JSON document is a JSONC one.
type PresetFormat string

// The three preset formats.
const (
	PresetYAML  PresetFormat = "yaml"
	PresetJSONC PresetFormat = "jsonc"
	PresetJS    PresetFormat = "js"
)

// PresetFormats is the closed set.
func PresetFormats() []PresetFormat { return []PresetFormat{PresetYAML, PresetJSONC, PresetJS} }

// CommentSyntax is how a comment is written in a preset file.
type CommentSyntax string

// The three comment syntaxes.
const (
	// CommentHash is a line that starts with #.
	CommentHash CommentSyntax = "hash"
	// CommentSlash is a line that starts with //.
	CommentSlash CommentSyntax = "slash"
	// CommentBlock is a text between /* and */.
	CommentBlock CommentSyntax = "block"
)

// CommentSyntaxes is the closed set.
func CommentSyntaxes() []CommentSyntax {
	return []CommentSyntax{CommentHash, CommentSlash, CommentBlock}
}

// Holds is whether a file in format f can hold a comment in syntax c.
func (f PresetFormat) Holds(c CommentSyntax) bool {
	switch f {
	case PresetYAML:
		return c == CommentHash
	case PresetJSONC, PresetJS:
		return c == CommentSlash || c == CommentBlock
	}
	return false
}

// Shape is how a preset file holds its entries.
type Shape string

// The three shapes.
const (
	// ShapeList is a list at a place in the file: each item is an entry, a
	// text or a map with its key at Key.
	ShapeList Shape = "list"
	// ShapeObject is an object at a place in the file: each member is an
	// entry, its name the key.
	ShapeObject Shape = "object"
	// ShapeFiles is one file per entry in a folder.
	ShapeFiles Shape = "files"
)

// Shapes is the closed set.
func Shapes() []Shape { return []Shape{ShapeList, ShapeObject, ShapeFiles} }

// Placement is where each part of an entry is written, each a JSON pointer
// (RFC 6901), as EntryMap reads them.
type Placement struct {
	// Shape is how the file holds its entries. Decides: which of the fields
	// below the file takes: a list and an object take Each, files take Dir.
	Shape Shape `json:"shape"`
	// Each is, for a list or an object, the place of the entries from the
	// document's root. Decides: where entries are written.
	Each string `json:"each,omitempty"`
	// Dir is, for files, the folder of the entry files, relative to the
	// preset's. Decides: which folder an entry's file is written to.
	Dir string `json:"dir,omitempty"`
	// Key is, in a list of maps or in a file, where an entry's key is, from
	// the entry. Decides: where the key is written.
	Key string `json:"key,omitempty"`
	// Value is where an entry's value is, from the entry. Decides: where the
	// value is written.
	Value string `json:"value,omitempty"`
	// Options is where an entry's options are, from the entry. Decides: where
	// the options are written.
	Options string `json:"options,omitempty"`
	// OptionsAt is, for a list, where a binding's options go, from the
	// document's root, with the binding's key as the one step written {key}:
	// a list's entries are bare names with no member of their own to hold
	// them (golangci-lint's is /linters/settings/{key}). A
	// placement names this or Options, never both. Decides: where the
	// options of a list preset's binding are written.
	OptionsAt string `json:"options_at,omitempty"`
	// OptionsTop is, for files, whether each member of a binding's options is
	// written as a member at the top of its entry file, under its own name:
	// an ast-grep rule file holds its language beside id, rule and severity.
	// A placement names this, Options or OptionsAt, one of
	// them. Decides: where the options of a files preset's binding are
	// written.
	OptionsTop bool `json:"options_top,omitempty"`
	// Severity is where an entry's severity is, from the entry, where the
	// tool keeps it on the entry (severities.form level). Decides: where the
	// severity is written.
	Severity string `json:"severity,omitempty"`
	// SeverityTable is where the table of severity overrides is, from the
	// document's root, where the tool keeps it apart (severities.form table).
	// Decides: where a trial binding's severity is written.
	SeverityTable string `json:"severity_table,omitempty"`
	// SeverityItem is the shape of one item of the table of severity
	// overrides, the zero value where the adapter declares none.
	// Decides: how a trial binding's override is written in the table; with no
	// item shape the engine writes no table and says so.
	SeverityItem SeverityItem `json:"severity_item,omitzero"`
}

// SeverityItem is how one override of the severity table is written.
type SeverityItem struct {
	// Shape is whether the table is a list of maps (ShapeList) or an object
	// keyed by entry (ShapeObject). Decides: which of the fields below apply.
	Shape Shape `json:"shape,omitempty"`
	// Key is, for a list, where the entry's key is in an item, a JSON pointer.
	// Decides: where the entry's name is written in an item.
	Key string `json:"key,omitempty"`
	// KeyForm is, for a list, whether the key place holds one text or a list
	// of texts. Decides: whether the name is written alone or as a list of one.
	KeyForm KeyForm `json:"key_form,omitempty"`
	// Level is where the level is, a JSON pointer: from an item for a list,
	// from the member for an object, where empty means the member itself.
	// Decides: where the level is written.
	Level string `json:"level,omitempty"`
}

// KeyForm is whether a key place holds one text or a list of texts.
type KeyForm string

// The two key forms.
const (
	// KeyOne is one text.
	KeyOne KeyForm = "one"
	// KeyList is a list of texts.
	KeyList KeyForm = "list"
)

// KeyForms is the closed set.
func KeyForms() []KeyForm { return []KeyForm{KeyOne, KeyList} }

// FixedLine is one top-level key the preset file carries, with the same value
// in every file written: the tool's version marker, or the folder its main file
// reads rules from. Exactly one of Value and Items is set. A text stays a text:
// the engine writes it quoted where the format would read it as another type.
type FixedLine struct {
	// Key is the name. Decides: the key written.
	Key string `json:"key"`
	// Value is a text value. Decides: the value written.
	Value string `json:"value,omitempty"`
	// Items is a list of texts. Decides: the list written.
	Items []string `json:"items,omitempty"`
}

// Wiring is how a project comes to use a preset.
type Wiring struct {
	// Import is where the project's own config names the preset it extends,
	// nil where the tool's config cannot extend another. Decides: where the
	// reference to the preset is written in the project's config.
	Import *ImportAt `json:"import,omitempty"`
	// Install is the arguments, literals only, of the tool's own documented
	// install, none where the adapter names none. Decides: the line a project
	// is told to run to get the tool; it is never run by the engine.
	Install []string `json:"install,omitempty"`
}

// ImportForm is how a config names the config it extends.
type ImportForm string

// The two import forms.
const (
	// ImportString is one text.
	ImportString ImportForm = "string"
	// ImportList is a list of texts.
	ImportList ImportForm = "list"
)

// ImportForms is the closed set.
func ImportForms() []ImportForm { return []ImportForm{ImportString, ImportList} }

// ImportAt is where a project's config names the config it extends.
type ImportAt struct {
	// At is the place from the project config's root, a JSON pointer.
	// Decides: where the reference is written.
	At string `json:"at"`
	// Form is whether the place holds one text or a list. Decides: whether a
	// reference replaces the text or is added to the list.
	Form ImportForm `json:"form"`
}
