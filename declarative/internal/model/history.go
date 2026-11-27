package model

// Lifecycle is the fields of an entity that a history entry guards: a change
// to one of them is made only with its entry. Each decides.
type Lifecycle struct {
	// Status is the rule's status.
	Status Status `json:"status,omitempty"`
	// Owner is who answers for the rule.
	Owner string `json:"owner,omitempty"`
	// ReviewBy is the review-by date, as YYYY-MM-DD.
	ReviewBy string `json:"review_by,omitempty"`
	// ZeroBy is the zero-by date, as YYYY-MM-DD.
	ZeroBy string `json:"zero_by,omitempty"`
	// ReplacedBy is the rule that took the rule's place.
	ReplacedBy RuleID `json:"replaced_by,omitempty"`
	// Literal is the header name, package or other literal a term stands for.
	Literal string `json:"literal,omitempty"`
}

// HistoryEntry is one line of history.jsonl. The file is append-only, and
// each entry names the one before it on its branch.
type HistoryEntry struct {
	// Format is the format version the line is written in.
	Format int `json:"format"`
	// Number is the sequence number. Decides: the reader orders entries by it.
	Number int `json:"number"`
	// Previous is the digest of the entry before it on its branch. The first
	// entry has none. Decides: the chain is checked by it.
	Previous Digest `json:"previous,omitempty"`
	// Kind is what the command did, as a slug such as promote. Display only.
	Kind string `json:"kind"`
	// Entity is the entity kind and its key, such as rule:store.schemas.2.
	// Decides.
	Entity string `json:"entity"`
	// Before is the entity's lifecycle fields before the change; nil when the
	// entity is new. Decides.
	Before *Lifecycle `json:"before,omitempty"`
	// After is the entity's lifecycle fields after the change. Decides.
	After Lifecycle `json:"after"`
	// Who made the change. Display only.
	Who string `json:"who"`
	// When it was made, as UTC text to the second. Display only.
	When string `json:"when"`
	// Plan is the digest of the plan the yes accepted. Decides: the guard
	// recomputes it.
	Plan Digest `json:"plan"`
	// Command is the command the yes agreed to, as a person types it and
	// without --apply; "" on a line written before it was recorded. Decides:
	// guard staged replays it to recompute Plan (arch.ports.19).
	Command string `json:"command,omitempty"`
	// Decision is the slug of the decision the entry records, when the
	// command takes one. Decides.
	Decision string `json:"decision,omitempty"`
	// Reverts is the digest of the entry an undo takes back, as Previous names
	// one; empty for any other entry. Decides: the next undo skips an entry that
	// an undo names and agrees with.
	Reverts Digest `json:"reverts,omitempty"`
	// Statements is the digest of each rule's statement, by rule id, as the
	// conversion wrote it: only a migrate entry holds it, and an empty map is
	// a migrate of a rulebook with no rule. Decides: the prose
	// lint holds a migrated rule whose statement still has its digest.
	Statements map[RuleID]Digest `json:"statements,omitempty"`
	// Digest is the canonical digest of the line as it was read, the one the
	// next line and an undo name it by. The reader sets it; it is no part of the
	// line, so it is never written.
	Digest Digest `json:"-"`
}
