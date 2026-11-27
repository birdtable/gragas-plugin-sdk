package contract_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/contract"
)

// TestThePinIsOneFullCommitOnOneLine: PIN names the gragas commit the schemas were copied at, as 40 hex digits and
// one newline, so a reviewer reads one line and a script compares it.
func TestThePinIsOneFullCommitOnOneLine(t *testing.T) {
	t.Parallel()
	if got := contract.Pin(); !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(got) {
		t.Errorf("the pin is %q, want 40 hex digits", got)
	}
}

// TestTheTwoSchemasReadAndNameThemselves: the copies are JSON, each carries its $id as the engine's contract
// folder spells it, and the version is contract 1.
func TestTheTwoSchemasReadAndNameThemselves(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		raw []byte
		id  string
	}{
		"wire":     {contract.Wire(), "contract/plugin/v1/wire.schema.json"},
		"manifest": {contract.Manifest(), "contract/plugin/v1/manifest.schema.json"},
	} {
		var doc struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(c.raw, &doc); err != nil || doc.ID != c.id {
			t.Errorf("%s: $id %q, %v; want %q", name, doc.ID, err, c.id)
		}
		if !strings.HasSuffix(string(c.raw), "\n") {
			t.Errorf("%s does not end in a newline", name)
		}
	}
	if contract.Version != 1 {
		t.Errorf("Version = %d, want 1", contract.Version)
	}
}
