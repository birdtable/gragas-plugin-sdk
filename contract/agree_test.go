package contract_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/contract"
)

func TestAgree(t *testing.T) {
	for _, c := range []struct{ name, doc, want string }{
		{"the same", `{"contract":1,"features":[]}`, ""},
		{"the number off by one", `{"contract":2,"features":[]}`, "the SDK is written for 1, the pinned gragas speaks 2"},
		{"a feature the SDK lacks", `{"contract":1,"features":["streams"]}`, "the SDK has [], the pinned gragas has [streams]"},
		{"no features key", `{"contract":1}`, "lacks"},
		{"not a document", `x`, "not one this check reads"},
		{"an unknown key", `{"contract":1,"features":[],"more":1}`, "not one this check reads"},
	} {
		err := contract.Agree([]byte(c.doc))
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: err = %v, want it to hold %q", c.name, err, c.want)
		}
	}
}
