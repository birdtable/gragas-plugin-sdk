package declarative

import "github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"

// Differs is differs, for the test that holds every field of the adapter to
// it.
func Differs(a, e model.Adapter) []string { return differs(a, e) }

// PointerClass is the class of characters a pointer's step may not hold, for the
// test that holds the schema's patterns to it.
const PointerClass = pointerClass
