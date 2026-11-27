package port

import "github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"

// Schema checks a document against a named schema. This is the one method of gragas's port.Schema that the reader calls.
type Schema interface {
	Check(name string, doc []byte) ([]model.Problem, error)
}
