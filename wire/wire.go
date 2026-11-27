// Package wire holds the types of plugin contract 1's wire: the request and response of each of the
// five operations, as contract/v1/wire.schema.json writes them.
//
// These types are hand-written to the schema's exact shapes until gragas's generator writes them;
// its output replaces this file and the package keeps its names, so no plugin's import moves. A test holds each
// enum below equal to the pinned schema's, so a copy that falls behind is red.
//
// Null. No field is ever null; an absent optional field means "none". The one exception is inside a Value: there
// null is a value the tool may hold, and it differs from an absent Value (an empty one).
package wire

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Value is a value whose shape is the tool's: an entry's value and options, and rank's preset and found. The
// engine treats it as opaque, and only the plugin reads inside it. An absent Value has no bytes; a JSON null is
// the four bytes "null".
type Value = json.RawMessage

// Op is an operation of the contract.
type Op string

// The five operations.
const (
	OpPlan     Op = "plan"
	OpEntries  Op = "entries"
	OpFindings Op = "findings"
	OpRank     Op = "rank"
	OpPreset   Op = "preset"
)

// Ops are the operations in the order the contract names them.
func Ops() []Op { return []Op{OpPlan, OpEntries, OpFindings, OpRank, OpPreset} }

// Target is the kind of file a rule applies to.
type Target string

// Targets are every target the contract names.
func Targets() []Target {
	return []Target{"source", "config", "migration", "container", "workflow", "iac", "api-spec", "prose", "commit",
		"telemetry-registry", "dependency", "package"}
}

// Severity is one of the engine's three words. Off is an entry with On false.
type Severity string

// The severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Severities are the three words in the contract's order.
func Severities() []Severity { return []Severity{SeverityError, SeverityWarning, SeverityInfo} }

// Level is how isolated a run is.
type Level string

// The two run levels.
const (
	LevelIsolated  Level = "isolated"
	LevelSandboxed Level = "sandboxed"
)

// Levels are the run levels in the contract's order.
func Levels() []Level { return []Level{LevelIsolated, LevelSandboxed} }

// For is what a plan is for: the tool's config, or a run of the tool.
type For string

// The two purposes of a plan.
const (
	ForConfig For = "config"
	ForRun    For = "run"
)

// Fors are the purposes in the contract's order.
func Fors() []For { return []For{ForConfig, ForRun} }

// Aspect is which part of an entry a rank pair compares.
type Aspect string

// The two aspects.
const (
	AspectValue   Aspect = "value"
	AspectOptions Aspect = "options"
)

// Aspects are the aspects in the contract's order.
func Aspects() []Aspect { return []Aspect{AspectValue, AspectOptions} }

// Word is rank's answer for one pair.
type Word string

// The four words.
const (
	Tighter Word = "tighter"
	Weaker  Word = "weaker"
	Same    Word = "same"
	Unknown Word = "unknown"
)

// Words are the words in the contract's order.
func Words() []Word { return []Word{Tighter, Weaker, Same, Unknown} }

// Kind is the kind of a result: a run or a read.
type Kind string

// The two kinds of step and of result.
const (
	KindRun  Kind = "run"
	KindRead Kind = "read"
)

// Code is the code of a plugin's error.
type Code string

// The four codes.
const (
	CodeUnsupported Code = "unsupported"
	CodeUnreadable  Code = "unreadable"
	CodeRefused     Code = "refused"
	CodeBug         Code = "bug"
)

// Codes are the codes in the contract's order.
func Codes() []Code { return []Code{CodeUnsupported, CodeUnreadable, CodeRefused, CodeBug} }

// Error is a plugin's error: a code, a message that is display only, and Path, a JSON pointer into the request
// body to the field at fault. It is an error value, so an operation returns it as it is.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

func (e *Error) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s (at %s)", e.Code, e.Message, e.Path)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Bytes is a byte string on the wire: "text" when the bytes are valid UTF-8, else "base64", so a case stays
// readable. An empty Bytes is the empty text. The engine sends either form; Bytes reads both.
type Bytes struct{ Data []byte }

// Text is b as a string, "" for nil.
func (b *Bytes) Text() string {
	if b == nil {
		return ""
	}
	return string(b.Data)
}

// Raw is b's bytes, nil for nil.
func (b *Bytes) Raw() []byte {
	if b == nil {
		return nil
	}
	return b.Data
}

// MarshalJSON writes b as {"text": ...} when it is valid UTF-8, else as {"base64": ...}.
func (b Bytes) MarshalJSON() ([]byte, error) {
	if utf8.Valid(b.Data) {
		return json.Marshal(struct {
			Text string `json:"text"`
		}{string(b.Data)})
	}
	return json.Marshal(struct {
		Base64 string `json:"base64"`
	}{base64.StdEncoding.EncodeToString(b.Data)})
}

// UnmarshalJSON reads {"text": ...} or {"base64": ...}, exactly one of the two.
func (b *Bytes) UnmarshalJSON(in []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(in, &m); err != nil {
		return err
	}
	text, hasText := m["text"]
	b64, hasB64 := m["base64"]
	switch {
	case hasText == hasB64:
		return errors.New("bytes holds exactly one of text and base64")
	case hasText:
		var s string
		if err := json.Unmarshal(text, &s); err != nil {
			return fmt.Errorf("bytes.text: %w", err)
		}
		b.Data = []byte(s)
	default:
		var s string
		if err := json.Unmarshal(b64, &s); err != nil {
			return fmt.Errorf("bytes.base64: %w", err)
		}
		d, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return fmt.Errorf("bytes.base64: %w", err)
		}
		b.Data = d
	}
	return nil
}

// File is a file placed in a run, read from the project or written by a preset's input: a relative path and its
// bytes.
type File struct {
	Path string `json:"path"`
	Data Bytes  `json:"data"`
}

// Config is a config file the engine found for the tool. Data is present when the file is 1 MiB or less; over that
// the plugin asks for a read.
type Config struct {
	Path string `json:"path"`
	Data *Bytes `json:"data,omitempty"`
}

// Run is a step that runs a granted program. Argv words are literal, except {scratch} and {null}.
type Run struct {
	Program string   `json:"program"`
	Argv    []string `json:"argv"`
	Stdin   *Bytes   `json:"stdin,omitempty"`
	Place   []File   `json:"place,omitempty"`
	Collect []string `json:"collect,omitempty"`
	Level   Level    `json:"level,omitempty"`
}

// Read is a step that reads the files a glob names, below the config set.
type Read struct {
	Glob string `json:"glob"`
}

// Step is one step of a plan: a run or a read, one of the two.
type Step struct {
	Run  *Run
	Read *Read
}

// MarshalJSON writes the step with its kind.
func (s Step) MarshalJSON() ([]byte, error) {
	switch {
	case s.Run != nil && s.Read == nil:
		return json.Marshal(struct {
			Kind Kind `json:"kind"`
			Run
		}{KindRun, *s.Run})
	case s.Read != nil && s.Run == nil:
		return json.Marshal(struct {
			Kind Kind `json:"kind"`
			Read
		}{KindRead, *s.Read})
	}
	return nil, errors.New("a step is a run or a read, one of the two")
}

// UnmarshalJSON reads a step by its kind.
func (s *Step) UnmarshalJSON(in []byte) error {
	var k struct {
		Kind Kind `json:"kind"`
	}
	if err := json.Unmarshal(in, &k); err != nil {
		return err
	}
	*s = Step{}
	switch k.Kind {
	case KindRun:
		s.Run = new(Run)
		return json.Unmarshal(in, s.Run)
	case KindRead:
		s.Read = new(Read)
		return json.Unmarshal(in, s.Read)
	}
	return fmt.Errorf("a step has the kind %q, want run or read", k.Kind)
}

// Result is what a step gave, as the engine hands it back in Have. Exit, Stdout, Stderr and Cut are those of a
// run; Files are those a run collected or a read found.
type Result struct {
	Round  int    `json:"round"`
	Step   int    `json:"step"`
	Kind   Kind   `json:"kind"`
	Exit   *int   `json:"exit,omitempty"`
	Stdout *Bytes `json:"stdout,omitempty"`
	Stderr *Bytes `json:"stderr,omitempty"`
	Cut    *bool  `json:"cut,omitempty"`
	Files  []File `json:"files"`
}

// Gathered is what the engine has gathered when it calls plan, entries or findings: the tool's version, the
// scratch folder as the tool saw it (so the plugin can cut it from printed paths), the config files, and the
// results of every step so far, by round and then step.
type Gathered struct {
	ToolVersion string   `json:"tool_version"`
	Scratch     string   `json:"scratch,omitempty"`
	Config      []Config `json:"config"`
	Have        []Result `json:"have"`
}

// PlanRequest asks what to run or read next.
type PlanRequest struct {
	Gathered
	For      For      `json:"for"`
	Target   Target   `json:"target"`
	Round    int      `json:"round"`
	Fixtures []string `json:"fixtures,omitempty"`
}

// PlanResponse is the steps to take, and whether they are the last round.
type PlanResponse struct {
	Steps []Step `json:"steps"`
	Final bool   `json:"final"`
}

// EntriesRequest asks for the entries the tool's output and config hold.
type EntriesRequest struct {
	Gathered
	Target Target `json:"target"`
}

// Entry is one rule of the tool as the project has it. Off is On false.
type Entry struct {
	Key      string   `json:"key"`
	On       bool     `json:"on"`
	Severity Severity `json:"severity,omitempty"`
	Value    Value    `json:"value,omitempty"`
	Options  Value    `json:"options,omitempty"`
}

// EntriesResponse is the entries, sorted by key, each key once.
type EntriesResponse struct {
	Entries []Entry `json:"entries"`
}

// FindingsRequest asks for the findings of a run over fixtures.
type FindingsRequest struct {
	Gathered
	Fixtures []string `json:"fixtures"`
}

// Finding is one finding of a run. Line and Column count from 1; zero is absent.
type Finding struct {
	Key      string   `json:"key"`
	Message  string   `json:"message"`
	Path     string   `json:"path,omitempty"`
	Line     int      `json:"line,omitempty"`
	Column   int      `json:"column,omitempty"`
	Severity Severity `json:"severity,omitempty"`
	URL      string   `json:"url,omitempty"`
}

// FindingsResponse is the findings, sorted by path, line, column, key, message. Ran is false when the tool failed
// and Why says so.
type FindingsResponse struct {
	Ran      bool      `json:"ran"`
	Why      string    `json:"why,omitempty"`
	Findings []Finding `json:"findings"`
}

// Pair is one value of the preset and the value found, to be ranked.
type Pair struct {
	Key    string `json:"key"`
	Aspect Aspect `json:"aspect"`
	Preset Value  `json:"preset"`
	Found  Value  `json:"found"`
}

// RankRequest asks which of each pair is tighter.
type RankRequest struct {
	ToolVersion string `json:"tool_version"`
	Pairs       []Pair `json:"pairs"`
}

// RankResponse answers the pairs by position, equal in number.
type RankResponse struct {
	Words []Word `json:"words"`
}

// PresetEntry is one entry of the bindings a preset is written from.
type PresetEntry struct {
	Key      string   `json:"key"`
	Target   Target   `json:"target"`
	Severity Severity `json:"severity,omitempty"`
	Value    Value    `json:"value,omitempty"`
	Options  Value    `json:"options,omitempty"`
	Trial    bool     `json:"trial"`
	Rules    []string `json:"rules"`
}

// PresetRequest asks for the tool's file, written from entries. Mark, when the manifest says mark: true, must
// stand in the first line of every file.
type PresetRequest struct {
	ToolVersion string        `json:"tool_version"`
	Mark        string        `json:"mark,omitempty"`
	Inputs      []File        `json:"inputs"`
	Entries     []PresetEntry `json:"entries"`
}

// PresetFile is a file of the preset: a path and its text, written byte for byte.
type PresetFile struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

// PresetResponse is the files, sorted by path, each once, and notes that are display only.
type PresetResponse struct {
	Files []PresetFile `json:"files"`
	Notes []string     `json:"notes,omitempty"`
}

// Request is the envelope of a call: the contract number, the features the engine has, the operation and its
// body. The body is read by the operation.
type Request struct {
	Contract int             `json:"contract"`
	Features []string        `json:"features"`
	Op       string          `json:"op"`
	Body     json.RawMessage `json:"body"`
}

// Response is the envelope of an answer: ok or error, one of the two.
type Response struct {
	OK    json.RawMessage `json:"ok,omitempty"`
	Error *Error          `json:"error,omitempty"`
}
