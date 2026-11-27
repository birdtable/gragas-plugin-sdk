package canon

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Compare orders two strings as RFC 8785 orders an object's keys: by their
// UTF-16 code units, each read as an unsigned number, a shorter string first
// when it is the start of the longer. It is -1 when a comes first, 1 when b
// does, and 0 only when a and b are the same bytes.
//
// For text below U+10000 that is code point order, which is Go's byte order
// of UTF-8. The two part only on a code point above U+FFFF, whose first code
// unit is a surrogate (U+D800 to U+DBFF) and so sorts ahead of U+E000 to
// U+FFFF: U+1F600 comes before U+FB33 here, and after it in byte order.
// Bytes that are not UTF-8 have no code units; Compare orders them by their
// bytes, so the order stays total, and the encoder refuses them anyway.
func Compare(a, b string) int {
	for a != "" && b != "" {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		if a[:na] == b[:nb] {
			a, b = a[na:], b[nb:]
			continue
		}
		if ua, ub := unit(ra), unit(rb); ua != ub {
			return cmp.Compare(ua, ub)
		}
		if ra != rb {
			// Two code points above U+FFFF with one first code unit: their
			// second units are in the order of the code points.
			return cmp.Compare(ra, rb)
		}
		// One rune from two spellings: bytes that are not UTF-8.
		return strings.Compare(a, b)
	}
	return cmp.Compare(len(a), len(b))
}

// unit is the first UTF-16 code unit of r.
func unit(r rune) rune {
	if r < 0x10000 {
		return r
	}
	return 0xd800 + (r-0x10000)>>10
}

// SortedKeys is m's keys in the order Compare gives, the one way to range over
// a map whose order could reach output, and the order the encoder writes an
// Object's keys in. A nil or empty map gives an empty slice.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, Compare)
	return keys
}

// SortByID sorts s in place by the id each element gives, in the order
// Compare gives the ids. Ids must be unique: two elements with one id would
// keep the order they arrived in, which could be a map's, so SortByID refuses
// a repeat with DuplicateID naming the id. s is sorted by then all the same;
// the order of the elements that share an id is the one thing not settled.
func SortByID[T any](s []T, id func(T) string) error {
	slices.SortStableFunc(s, func(a, b T) int { return Compare(id(a), id(b)) })
	for i := 1; i < len(s); i++ {
		if k := id(s[i]); k == id(s[i-1]) {
			return &Error{Kind: DuplicateID, Offset: -1, Detail: strconv.Quote(k) + " twice"}
		}
	}
	return nil
}
