package model

// A Finding is one thing a check, validation or proof says is wrong: the
// entity it names and the fix it offers.
type Finding struct {
	Entity string `json:"entity"`
	Msg    string `json:"msg"`
	Fix    string `json:"fix,omitempty"`
	// File is the file it is about, slash-separated and relative to the
	// repository's root, "" when it is about no file; Line is the line, 0
	// for a whole file. Entity is for display and is never read back for
	// either.
	File string `json:"-"`
	Line int    `json:"-"`
}

// A Notice is one thing a check lists as information. It never changes the
// closing word: a stale translation is one.
type Notice struct {
	Entity string `json:"entity"`
	Msg    string `json:"msg"`
	// File and Line are as a Finding's.
	File string `json:"-"`
	Line int    `json:"-"`
}

// A Result is what a new command ends with when it prints no document of its
// own (cli.output.6): the word, what it changed, the next line, the kind of
// error and the run id. The old coreapi.Result stays for check and gen.
type Result struct {
	Word     Word      `json:"word"`
	Changes  []string  `json:"changes,omitempty"`
	Next     string    `json:"next,omitempty"`
	Kind     Kind      `json:"kind,omitempty"`
	Run      string    `json:"run,omitempty"`
	Findings []Finding `json:"findings,omitempty"`
	Notices  []Notice  `json:"notices,omitempty"`
}
