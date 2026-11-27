// Package canon is gragas's one encoder of JSON documents and its one digest:
// the same value gives the same bytes, and so the same digest, on every run
// and every machine. It sums through internal/digest, gragas's
// one owner of SHA-256, and imports nothing else of gragas's.
//
// The canonical form is RFC 8785, the JSON Canonicalization Scheme:
// keys in order of their UTF-16 code units, numbers in the form ECMAScript
// writes a binary64, strings with the RFC's minimal escaping, and no white
// space. canon writes none of those bytes itself: it checks a value against
// its limits and hands it to encoding/json/jsontext's Value.Canonicalize, the
// standard library's implementation of the RFC. Nothing else in gragas imports
// jsontext, so a stock JCS implementation anywhere reads and reproduces canon's
// bytes and digests.
//
// A value is built from the closed set of types below. canon adds nothing to
// it: it reads no clock, so a date in a document is a field the caller set.
// Decode reads JSON written by anyone back into a value, and refuses what two
// readers could read two ways.
package canon

// MaxDepth is the most arrays and objects one value may nest, counting the
// outermost. The encoder and Decode refuse a deeper value with TooDeep, so a
// value that holds itself ends in a refusal and hostile input in a bounded
// stack.
const MaxDepth = 512

// MaxSafe is the largest whole number a binary64 holds with every whole
// number below it, 2^53 - 1. RFC 8785 reads every number as a binary64, so a
// whole number past it would be rounded by any reader. gragas has none: the
// encoder and Decode refuse a number whose magnitude is past MaxSafe with
// TooLarge, never rounding it.
const MaxSafe = 1<<53 - 1

// Value is one JSON value: Null, Bool, Int, Float, String, Array or Object.
// The set is closed: no type outside this package implements Value. A nil
// Value is not a value. The encoder refuses it with NilValue, so a field with
// no value is left out of its Object, and a JSON null is Null{}.
type Value interface{ canonValue() }

// Null is JSON null, written null.
type Null struct{}

// Bool is JSON true or false.
type Bool bool

// Int is a whole number from -MaxSafe to MaxSafe, written in decimal with a
// leading minus when it is negative and with no plus, no leading zero and no
// point. One past either end is refused with TooLarge.
type Int int64

// Float is a finite binary64 number from -MaxSafe to MaxSafe, written as RFC
// 8785 writes it: the shortest decimal that reads back to the same float, with
// an exponent below 1e-6 (1.5e-7) and with neither point nor exponent when it
// is whole (Float(1) is 1, as Int(1) is). Negative zero is written 0. NaN and
// the infinities are refused with NotFinite, and a magnitude past MaxSafe with
// TooLarge. Since a whole Float and its Int have one encoding, Decode reads a
// whole number back as an Int.
type Float float64

// String is text, which must be valid UTF-8; the encoder refuses other bytes
// with BadUTF8. canon keeps a string's code points as they are and does not
// normalise Unicode, as RFC 8785 does not, so a composed and a decomposed
// accent are two strings with two encodings.
type String string

// Array is a list of values, written in its own order. A nil Array is written
// as [], as an empty one is.
type Array []Value

// Object is a set of members keyed by name, written in the order Compare
// gives the keys, which is RFC 8785's order of UTF-16 code units. The order
// the map was built or ranged in never reaches the bytes. A nil Object is
// written as {}, as an empty one is.
type Object map[string]Value

func (Null) canonValue()   {}
func (Bool) canonValue()   {}
func (Int) canonValue()    {}
func (Float) canonValue()  {}
func (String) canonValue() {}
func (Array) canonValue()  {}
func (Object) canonValue() {}
