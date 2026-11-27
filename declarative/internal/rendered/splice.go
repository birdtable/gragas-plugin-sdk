// Package rendered holds the one body that replaces what stands between two
// marker lines of a text. It stays in gragas until a second live program needs it.
package rendered

import (
	"bytes"
	"errors"
)

// The refusals of Splice. Each is a plain value: the caller adds the name of
// the file with %w, and Splice names no file.
var (
	// ErrNoMarker is the answer when the text holds no line for the begin
	// marker, or none for the end marker.
	ErrNoMarker = errors.New("a marker line is missing")
	// ErrMarkerTwice is the answer when a marker stands on two lines or more.
	ErrMarkerTwice = errors.New("a marker line stands twice")
	// ErrMarkerOrder is the answer when the end marker stands before the begin.
	ErrMarkerOrder = errors.New("the end marker stands before the begin marker")
)

// Splice answers text with what stands between the begin and the end line
// replaced by block. Both marker lines stay. A line is a marker when it
// equals the marker after its leading spaces and tabs and one trailing \r are
// taken off; a line ends at \n, so a marker line that ends \r\n is found, and
// every byte of text outside the markers, line ends included, is kept as it
// is. block is lines, each ending in \n, written as given except that each
// line that holds more than its \n begins with the indent of the begin line.
// An empty block leaves the two marker lines side by side.
//
// It refuses with ErrNoMarker, ErrMarkerTwice or ErrMarkerOrder. It reads no
// file and calls no compare.
func Splice(text []byte, begin, end string, block []byte) ([]byte, error) {
	var bAt, eAt, bEnd, nb, ne int
	var indent []byte
	for at := 0; at < len(text); {
		next := len(text)
		if i := bytes.IndexByte(text[at:], '\n'); i >= 0 {
			next = at + i + 1
		}
		core, lead := markerCore(text[at:next])
		switch string(core) {
		case begin:
			if nb++; nb == 1 {
				bAt, bEnd, indent = at, next, lead
			}
		case end:
			if ne++; ne == 1 {
				eAt = at
			}
		}
		at = next
	}
	switch {
	case nb == 0 || ne == 0:
		return nil, ErrNoMarker
	case nb > 1 || ne > 1:
		return nil, ErrMarkerTwice
	case eAt < bAt:
		return nil, ErrMarkerOrder
	}
	var out bytes.Buffer
	out.Write(text[:bEnd])
	for len(block) > 0 {
		line := block
		if i := bytes.IndexByte(block, '\n'); i >= 0 {
			line = block[:i+1]
		}
		block = block[len(line):]
		if len(bytes.TrimSpace(line)) > 0 {
			out.Write(indent)
		}
		out.Write(line)
	}
	out.Write(text[eAt:])
	return out.Bytes(), nil
}

// markerCore is a line without its line end, one trailing \r and its leading
// spaces and tabs, and those leading bytes.
func markerCore(line []byte) (core, lead []byte) {
	line = bytes.TrimSuffix(line, []byte("\n"))
	line = bytes.TrimSuffix(line, []byte("\r"))
	core = bytes.TrimLeft(line, " \t")
	return core, line[:len(line)-len(core)]
}
