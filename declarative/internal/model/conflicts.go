package model

// Entry conflicts: two bindings of one resolved rulebook that name the same
// tool entry with different values, options or severity (docs/spec.md).
// Types only; link.Conflicts builds them and the listing draws them.
// Every field says what it decides or that it is display only (arch.ports.19).

// EntryRef is a tool entry: the tool, the target type and the key.
type EntryRef struct {
	// Tool is the tool's slug. Decides: which adapter compares the values.
	Tool string `json:"tool"`
	// Target is what the entry is about. Decides.
	Target TargetType `json:"target"`
	// Key is the tool's entry. Decides.
	Key string `json:"key"`
}

// What two bindings of an entry can differ in.
const (
	DiffValue    = "value"
	DiffOptions  = "options"
	DiffSeverity = "severity"
)

// ConflictSide is one binding of a conflicting entry, with the rule that holds
// it.
type ConflictSide struct {
	// Ref names the binding by what decides it. Decides: the binding that is
	// blocked.
	Ref BindingRef `json:"-"`
	// Rule is the rule that holds the binding. Display only.
	Rule RuleID `json:"rule"`
	// Value is the entry's value in the binding. Display only.
	Value any `json:"value,omitempty"`
	// Options are the entry's options in the binding. Display only.
	Options map[string]any `json:"options,omitempty"`
	// Severity is the binding's severity. Display only.
	Severity Severity `json:"severity"`
}

// Conflict is one entry two or more bindings of the resolved rulebook set
// differently.
type Conflict struct {
	// Entry is the tool entry the bindings share. Decides.
	Entry EntryRef `json:"entry"`
	// Differs says what the bindings differ in, of value, options and
	// severity, in that order. Decides: nothing past the listing.
	Differs []string `json:"differs"`
	// Sides are the conflicting bindings, in the order of the rules. Decides:
	// which bindings the conflict blocks.
	Sides []ConflictSide `json:"sides"`
	// Winner is the rule a decision names as the winner of the conflict, ""
	// while the conflict is open. Decides: a conflict with a winner is
	// resolved and blocks nothing.
	Winner RuleID `json:"winner,omitempty"`
}

// Open says whether the conflict blocks its bindings.
func (c Conflict) Open() bool { return c.Winner == "" }

// Chain is the resolved chain as the standing of a rule reads it (S32's third
// input): the conflicts of the resolved rulebook, of which the open ones block
// the bindings they name.
type Chain struct {
	// Conflicts are the entry conflicts, by entry. Decides: the blocked
	// bindings.
	Conflicts []Conflict `json:"conflicts"`
}

// Blocks says whether a binding is in an open conflict.
func (c Chain) Blocks(ref BindingRef) bool {
	for _, k := range c.Conflicts {
		if !k.Open() {
			continue
		}
		for _, s := range k.Sides {
			if s.Ref == ref {
				return true
			}
		}
	}
	return false
}

// ConflictsCheck is the id every result of conflicts carries in SARIF, listed
// in the tool's checks.
const ConflictsCheck = EnginePart + ".conflicts.1"

// ConflictsListing is what the conflicts command lists: the entry conflicts of
// the rulebook, open and resolved.
type ConflictsListing struct {
	// Dir is the folder of the rulebook below the root, "" for the root.
	// Display only.
	Dir string `json:"dir"`
	// Conflicts are the conflicts, by entry in the order the rules hold them.
	// Decides: the closing word.
	Conflicts []Conflict `json:"conflicts"`
}

// Open is how many conflicts block their bindings.
func (l ConflictsListing) Open() int {
	n := 0
	for _, c := range l.Conflicts {
		if c.Open() {
			n++
		}
	}
	return n
}

// Word is how conflicts ends: findings when any is open, otherwise done.
func (l ConflictsListing) Word() Word {
	if l.Open() > 0 {
		return Findings
	}
	return Done
}
