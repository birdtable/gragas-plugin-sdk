package model

import "slices"

// Term is a word a rulebook defines once.
type Term struct {
	// Slug is the term's key. Decides.
	Slug string `json:"slug"`
	// Definition says what the word means. Display only.
	Definition string `json:"definition"`
	// Literal is the header name, package or other literal the term stands
	// for, so statements stay neutral. Decides: the prose lint matches it.
	Literal string `json:"literal,omitempty"`
	// Status is active or retired; empty is active. A retired term stays in
	// the file and no statement may use it. Decides: history guards it.
	Status Status `json:"status,omitempty"`
}

// SourceKind is what a source is.
type SourceKind string

// The seven kinds of source.
const (
	SourceStandard       SourceKind = "standard"
	SourceGuide          SourceKind = "guide"
	SourceStudy          SourceKind = "study"
	SourceIncident       SourceKind = "incident"
	SourceDecisionRecord SourceKind = "decision-record"
	SourceVendorDocument SourceKind = "vendor-document"
	SourceOwnDecision    SourceKind = "own-decision"
)

// SourceKinds is the closed set.
func SourceKinds() []SourceKind {
	return []SourceKind{SourceStandard, SourceGuide, SourceStudy, SourceIncident, SourceDecisionRecord,
		SourceVendorDocument, SourceOwnDecision}
}

// Source is a thing the rules cite.
type Source struct {
	// Slug is the source's key. Decides: a citation names it.
	Slug string `json:"slug"`
	// Kind is what the source is. Decides: an own decision counts as a
	// source.
	Kind SourceKind `json:"kind"`
	// Title names the source. Display only.
	Title string `json:"title"`
	// Locator says where the source is: a URL, an ISBN, a DOI, an RFC or a
	// decision slug. Display only.
	Locator string `json:"locator"`
	// Attribution credits the source's author. Display only.
	Attribution string `json:"attribution,omitempty"`
	// Verified is the day someone opened the source, as YYYY-MM-DD. Kept so a
	// reviewer sees how long ago a source was last read; nothing computes from
	// it. Display only.
	Verified string `json:"verified,omitempty"`
}

// Decision is something decided, with its reasons.
type Decision struct {
	// Slug is the decision's key. Decides: history, exclusions and rules name
	// it.
	Slug string `json:"slug"`
	// Title names the decision. Display only.
	Title string `json:"title"`
	// Text is what was decided and why. Display only.
	Text string `json:"text"`
	// Rejected are the options that were not taken. Display only.
	Rejected []string `json:"rejected,omitempty"`
	// Cites are the sources, rules and bindings the decision rests on.
	Cites *Cites `json:"cites,omitempty"`
	// Who made the decision. Display only.
	Who string `json:"who"`
	// Date is the day it was made, as YYYY-MM-DD. Display only.
	Date string `json:"date"`
	// Supersedes is the decision this one replaces. Decides.
	Supersedes string `json:"supersedes,omitempty"`
	// SupersededBy is the decision that replaced this one. Decides.
	SupersededBy string `json:"superseded_by,omitempty"`
}

// Cites are what a decision rests on.
type Cites struct {
	// Sources are source slugs.
	Sources []string `json:"sources,omitempty"`
	// Rules are rule ids.
	Rules []RuleID `json:"rules,omitempty"`
	// Bindings are bindings, each written <rule id> <tool> <key>.
	Bindings []string `json:"bindings,omitempty"`
}

// Exclusion is an overlay dropping a parent's rule, with a decision.
type Exclusion struct {
	// Overlay is the overlay's name. Decides: part of the exclusion's key.
	Overlay string `json:"overlay"`
	// Rule is the parent's rule id. Decides: part of the exclusion's key.
	Rule RuleID `json:"rule"`
	// Number is the sequence number, the last part of the key. Decides.
	Number int `json:"number"`
	// Decision is the slug of the decision that excluded the rule. Decides.
	Decision string `json:"decision"`
	// Who made the exclusion. Display only.
	Who string `json:"who"`
	// When it was made, as UTC text to the second. Display only.
	When string `json:"when"`
	// Lifted is set when the exclusion was lifted. A lifted record stays.
	Lifted *Lifted `json:"lifted,omitempty"`
}

// Lifted is how an exclusion was lifted.
type Lifted struct {
	// Decision is the slug of the decision that lifted it. Decides.
	Decision string `json:"decision"`
	// Who lifted it. Display only.
	Who string `json:"who"`
	// When it was lifted, as UTC text to the second. Display only.
	When string `json:"when"`
}

// Translation is one field of one entity in another language.
type Translation struct {
	// Entity is the entity kind and its key, such as rule:store.schemas.2.
	// Decides.
	Entity string `json:"entity"`
	// Field is the entity's field that was translated. Decides.
	Field string `json:"field"`
	// Language is the language tag of the translation. Decides.
	Language string `json:"language"`
	// Text is the translated text. Display only.
	Text string `json:"text"`
	// Hash is the digest of the authoritative text this translated. Decides:
	// a translation is stale when it differs from the digest of the text now.
	// Staleness is derived and is not a field.
	Hash Digest `json:"hash"`
}

// ProposalState is where a proposal stands.
type ProposalState string

// The four states of a proposal.
const (
	ProposalOpen      ProposalState = "open"
	ProposalAccepted  ProposalState = "accepted"
	ProposalDeclined  ProposalState = "declined"
	ProposalWithdrawn ProposalState = "withdrawn"
)

// ProposalStates is the closed set.
func ProposalStates() []ProposalState {
	return []ProposalState{ProposalOpen, ProposalAccepted, ProposalDeclined, ProposalWithdrawn}
}

// CanBecome is whether a proposal in state s may move to state to (spec
// 16.1): an open proposal is accepted, declined or withdrawn, and the other
// three are final. Staying in a state is no move, so it is not one either.
func (s ProposalState) CanBecome(to ProposalState) bool {
	return s == ProposalOpen && to != ProposalOpen && slices.Contains(ProposalStates(), to)
}

// Proposal is a change an overlay carries to a parent, as the overlay's file
// records it. The proposal itself is a contract document.
type Proposal struct {
	// Format is the format version the file is written in.
	Format int `json:"format"`
	// Slug is the proposal's key. Decides.
	Slug string `json:"slug"`
	// Target is the rulebook the proposal is for. Decides.
	Target string `json:"target"`
	// State is where the proposal stands. Decides.
	State ProposalState `json:"state"`
	// Link is the pull request or issue that carries it. Display only.
	Link string `json:"link,omitempty"`
	// Opened is the day it was opened, as YYYY-MM-DD. Kept so a reviewer sees
	// how long a proposal has waited; nothing computes from it. Display only.
	Opened string `json:"opened"`
	// Document is the proposal as a contract document, whose schema is the
	// contract's. Display only here.
	Document map[string]any `json:"document"`
}
