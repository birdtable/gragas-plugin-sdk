// Package plugin is what a plugin for gragas is written on: a typed interface of the five
// operations, and Serve, which reads one request from stdin, calls the operation and writes one answer to stdout.
//
// A plugin implements Plugin and embeds Unsupported for the operations it does not provide, so each answers
// "unsupported" with no code of the author's. Serve answers an unknown op, target, purpose or aspect with
// "unsupported" too, ignores a request field it does not know, and ends a request it cannot read in a "bug" error
// (the behaviour checks C12 to C17 of the plugin contract). Every answer leaves sorted by the order rules of the plugin contract
// (3), so an author cannot get them wrong; a key or path given twice is the author's bug, said by name.
//
// The package runs natively under go test and builds for wasip1 with standard Go; it holds no clock, no
// randomness and no state, so the same request gives the same answer on every system (4.5 (1)). It imports the
// standard library and this module's wire package only.
package plugin

import "github.com/birdtable/gragas-plugin-sdk/wire"

// Plugin is the five operations of contract 1. An operation a plugin does not provide answers Unsupported;
// embed Unsupported and write the ones it does.
type Plugin interface {
	// Plan says what to run or read next, for a purpose and a target, given what has been gathered. It may
	// answer an error of the codes unsupported, refused (a config it must not hand to the tool), unreadable
	// (after round 0) and bug.
	Plan(wire.PlanRequest) (wire.PlanResponse, error)
	// Entries turns what the tool printed into entries. It is a pure function of the request.
	Entries(wire.EntriesRequest) (wire.EntriesResponse, error)
	// Findings turns a run over fixtures into findings.
	Findings(wire.FindingsRequest) (wire.FindingsResponse, error)
	// Rank says, for each pair, whether the found value is tighter, weaker or the same as the preset's, or
	// unknown. It answers the pairs by position.
	Rank(wire.RankRequest) (wire.RankResponse, error)
	// Preset writes the tool's file from entries. It may answer refused for an option it cannot write.
	Preset(wire.PresetRequest) (wire.PresetResponse, error)
}

// Unsupported is a Plugin that provides nothing: each operation answers unsupported. Embed it in a plugin, and
// write the methods of the operations it provides.
type Unsupported struct{}

// Plan answers unsupported.
func (Unsupported) Plan(wire.PlanRequest) (wire.PlanResponse, error) {
	return wire.PlanResponse{}, Unsupportedf("this plugin does not provide plan")
}

// Entries answers unsupported.
func (Unsupported) Entries(wire.EntriesRequest) (wire.EntriesResponse, error) {
	return wire.EntriesResponse{}, Unsupportedf("this plugin does not provide entries")
}

// Findings answers unsupported.
func (Unsupported) Findings(wire.FindingsRequest) (wire.FindingsResponse, error) {
	return wire.FindingsResponse{}, Unsupportedf("this plugin does not provide findings")
}

// Rank answers unsupported.
func (Unsupported) Rank(wire.RankRequest) (wire.RankResponse, error) {
	return wire.RankResponse{}, Unsupportedf("this plugin does not provide rank")
}

// Preset answers unsupported.
func (Unsupported) Preset(wire.PresetRequest) (wire.PresetResponse, error) {
	return wire.PresetResponse{}, Unsupportedf("this plugin does not provide preset")
}
