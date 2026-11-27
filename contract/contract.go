// Package contract is the plugin contract this SDK is written for: its number and the two schemas it was
// copied from gragas at one named commit.
//
// The SDK cannot import gragas, so it holds a copy of contract/plugin/v1/ of birdtable/gragas, and the commit it
// was copied at is the one line of PIN. The push tier compares the number and the features with the pinned
// gragas (gragas plugin contract --json), so a copy that has fallen behind is refused there.
package contract

import (
	"embed"
	"strings"
)

// Version is the contract number: the one the wire schema's request names, and the one a plugin's manifest
// declares.
const Version = 1

//go:embed v1/wire.schema.json v1/manifest.schema.json PIN
var files embed.FS

// Wire is the wire schema, as copied at Pin.
func Wire() []byte { return read("v1/wire.schema.json") }

// Manifest is the manifest schema, as copied at Pin.
func Manifest() []byte { return read("v1/manifest.schema.json") }

// Pin is the gragas commit the two schemas were copied at.
func Pin() string { return strings.TrimSpace(string(read("PIN"))) }

func read(name string) []byte {
	b, err := files.ReadFile(name)
	if err != nil {
		// The files are embedded at build time, so a miss is a broken build and not a runtime condition.
		panic("contract: " + err.Error())
	}
	return b
}
