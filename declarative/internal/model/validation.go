package model

// EnginePart is the first word of every id the engine's own checks carry,
// and a word the format reserves: both loaders refuse a part, or a part's
// alias, spelled so, and a rule's id or alias whose first word it is, for
// every status, each as a finding at its line. The id is refused on its own
// because format 3 does not hold an id to its part and the legacy loader does
// not hold a retired one. So no rule's id can be an engine check's, and a
// SARIF reader keying on ruleId never merges the two. A rulebook's name is not reserved:
// an imported id is namespaced as name.part.concern.n, four words, and an
// engine check's id is three. An engine check added later takes this word
// first, so the one reserved word covers them all.
const EnginePart = "gragas"

// ValidateCheck is the id every result of validate carries in SARIF, listed
// once in the tool's rules. It is the validate check's own id, not a rulebook
// rule. It is here, beside the Validation it names, because the core and the
// commands hold no rule id in a string (arch.ports.2).
const ValidateCheck = EnginePart + ".validate.1"

// A Validation is what validate found in a rulebook: where its files are,
// how many rules it holds, what is wrong with it and what is only worth
// saying. The closing word is derived, never stored: any finding ends
// findings, and a notice changes nothing.
type Validation struct {
	// Dir is the folder of rulebook.yaml below the root, "" for the root.
	Dir string
	// Rules is how many rules the rulebook holds.
	Rules int
	// Findings are the problems, in the order of the files read.
	Findings []Finding
	// Notices are what the load says that is not a problem.
	Notices []Notice
	// Held is how many migrated rules the prose lint holds, each statement
	// still with the digest its conversion recorded. It only
	// falls: a reworded statement is linted from then on.
	Held int
}

// Word is how validate ends on v: findings when there is one, otherwise done.
func (v Validation) Word() Word {
	if len(v.Findings) > 0 {
		return Findings
	}
	return Done
}
