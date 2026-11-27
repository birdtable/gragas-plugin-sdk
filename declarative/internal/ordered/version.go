package ordered

// VersionIs is what Version found in a versioned file's version key.
type VersionIs int

const (
	// VersionRead is a whole number from 1 up to the reader's own version: the
	// file is read at it.
	VersionRead VersionIs = iota
	// VersionNewer is a whole number above the reader's own: the file is not
	// ready for this reader, and the caller names both numbers.
	VersionNewer
	// VersionNotWhole is no whole number at all: a word, a fraction, a list,
	// or no value. It is not a version.
	VersionNotWhole
	// VersionBelowOne is a whole number below 1. It is not a version either.
	VersionBelowOne
)

// Version is the one rule for the version key of a file gragas reads: v is
// the key's value as this package parses it, and reads is the newest version
// the reader knows. A version is a whole number of at least 1. One above
// reads is newer than the reader, and the file is not ready for it. Version
// answers the number with its verdict, and words nothing: each file's reader
// writes its own refusal, naming its file and line. A missing key (nil) is
// VersionNotWhole, and never read as the reader's own version.
func Version(v any, reads int) (int64, VersionIs) {
	n, ok := v.(int64)
	switch {
	case !ok:
		return 0, VersionNotWhole
	case n > int64(reads):
		return n, VersionNewer
	case n < 1:
		return n, VersionBelowOne
	}
	return n, VersionRead
}
