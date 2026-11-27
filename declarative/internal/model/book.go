package model

// The rulebook's types: what a rulebook's files describe once they are read
// (format v3, docs/architecture.md section 7.1). They are types only
// (arch.layers.11): no method reads a disk, and loading files into them is
// the format context's work.
//
// Every field's JSON name is its name in the schema under
// internal/host/assets/format/schema/v3/, and a repository test holds the two
// to each other in both directions. A field that carries text a person wrote
// says "display only", or "decides" when the engine acts on it (arch.ports.19):
// nothing a file says decides anything unless its comment says so.

// FormatVersion is the format version these types and schemas describe. A
// file carries its version inside it; a reader that finds an older one
// migrates it forward and one that finds a newer one stops and names both
// versions (store.schemas.2).
const FormatVersion = 3

// ContractVersion is the version of the contract, the documents gragas emits
// under --json: the one place it is written. Every document names it in one
// top-level field (cli.output.1), and the schemas under
// internal/host/assets/contract/schema/ are those of this version.
const ContractVersion = "1.3"

// RuleID is a rule's id, part.concern.number, frozen at birth and never
// reused. Decides: it is the key every reference to a rule uses.
type RuleID string

// Book is one rulebook as the format context loads it: the identity and the
// structure, and every entity of the other files. It is assembled, not read
// from one file, so no schema describes it as a whole. Standing is not here:
// it is derived from the status, the bindings and their proof.
type Book struct {
	// Identity is rulebook.yaml.
	Identity RulebookIdentity
	// Structure is structure.yaml.
	Structure Structure
	// Rules are the rules of every YAML file and every Markdown block, in the
	// order of the structure.
	Rules []Rule
	// Clashed are the rules read whose id another rule of the book held
	// already, in the order read. They are no part of Rules and no check reads
	// them: only a renumber does, to give one of them an id of its own.
	Clashed []Rule `json:"-"`
	// Anchors is the anchor of the Markdown section each rule stands in, by
	// rule id. A rule written in YAML has none.
	Anchors map[RuleID]string
	// Terms are the entries of terms.yaml.
	Terms []Term
	// Sources are the entries of sources.yaml.
	Sources []Source
	// Decisions are the entries of decisions.yaml.
	Decisions []Decision
	// Exclusions are the entries of exclusions.yaml.
	Exclusions []Exclusion
	// Proposals are the files of proposals/.
	Proposals []Proposal
	// Translations are the entries of every file under translations/.
	Translations []Translation
	// BindingRecords are the files of bindings/, kept opaque, in path order.
	BindingRecords []Opaque
	// History is history.jsonl, in the order of its lines.
	History []HistoryEntry
}

// Tightening is one bindings/ record as link reads it: a binding a rulebook
// holds a rule it borrowed to, a parent's rule in an overlay or an imported
// one. Its subject is the borrowed rule's id; the rulebook's own
// prefix is the folder it is written in, never the id.
type Tightening struct {
	// Path is the record's file, relative to the book's folder.
	Path string
	// Subject is the borrowed rule's id.
	Subject RuleID
	// Binding is the binding, in the keys of a rule's binding.
	Binding Binding
	// Text is the record as the file holds it, for a change to name what it
	// replaces.
	Text string
}

// Opaque is a file kept opaque: its path below the book's folder, the digest
// of its bytes ("sha256:" and the sum in lower-case hex) and the bytes as they
// are. Nothing in it is read or checked by the format context; the context
// that owns its meaning reads it. The bindings/ records of an overlay are
// these (link owns what they mean).
type Opaque struct {
	// Path is relative to the book's folder, slash-separated.
	Path string
	// Digest is the digest of Data, as the file stands.
	Digest Digest
	// Data is the file's whole content.
	Data []byte
}
