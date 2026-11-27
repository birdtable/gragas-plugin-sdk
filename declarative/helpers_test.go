package declarative_test

import (
	"os"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/port"
)

// Helpers of gragas's link_test.go that the copied tests share, verbatim: that file needs gragas's fake Schema
// world and stays out of this copy (see SOURCE).

func read(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// stubSchema is the Schema port of these tests: every document is valid. gragas's own tests ran a fake world with
// the adapter's schema compiled, and planted problems in it; the schema check of the adapter files is the plugins
// repository's gate, and the one test that needs a real refusal stays in gragas (see SOURCE).
type stubSchema struct{}

func (stubSchema) Check(string, []byte) ([]model.Problem, error) { return nil, nil }

// schema is gragas's helper of the same name, with the stub in place of the fake world; the second answer is the
// world's, which no test here plants in.
func schema(*testing.T) (port.Schema, struct{}) { return stubSchema{}, struct{}{} }

// TestMain runs the tests as they are: gragas's configtest.Main isolated the environment of a program that reads
// files and runs tools, and these tests do neither.
func TestMain(m *testing.M) { os.Exit(m.Run()) }
