// Package model holds gragas's shared types: the closing words and the
// result a command ends with. It is data. The only functions here are the
// constructors that make a refusal or a failure impossible to build without
// what it must carry (cli.errors.2), and they have no effect.
package model

import (
	"errors"
	"fmt"
	"slices"
)

// A Word is how a run ends, one of eight and nothing else (cli.look.5).
// A person reads the word and a script reads the code internal/exit gives it.
type Word string

// The eight closing words. Seven are outcomes of the work; Failed is the
// engine itself failing, and carries the run id.
const (
	Done     Word = "done"
	Failed   Word = "failed"
	Findings Word = "findings"
	Usage    Word = "usage"
	Busy     Word = "busy"
	NotReady Word = "not ready"
	Refused  Word = "refused"
	Declined Word = "declined"
)

// Words returns the closed set, in the order the spec lists the codes. It is
// a fresh slice each time, so no caller can change the set.
func Words() []Word {
	return []Word{Done, Failed, Findings, Usage, Busy, NotReady, Refused, Declined}
}

// Known says whether w is one of the eight.
func Known(w Word) bool {
	return slices.Contains(Words(), w)
}

// refusing are the words that mean nothing was done and the person must be
// told what to type next: a refusal carries a hint (cli.errors.2).
func refusing(w Word) bool {
	return w == Usage || w == Busy || w == NotReady || w == Refused || w == Declined
}

// Errors the constructors return, for a caller or a test to match.
var (
	// ErrNotRefusing is a refusal built with a word that is not one of the
	// five that refuse.
	ErrNotRefusing = errors.New("model: the word does not refuse")
	// ErrNoHint is a refusal built without the line to type.
	ErrNoHint = errors.New("model: a refusal needs a hint")
	// ErrNoKind is a failure built without its kind.
	ErrNoKind = errors.New("model: a failure needs a kind")
	// ErrNoRun is a failure built without the run id.
	ErrNoRun = errors.New("model: a failure needs a run id")
)

// A Refusal is a run that did nothing and says why: the word, what was
// observed, the valid choices when the set is closed, and the line to type
// (cli.errors.6). The fields are private so the constructor is the only way
// to build one with a word and a hint.
type Refusal struct {
	word     Word
	observed string
	valid    []string
	hint     string
}

// NewRefusal builds a refusal. The word must be usage, busy, not ready,
// refused or declined, and the hint, the line to type, must not be empty.
// valid is the closed set of choices, nil when there is none.
func NewRefusal(w Word, observed string, valid []string, hint string) (Refusal, error) {
	if !refusing(w) {
		return Refusal{}, fmt.Errorf("%w: %q", ErrNotRefusing, string(w))
	}
	if hint == "" {
		return Refusal{}, ErrNoHint
	}
	return Refusal{word: w, observed: observed, valid: append([]string(nil), valid...), hint: hint}, nil
}

// Word is the closing word the refusal ends on.
func (r Refusal) Word() Word { return r.word }

// Observed is what was seen that made the run stop.
func (r Refusal) Observed() string { return r.observed }

// Valid is the closed set of choices, a copy; empty when there is none.
func (r Refusal) Valid() []string { return append([]string(nil), r.valid...) }

// Hint is the line to type.
func (r Refusal) Hint() string { return r.hint }

// A Kind names how the engine failed, the "kind" of the JSON document, so a
// script tells a failed run from a run with findings though both exit 1.
type Kind string

// A Failure is the engine itself failing: an unreadable settings file, a
// child that dies, a torn journal. It names its kind and the run id, so
// the person has an id to quote.
type Failure struct {
	kind Kind
	run  string
}

// NewFailure builds a failure. Both the kind and the run id are required.
func NewFailure(kind Kind, run string) (Failure, error) {
	if kind == "" {
		return Failure{}, ErrNoKind
	}
	if run == "" {
		return Failure{}, ErrNoRun
	}
	return Failure{kind: kind, run: run}, nil
}

// Word is Failed.
func (Failure) Word() Word { return Failed }

// Kind is how it failed.
func (f Failure) Kind() Kind { return f.kind }

// Run is the run id.
func (f Failure) Run() string { return f.run }
