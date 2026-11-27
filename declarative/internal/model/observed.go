package model

// What an enforcer printed, read through its adapter: the diagnostics of a
// run and the entries of an effective config. Both come from a program the
// engine does not control, so every field says what it decides
// or that it is display only (arch.ports.19).

// Diagnostic is one finding an enforcer printed over fixtures.
type Diagnostic struct {
	// Key is the tool's entry the finding belongs to, such as a linter's or
	// a rule's name. Decides: which binding the finding proves.
	Key string
	// Message is the finding's text. Display only.
	Message string
	// Path is the file the finding is in, as the tool printed it. proof reads
	// it in the slash form only: relative to the fixture folder, or absolute
	// inside it. A path in another form (a backslash, a drive letter, a file:
	// URI) ends the proof as the tool's output not read. Decides: which
	// fixture the finding is in.
	Path string
	// Line is the finding's line, 0 when the tool printed none. Decides:
	// which line of the fixture the finding is on.
	Line int
	// Column is the finding's column, 0 when the tool printed none.
	// Display only.
	Column int
	// Severity is the tool's own word for the finding's severity, "" when
	// the tool printed none. Decides: the severity observed, through the
	// adapter's levels.
	Severity string
	// URL is the finding's docs URL, "" when the tool printed none.
	// Display only.
	URL string
}

// ToolEntry is one entry of a tool's effective config, as the adapter's
// entry map reads it.
type ToolEntry struct {
	// Key is the entry's key in the tool, such as a linter's name. Decides:
	// which entry a binding names.
	Key string
	// Value is the entry's value, a string as printed or any other value as
	// JSON on one line, "" where the adapter names none. Decides: the value the
	// entry stands at.
	Value string
	// Options is the entry's options as JSON on one line, "" where the adapter
	// names none. Decides: the options the entry stands at.
	Options string
	// Severity is the tool's word for the entry's severity, "" where the
	// adapter names none. Decides: the severity the entry stands at.
	Severity string
}
