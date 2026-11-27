package model

// What a project's effective config says about each entry of the resolved
// preset (docs/spec.md 17.4): the six words, the preset entries the
// caller builds from the chain, and what the link context answers. These are
// types only. A word is a value the program established; where it could
// not establish one it says why and gives none (cli.output.9).

// ObservationWord is how a project's effective config stands against one
// entry of the preset.
type ObservationWord string

// The six words.
const (
	// ObservedAsSet is an entry the project runs as the preset ships it.
	ObservedAsSet ObservationWord = "as-set"
	// ObservedTighter is an entry the project set stricter: a stricter
	// severity, or options that are a subset of the preset's.
	ObservedTighter ObservationWord = "tighter"
	// ObservedWeaker is an entry the project set looser, or turned off.
	ObservedWeaker ObservationWord = "weaker"
	// ObservedAbsent is a preset entry that is not in the effective config.
	ObservedAbsent ObservationWord = "absent"
	// ObservedUnmapped is an entry in the effective config that no rule
	// explains.
	ObservedUnmapped ObservationWord = "unmapped"
	// ObservedExcluded is a preset entry whose rule an overlay dropped with a
	// decision.
	ObservedExcluded ObservationWord = "excluded"
)

// ObservationWords is the closed set, in the order the spec gives them.
func ObservationWords() []ObservationWord {
	return []ObservationWord{ObservedAsSet, ObservedTighter, ObservedWeaker, ObservedAbsent, ObservedUnmapped, ObservedExcluded}
}

// ExcludedBy names the overlay that dropped a rule and the decision that says
// why.
type ExcludedBy struct {
	// Overlay is the layer that dropped the rule. Display only.
	Overlay string `json:"overlay"`
	// Decision is the slug of the decision that settles the drop.
	// Display only.
	Decision string `json:"decision"`
}

// PresetEntry is one entry of the resolved preset, built by the caller from
// the chain: a binding with where it came from. The caller resolves the chain
// (the link context reads no file and names no rulebook).
type PresetEntry struct {
	// Binding is the entry as the preset ships it: tool, target, key, value,
	// options and severity. Decides: which entry it is and what it is set to.
	Binding Binding `json:"binding"`
	// Layer is the layer of the chain that set the entry. Display only.
	Layer string `json:"layer,omitempty"`
	// Decision is the slug of the decision behind the entry, "" for none.
	// Display only.
	Decision string `json:"decision,omitempty"`
	// Excluded is set where the rule that holds the entry was dropped by an
	// overlay with a decision. Decides: whether the entry is excluded, or, if
	// the config still holds it, unmapped.
	Excluded *ExcludedBy `json:"excluded,omitempty"`
}

// Provenance is where an observed entry comes from: the layer and decision of
// the preset entry, and the overlay and decision that dropped it.
type Provenance struct {
	// Layer is the layer that set the preset entry, "" for an entry the
	// preset does not hold. Display only.
	Layer string `json:"layer,omitempty"`
	// Decision is the decision behind the preset entry, "" for none.
	// Display only.
	Decision string `json:"decision,omitempty"`
	// ExcludedBy is who dropped the rule, nil where none was. Display only.
	ExcludedBy *ExcludedBy `json:"excluded_by,omitempty"`
}

// Observation is one entry of the effective config or of the preset, and how
// the first stands against the second.
type Observation struct {
	// Key is the tool's entry. Display only.
	Key string `json:"key"`
	// Word is the observation, "" where the program could not establish one;
	// Reason then says why. Decides: what a finding is made from.
	Word ObservationWord `json:"word,omitempty"`
	// Reason is why Word is empty, "" where a word was established.
	// Display only.
	Reason string `json:"reason,omitempty"`
	// Provenance is where the entry comes from. Display only.
	Provenance Provenance `json:"provenance"`
}

// Observed is what observing an effective config against a preset answers.
type Observed struct {
	// Entries are the observations: each preset entry in the preset's order,
	// then each entry of the config that no preset entry names, in the
	// config's order. Empty where Unknown is set. Decides: which observations
	// a finding is made from.
	Entries []Observation `json:"entries,omitempty"`
	// Unknown is why the config could not be read at all, "" where it was.
	// Display only.
	Unknown string `json:"unknown,omitempty"`
}
