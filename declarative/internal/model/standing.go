package model

import (
	"fmt"
	"slices"
	"strings"
)

// Truth is what the proof says of one binding: the result of its pair, as
// the proof grid holds it (spec 17.5). Decides: a binding at rung 0 to 5 is
// effective only on TruthPass.
type Truth string

// The four cells of the proof grid. A binding the grid does not name is
// TruthNotRun.
const (
	TruthPass     Truth = "pass"
	TruthFail     Truth = "fail"
	TruthNotRun   Truth = "not run"
	TruthReviewed Truth = "reviewed"
)

// Truths is the closed set.
func Truths() []Truth { return []Truth{TruthPass, TruthFail, TruthNotRun, TruthReviewed} }

// Standing is what a rule is held as, derived from its status, its effective
// bindings and their proof, and never typed (spec 17.2). It has six
// values.
type Standing string

// The six standings.
const (
	StandingProposed Standing = "proposed"
	StandingEnforced Standing = "enforced"
	StandingReviewed Standing = "reviewed"
	StandingPlanned  Standing = "planned"
	StandingAdvisory Standing = "advisory"
	StandingRetired  Standing = "retired"
)

// Standings is the closed set, in the order spec 17.2 lists them.
func Standings() []Standing {
	return []Standing{StandingProposed, StandingEnforced, StandingReviewed, StandingPlanned, StandingAdvisory, StandingRetired}
}

// Mark is shown beside a standing and never replaces it (spec 17.3). It has
// three values.
type Mark string

// The three marks.
const (
	MarkTrial    Mark = "trial"
	MarkBlocked  Mark = "blocked"
	MarkUnproven Mark = "unproven"
)

// Marks is the closed set, in the order spec 17.3 lists them.
func Marks() []Mark { return []Mark{MarkTrial, MarkBlocked, MarkUnproven} }

// BindingRef names one binding of a rule by what decides it: the rule and
// the binding's deciding fields (every field but the remedy, which is display
// only), never its place in the rule's Bindings. A cell recorded for a binding
// that has since been unbound, reordered or changed matches no binding, so
// the binding that took its place reads as not run.
type BindingRef struct {
	Rule    RuleID
	Binding string
}

// Ref is the reference to the binding b of the rule.
func (b Binding) Ref(rule RuleID) BindingRef {
	return BindingRef{Rule: rule, Binding: fmt.Sprintf("%q %q %q %#v %#v %q %d %q %d %q %q %q",
		b.Tool, b.Target, b.Key, b.Value, b.Options, b.Severity, b.Rung, b.RunPoint, b.Pair, b.PairPath, b.Tightens, b.Checker)}
}

// Grid is the proof grid as a value: the truth of each binding it names. A
// binding it does not name is not run. Reading a grid from disk is the
// proof's, not this package's.
type Grid map[BindingRef]Truth

// Held is a rule's derived standing and its marks, in the order of Marks.
type Held struct {
	Rule     RuleID
	Standing Standing
	Marks    []Mark
}

// Has says whether the mark is shown beside the standing.
func (h Held) Has(m Mark) bool { return slices.Contains(h.Marks, m) }

// String is the words as a person reads them: "enforced, trial".
func (h Held) String() string {
	var s strings.Builder
	s.WriteString(string(h.Standing))
	for _, m := range h.Marks {
		s.WriteString(", ")
		s.WriteString(string(m))
	}
	return s.String()
}
