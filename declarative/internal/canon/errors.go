package canon

import "strconv"

// Kind says why canon refused a value, an input or a call. The set is
// closed: the constants below, each with the word String returns. A caller
// that turns a refusal into a word of its own switches on Kind, never on the
// text of an Error.
type Kind int

// The kinds of refusal.
const (
	// BadSyntax is input that is not one JSON value with only white space
	// around it.
	BadSyntax Kind = iota + 1
	// BadUTF8 is a string that is not valid UTF-8, or an escape that names
	// half of a surrogate pair.
	BadUTF8
	// DuplicateKey is an object that names one key twice.
	DuplicateKey
	// TooLarge is a number whose magnitude is past MaxSafe, which a binary64
	// reader would round, or a number too large for a binary64 at all.
	TooLarge
	// NotFinite is a Float that is NaN or an infinity, which JSON cannot
	// write.
	NotFinite
	// Underflow is a number other than zero too small for a binary64, which a
	// reader would read as 0.
	Underflow
	// TooDeep is a value that nests deeper than MaxDepth.
	TooDeep
	// NilValue is a nil Value where a value was due.
	NilValue
	// Misuse is a value outside canon's set, or a layout that is not one of
	// the three.
	Misuse
	// BadDigest is text that is not a digest's one form.
	BadDigest
	// DuplicateID is two elements SortByID found with one id.
	DuplicateID
)

// String is the kind's word, as an Error's text starts.
func (k Kind) String() string {
	switch k {
	case BadSyntax:
		return "bad syntax"
	case BadUTF8:
		return "bad utf-8"
	case DuplicateKey:
		return "duplicate key"
	case TooLarge:
		return "number too large"
	case NotFinite:
		return "number not finite"
	case Underflow:
		return "number underflows"
	case TooDeep:
		return "too deep"
	case NilValue:
		return "nil value"
	case Misuse:
		return "misuse"
	case BadDigest:
		return "bad digest"
	case DuplicateID:
		return "duplicate id"
	default:
		return "kind " + strconv.Itoa(int(k))
	}
}

// Error is one refusal. canon returns it as *Error from every function and
// method that can refuse; errors.As finds it.
type Error struct {
	// Kind is why: one of the Kind constants.
	Kind Kind
	// Offset is the byte offset into the input where Decode or ParseDigest
	// stopped, and -1 for a refusal of the encoder or of SortByID.
	Offset int
	// Path is the JSON Pointer (RFC 6901) of the refused place inside the
	// value being encoded, "" for the top or for a refusal of Decode,
	// ParseDigest or SortByID.
	Path string
	// Detail is display only: what was found, for a person to read.
	Detail string
}

// Error is the refusal in one line: the kind's word, where, and the detail.
func (e *Error) Error() string {
	s := "canon: " + e.Kind.String()
	switch {
	case e.Offset >= 0:
		s += " at byte " + strconv.Itoa(e.Offset)
	case e.Path != "":
		s += " at " + strconv.Quote(e.Path)
	}
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	return s
}
