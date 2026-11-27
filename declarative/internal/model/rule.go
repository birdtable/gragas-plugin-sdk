package model

// Strength is how firmly a rule binds.
type Strength string

// The three strengths.
const (
	Must   Strength = "must"
	Should Strength = "should"
	May    Strength = "may"
)

// Strengths is the closed set.
func Strengths() []Strength { return []Strength{Must, Should, May} }

// Status is the state of a rule that people type. It has four values and
// nothing else: standing (proposed, enforced, reviewed, planned, advisory,
// retired) is derived from the status, the bindings and their proof, and is
// never a field of any type here.
type Status string

// The four statuses.
const (
	Proposed Status = "proposed"
	Trial    Status = "trial"
	Active   Status = "active"
	Retired  Status = "retired"
)

// Statuses is the closed set.
func Statuses() []Status { return []Status{Proposed, Trial, Active, Retired} }

// Origin is how a rule came to be.
type Origin string

// The three origins.
const (
	Authored Origin = "authored"
	Imported Origin = "imported"
	Migrated Origin = "migrated"
)

// Origins is the closed set.
func Origins() []Origin { return []Origin{Authored, Imported, Migrated} }

// TargetType is what a pair or a binding is about.
type TargetType string

// The twelve target types.
const (
	TargetSource            TargetType = "source"
	TargetConfig            TargetType = "config"
	TargetMigration         TargetType = "migration"
	TargetContainer         TargetType = "container"
	TargetWorkflow          TargetType = "workflow"
	TargetIaC               TargetType = "iac"
	TargetAPISpec           TargetType = "api-spec"
	TargetProse             TargetType = "prose"
	TargetCommit            TargetType = "commit"
	TargetTelemetryRegistry TargetType = "telemetry-registry"
	TargetDependency        TargetType = "dependency"
	TargetPackage           TargetType = "package"
)

// TargetTypes is the closed set.
func TargetTypes() []TargetType {
	return []TargetType{TargetSource, TargetConfig, TargetMigration, TargetContainer, TargetWorkflow, TargetIaC,
		TargetAPISpec, TargetProse, TargetCommit, TargetTelemetryRegistry, TargetDependency, TargetPackage}
}

// Severity is how loudly a tool reports a binding's entry.
type Severity string

// The three severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Severities is the closed set.
func Severities() []Severity { return []Severity{SeverityError, SeverityWarning, SeverityInfo} }

// RunPoint is where in the work a binding runs. It is a field beside the
// rung: the rung says how the rule is held, the run point when.
type RunPoint string

// The seven run points.
const (
	RunEdit    RunPoint = "edit"
	RunCommit  RunPoint = "commit"
	RunPush    RunPoint = "push"
	RunLand    RunPoint = "land"
	RunReview  RunPoint = "review"
	RunRelease RunPoint = "release"
	RunDeploy  RunPoint = "deploy"
)

// RunPoints is the closed set.
func RunPoints() []RunPoint {
	return []RunPoint{RunEdit, RunCommit, RunPush, RunLand, RunReview, RunRelease, RunDeploy}
}

// Backing is what a citation of a source backs.
type Backing string

// The three things a citation backs.
const (
	BacksRule      Backing = "rule"
	BacksThreshold Backing = "threshold"
	BacksCheck     Backing = "check"
)

// Backings is the closed set.
func Backings() []Backing { return []Backing{BacksRule, BacksThreshold, BacksCheck} }

// Rung is how a binding holds its rule: 0 upstream preset, 1 configured
// built-in, 2 pattern, 3 own plugin or analyzer, 4 fact held by a policy
// tool, 5 test or kit, 6 review. Decides: standing is derived from it.
type Rung int

// MaxRung is the highest rung.
const MaxRung Rung = 6

// Rule is one rule, from a YAML file or a Markdown block. It carries no
// anchor: a section's anchor is written once on its RuleBlock, and Book.Anchors
// keeps it by rule id once the files are loaded, so a rule is the same value
// in either source form.
type Rule struct {
	// ID is the rule's frozen id. Decides.
	ID RuleID `json:"id"`
	// Aliases are the ids the rule had before it was renumbered. Decides.
	Aliases []RuleID `json:"aliases,omitempty"`
	// Statement is the rule in one sentence. Display only: the hash of its
	// text decides whether a translation is stale.
	Statement string `json:"statement"`
	// Strength is how firmly the rule binds. Decides.
	Strength Strength `json:"strength"`
	// Status is the rule's status. Decides.
	Status Status `json:"status"`
	// Scope is where the rule applies. Decides.
	Scope *RuleScope `json:"scope,omitempty"`
	// Exemptions are the scope exceptions, each with a reason. Decides.
	Exemptions []Exemption `json:"exemptions,omitempty"`
	// Reasoning is why the rule holds. Display only.
	Reasoning *Reasoning `json:"reasoning,omitempty"`
	// Pairs are the rule's example pairs, one folder each under pairs/.
	Pairs []Pair `json:"pairs,omitempty"`
	// Owner is who answers for the rule. Decides: the guard refuses a hand
	// edit of it, which only rule edit makes.
	Owner string `json:"owner,omitempty"`
	// ReviewBy is the date an active rule is next confirmed, as YYYY-MM-DD.
	// Decides: an overdue date is a finding.
	ReviewBy string `json:"review_by,omitempty"`
	// ZeroBy is the date a planned rule should have an effective binding, as
	// YYYY-MM-DD. Decides: a planned rule past it is an attention item.
	ZeroBy string `json:"zero_by,omitempty"`
	// Relations are the rule's links to other rules. Decides: the guard
	// refuses a hand edit of them.
	Relations *Relations `json:"relations,omitempty"`
	// Origin is how the rule came to be. Decides: an imported rule is
	// grandfathered.
	Origin Origin `json:"origin"`
	// Sources are what the rule cites. Decides: a rule leaves proposed only
	// with one.
	Sources []Citation `json:"sources,omitempty"`
	// Bindings are the tool entries that hold the rule. Decides.
	Bindings []Binding `json:"bindings,omitempty"`
	// ReplacedBy is the rule that took this one's place. Decides.
	ReplacedBy RuleID `json:"replaced_by,omitempty"`
}

// RuleScope is where a rule applies. A field given must hold and an absent field
// holds.
type RuleScope struct {
	// Parts are part slugs.
	Parts []string `json:"parts,omitempty"`
	// Paths are slash-separated globs from the project root, matched
	// case-sensitively against a tracked path.
	Paths []string `json:"paths,omitempty"`
	// Kinds are kind slugs of the structure.
	Kinds []string `json:"kinds,omitempty"`
}

// Exemption is a scope exception by path or kind.
type Exemption struct {
	// Paths are slash-separated globs from the project root, matched
	// case-sensitively against a tracked path. Decides.
	Paths []string `json:"paths,omitempty"`
	// Kinds are kind slugs of the structure. Decides.
	Kinds []string `json:"kinds,omitempty"`
	// Reason says why the exception holds. Display only.
	Reason string `json:"reason"`
}

// Reasoning is the explanation of a rule. All of it is display only.
type Reasoning struct {
	// Definition says what the rule means.
	Definition string `json:"definition,omitempty"`
	// Pros are the reasons for the rule.
	Pros []string `json:"pros,omitempty"`
	// Cons are the costs of the rule.
	Cons []string `json:"cons,omitempty"`
	// Decision is the slug of the decision that settles the rule. Decides: a
	// reference that must resolve.
	Decision string `json:"decision,omitempty"`
}

// Pair is a rule's example pair: a good and a bad example in the folder
// pairs/<rule id>/<number>/.
type Pair struct {
	// Number is the pair's folder number. Decides.
	Number int `json:"number"`
	// Target is what the pair's files are. Decides.
	Target TargetType `json:"target"`
}

// Relations are a rule's links to other rules, by id. Each decides.
type Relations struct {
	// Relates are rules about the same matter.
	Relates []RuleID `json:"relates,omitempty"`
	// Conflicts are rules that cannot both hold.
	Conflicts []RuleID `json:"conflicts,omitempty"`
	// Depends are rules this one needs.
	Depends []RuleID `json:"depends,omitempty"`
	// Covers are rules this one holds as written or stricter.
	Covers []RuleID `json:"covers,omitempty"`
}

// Citation is one use of a source by a rule.
type Citation struct {
	// Ref is the source's slug. Decides: it must resolve.
	Ref string `json:"ref"`
	// Where is the place inside the source. Display only.
	Where string `json:"where,omitempty"`
	// Says is what the source says there, at most forty words. Display only.
	Says string `json:"says,omitempty"`
	// Backs is what the citation backs. Decides.
	Backs Backing `json:"backs"`
}

// Binding holds a rule in one entry of one tool.
type Binding struct {
	// Tool is the tool's slug. Decides: an adapter is found by it.
	Tool string `json:"tool"`
	// Target is what the entry is about. Decides.
	Target TargetType `json:"target"`
	// Key is the tool's entry, or at rung 6 the review question. Decides.
	Key string `json:"key"`
	// Value is the entry's value, or at rung 6 when the question applies.
	// Decides.
	Value any `json:"value,omitempty"`
	// Options are the entry's options. Decides.
	Options map[string]any `json:"options,omitempty"`
	// Severity is how loudly the tool reports the entry. Decides.
	Severity Severity `json:"severity"`
	// Rung is how the binding holds the rule. Decides.
	Rung Rung `json:"rung"`
	// RunPoint is where in the work the binding runs. Decides.
	RunPoint RunPoint `json:"run_point"`
	// Remedy names the tool's fixer, which gragas never runs. Display only.
	Remedy string `json:"remedy,omitempty"`
	// Pair is the number of the pair that proves the binding. Decides.
	Pair int `json:"pair,omitempty"`
	// PairPath is the path the pair stands at in the fixture it is run in.
	// Decides.
	PairPath string `json:"pair_path,omitempty"`
	// Tightens is the parent's or imported rule this binding tightens, in an
	// overlay or an importer. Decides.
	Tightens RuleID `json:"tightens,omitempty"`
	// Checker is the name of the rulebook's checker-code declaration,
	// checkers/<name>/, the binding's entry comes from, when the entry is the
	// rulebook's own plugin or analyzer; "" for a tool's own entry.
	// Decides: which declared checker code the binding needs.
	Checker string `json:"checker,omitempty"`
}

// RuleBlock is the YAML of one fenced gragas block of a Markdown guide: the
// section's anchor, once, and the rules the section's prose shares.
type RuleBlock struct {
	// Anchor is the section's anchor. Decides: a rename of the anchor is a
	// history entry and a redirect, never an id change.
	Anchor string `json:"anchor"`
	// Rules are the rules of the section.
	Rules []Rule `json:"rules"`
}
