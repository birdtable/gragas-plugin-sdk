package model

// IdentityKind is what a rulebook is to other rulebooks.
type IdentityKind string

// The five kinds of rulebook.
const (
	KindBase      IdentityKind = "base"
	KindOverlay   IdentityKind = "overlay"
	KindCompanion IdentityKind = "companion"
	KindFork      IdentityKind = "fork"
	KindArchived  IdentityKind = "archived"
)

// IdentityKinds is the closed set, in the order of the spec.
func IdentityKinds() []IdentityKind {
	return []IdentityKind{KindBase, KindOverlay, KindCompanion, KindFork, KindArchived}
}

// Contribution is how a rulebook takes changes from others.
type Contribution string

// The three contribution modes of a governance.
const (
	ContributionOpen      Contribution = "open"
	ContributionProposals Contribution = "proposals"
	ContributionClosed    Contribution = "closed"
)

// Contributions is the closed set.
func Contributions() []Contribution {
	return []Contribution{ContributionOpen, ContributionProposals, ContributionClosed}
}

// ProofLevel is how isolated a proof run is.
type ProofLevel string

// The two levels third-party checker code can be asked to reach.
const (
	ProofIsolated  ProofLevel = "isolated"
	ProofSandboxed ProofLevel = "sandboxed"
)

// ProofLevels is the closed set.
func ProofLevels() []ProofLevel { return []ProofLevel{ProofIsolated, ProofSandboxed} }

// RulebookIdentity is rulebook.yaml: who the rulebook is, what it extends and
// imports, who may change it and what it was tested on.
type RulebookIdentity struct {
	// Format is the format version the file is written in.
	Format int `json:"format"`
	// Name is the rulebook's short name. Decides: the index and a project's
	// pin resolve it.
	Name string `json:"name"`
	// Version is the release the file declares, as semantic-version text.
	// Decides: it is the version a release publishes unchanged for the first
	// release.
	Version string `json:"version"`
	// Kind is what the rulebook is. Decides: it picks the overlay contract.
	Kind IdentityKind `json:"kind"`
	// Licences are the licences of the rulebook's kinds of content.
	Licences Licences `json:"licences"`
	// Language is the authoritative language, a language tag. Decides: it is
	// the one language whose text is the law and whose hash a translation
	// carries.
	Language string `json:"language"`
	// DocsURL is the base of the published pages. Display only: it makes
	// links and promises nothing about serving.
	DocsURL string `json:"docs_url,omitempty"`
	// ExplorerURL is the base of an explorer the owner hosts. Display only.
	ExplorerURL string `json:"explorer_url,omitempty"`
	// Extends is the parent at a pin, for an overlay.
	Extends *Extends `json:"extends,omitempty"`
	// Imports are the parts taken by name from other rulebooks.
	Imports []Import `json:"imports,omitempty"`
	// Owners are the people who own the rulebook, shown in its views; they
	// grant nothing. Display only.
	Owners []string `json:"owners"`
	// Governance is the rulebook's published rules for changing it.
	Governance Governance `json:"governance"`
	// Tested is the set of versions the last release was proven on.
	Tested *Tested `json:"tested,omitempty"`
	// Budgets are the length limits the prose lint holds statements to.
	Budgets *Budgets `json:"budgets,omitempty"`
}

// Licences are the SPDX expressions of the three kinds of content.
type Licences struct {
	// Rules is the licence of the rules and their text. Display only.
	Rules string `json:"rules"`
	// CheckerCode is the licence of hand-written checker code. Display only.
	CheckerCode string `json:"checker_code,omitempty"`
	// Presets is the licence of the generated presets. Display only.
	Presets string `json:"presets,omitempty"`
}

// Extends is a parent rulebook at a pin.
type Extends struct {
	// Rulebook is the parent's short name or full location. Decides: the
	// release is fetched from it.
	Rulebook string `json:"rulebook"`
	// Version is the pinned release, as semantic-version text. Decides.
	Version string `json:"version"`
	// Source is where releases come from. Decides.
	Source string `json:"source,omitempty"`
	// ContentHash is the content hash of the pinned release, in the form
	// internal/digest writes, given by the maintainer (init asks for it).
	// Decides: a fetched release that does not hash to it is refused.
	// Absent only until those land.
	ContentHash Digest `json:"content_hash,omitempty"`
}

// Import is parts taken by name from another rulebook.
type Import struct {
	// Rulebook is the imported rulebook's short name or full location.
	// Decides.
	Rulebook string `json:"rulebook"`
	// Version is the pinned release, as semantic-version text. Decides.
	Version string `json:"version"`
	// Source is where releases come from. Decides.
	Source string `json:"source,omitempty"`
	// Parts are the part slugs imported. Decides.
	Parts []string `json:"parts"`
	// ContentHash is the content hash of the pinned release, given by the
	// maintainer (init asks for it, S67; an import added later is an identity
	// edit). Decides: a fetched release that does not hash to it is
	// refused.
	ContentHash Digest `json:"content_hash,omitempty"`
}

// Governance is what a rulebook publishes about changing it.
type Governance struct {
	// Contribution is how the rulebook takes changes. Decides.
	Contribution Contribution `json:"contribution"`
	// CheckerPublishers are the owners who may publish checker code. Decides.
	CheckerPublishers []string `json:"checker_publishers,omitempty"`
	// CheckerSecondReviewer says checker code needs a second reviewer.
	CheckerSecondReviewer bool `json:"checker_second_reviewer,omitempty"`
	// CheckerProof is the proof level third-party checker code must reach.
	// Decides.
	CheckerProof ProofLevel `json:"checker_proof,omitempty"`
	// DecisionRequired says a promotion needs a recorded decision. Decides.
	DecisionRequired bool `json:"decision_required"`
	// TrialReleases is the longest a trial lasts, in releases. Decides.
	TrialReleases int `json:"trial_releases"`
	// ReviewDays is the default cadence of a rule's review-by date, in days.
	// Decides.
	ReviewDays int `json:"review_days"`
}

// Tested is the versions a release was proven on.
type Tested struct {
	// Tools are the tools and their versions.
	Tools []Version `json:"tools,omitempty"`
	// Plugins are the plugins and their versions.
	Plugins []Version `json:"plugins,omitempty"`
	// Kits are the kits and their versions.
	Kits []Version `json:"kits,omitempty"`
	// Adapters are the adapters and their versions.
	Adapters []Version `json:"adapters,omitempty"`
	// MinEngine is the oldest engine that reads the release, as
	// semantic-version text. Decides.
	MinEngine string `json:"min_engine"`
}

// Version is one named thing at one version.
type Version struct {
	// Name is the tool, plugin, kit or adapter, as a slug.
	Name string `json:"name"`
	// Version is its version, as the thing itself writes it.
	Version string `json:"version"`
}

// Budgets are the length limits of the prose lint.
type Budgets struct {
	// StatementWords is the most words a rule's statement has. Decides.
	StatementWords int `json:"statement_words,omitempty"`
	// ReasoningWords is the most words a rule's reasoning has. Decides.
	ReasoningWords int `json:"reasoning_words,omitempty"`
}
