package plugin

import (
	"fmt"

	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// Unsupportedf is an error of the code unsupported: an operation, a purpose, a target, an aspect or a tool
// version the plugin does not serve. Valid in every operation.
func Unsupportedf(format string, args ...any) error {
	return &wire.Error{Code: wire.CodeUnsupported, Message: msg(format, args)}
}

// Unreadablef is an error of the code unreadable: the tool's output does not fit. path is a JSON pointer into
// the request body to the result at fault, for example /have/0/stdout. Valid in plan after round 0, entries and
// findings.
func Unreadablef(path, format string, args ...any) error {
	return &wire.Error{Code: wire.CodeUnreadable, Message: msg(format, args), Path: path}
}

// Refusedf is an error of the code refused: an input the plugin will not handle, a config that loads code or an
// option it cannot write. path is a JSON pointer into the request body, for example /entries/3/options. Valid in
// plan and preset.
func Refusedf(path, format string, args ...any) error {
	return &wire.Error{Code: wire.CodeRefused, Message: msg(format, args), Path: path}
}

// Bugf is an error of the code bug: the plugin's own fault. Valid in every operation. An error that is not one
// of these four is answered as a bug.
func Bugf(format string, args ...any) error {
	return &wire.Error{Code: wire.CodeBug, Message: msg(format, args)}
}

// msg is the message of an error: never empty, as the contract wants at least one character, and cut at the
// contract's 2048 bytes without breaking a character.
func msg(format string, args []any) string {
	s := format
	if len(args) > 0 {
		s = fmt.Sprintf(format, args...)
	}
	return clip(s, 2048, "an error with no message")
}

// clip is s cut to at most n bytes at a character boundary, or empty when s is empty.
func clip(s string, n int, empty string) string {
	if s == "" {
		return empty
	}
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !startsRune(s[cut]) {
		cut--
	}
	return s[:cut]
}

// startsRune is whether b begins a UTF-8 character and does not continue one.
func startsRune(b byte) bool { return b&0xC0 != 0x80 }
