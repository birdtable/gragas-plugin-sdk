package model

// The values that cross a port (arch.ports.10). They are data: no method
// here reads a disk, a clock or a process. Every field says its unit.

// Root names one root the engine reads or writes: a rulebook's repository, a
// project, or the engine's own state folder. It is the absolute folder the
// Tree opened it at, slash-separated, and is only ever a value Tree.Open or
// Tree.Locate answered with; a path built by hand is not a root.
type Root string

// Digest is a digest as canon writes it: "sha256:" and sixty-four lower-case
// hexadecimal digits. The empty Digest is no digest; the zero value of a
// field of this type says the digest was not taken.
type Digest string

// Entry is one file or folder of a root, as a listing sees it.
type Entry struct {
	// Path is relative to the root, slash-separated, never with a leading
	// slash and never climbing.
	Path string
	// Size is the file's length in bytes; 0 for a folder.
	Size int64
	// Dir is true for a folder.
	Dir bool
	// Exec is true when the system would run the file: a regular file it
	// finds runnable. It is false for a link.
	Exec bool
	// Link is true for a symbolic link that a repository tracks, read from
	// git's own mode field (120000), never by a stat of the work tree; its
	// Size is the length of the path it names. Only History.Files sets it.
	Link bool
	// Special is true for a file that is neither a folder nor a regular file,
	// as the disk lists it: a named pipe, a socket, a device, or a symbolic
	// link. Its content is not a file's, and opening one may wait on another
	// program, so a reader of entries refuses it before the open.
	// Only the Tree port's listing sets it.
	Special bool
}

// File is one file's path and whole content, for a port that is handed files:
// the fixtures a sandbox places, the schemas a schema port is given.
type File struct {
	// Path is relative to the folder it is placed in, slash-separated.
	Path string
	// Data is the content; nil and empty are the same empty file.
	Data []byte
}

// Scope says which files of a root a listing names.
type Scope string

// The three scopes of a listing.
const (
	// Tracked is the files the root's history holds.
	Tracked Scope = "tracked"
	// Untracked is the files on disk its history does not hold.
	Untracked Scope = "untracked"
	// Every is both.
	Every Scope = "every"
)

// Problem is one way a document fails a schema: the line of the place in the
// document that failed (0 when unknown) and a sentence saying how.
type Problem struct {
	Line int
	Msg  string
}

// RecordVersion is the version of the journal's line this engine writes and
// the newest it reads. A line of an older version is read as it is; one of a
// newer version is refused, saying both.
const RecordVersion = 1

// The four kinds of a journal record.
const (
	// KindIntent is the whole plan, written before the first effect.
	KindIntent = "intent"
	// KindCopy says a file's old content is kept, and its digest.
	KindCopy = "copy"
	// KindStep says one effect is about to be made, or was.
	KindStep = "step"
	// KindClose ends a run, with each written file's copy and digest after.
	KindClose = "close"
)

// Record is one line of a run's journal. The fixed fields are the same
// whoever did the work; what a tool or the platform adds goes in Detail, one
// JSON value the journal passes through and never reads inside. The journal
// holds argv, paths and digests, never a token, a key or a credential.
type Record struct {
	// Version is the version of the line. The journal sets it on a write; a
	// read answers the version the line had.
	Version int
	// Run is the id of the run the line belongs to.
	Run string
	// Kind is one of KindIntent, KindCopy, KindStep, KindClose.
	Kind string
	// At is when the line was written, in seconds since the Unix epoch,
	// UTC. The journal sets it on a write.
	At int64
	// Command is the command that did the work, as a person types it.
	Command string
	// Plan is the digest of the plan the run carries out.
	Plan string
	// Root is the digest of the root the journal is of. The journal sets it
	// on a write.
	Root string
	// Engine is the version of the engine that wrote the line.
	Engine string
	// Detail is one JSON value, or nothing.
	Detail []byte
}

// Lock names one of the two locks a run takes. The order is Root first and
// Scratch second, never the reverse.
type Lock string

// The two locks.
const (
	// RootLock keeps two writers off one root.
	RootLock Lock = "root"
	// ScratchLock keeps proof runs and a clear off one another's scratch
	// folders. The scratch folders are made in one folder for the whole
	// machine, whatever root a run is of, so the scratch lock is one lock
	// for the machine: the root a Take names is the taker's, and a run of any
	// other root that holds it holds it for this one too.
	ScratchLock Lock = "scratch"
)

// Hold is a lock that was taken, for Lock.Release to put back.
type Hold struct {
	Root  Root
	Which Lock
	// Since is when the holder took it, in seconds since the Unix epoch, UTC.
	Since int64
}

// Level is how isolated a prepared run is.
type Level string

// The two levels (architecture 4.2).
const (
	// Isolated is a scratch folder with a scrubbed environment and limits.
	Isolated Level = "isolated"
	// Sandboxed is the same inside a container or the OS sandbox, denying
	// network and file access beyond the folder.
	Sandboxed Level = "sandboxed"
)

// Scratch is a prepared folder and the level it reached.
type Scratch struct {
	// Dir is the absolute folder, slash-separated.
	Dir string
	// Level is the level reached, which may be lower than the one asked.
	Level Level
	// Seconds is the deadline a run in the folder is held to: the one
	// Prepare was asked for, or the adapter's own bound in place of a 0. It
	// is never 0.
	Seconds int
	// MemoryMiB is the memory limit a run in the folder is held to, the one
	// Prepare was asked for; 0 is no limit, since gragas sets none of its
	// own.
	MemoryMiB int
}

// Prepare is what a sandbox is asked to set up.
type Prepare struct {
	// Level is the level asked for.
	Level Level
	// Fixtures are placed at their declared paths in the folder.
	Fixtures []File
	// Config is the config set, mirrored into the folder.
	Config []File
	// Seconds is the deadline of the run, in whole seconds; 0 is the
	// adapter's own bound, never none.
	Seconds int
	// MemoryMiB is the memory limit in mebibytes; 0 is no limit, never a
	// bound of the adapter's own. Below 0 is refused.
	MemoryMiB int
	// Resolve is the folder the tool finds its plugins in, linked into the
	// folder; the zero Link links none.
	Resolve Link
}

// Link is a folder outside a scratch folder that a run reaches by a link
// inside it: the adapter's resolve-from folder (architecture 4.1). It is
// linked, never copied. Only the sandboxed level stops a tool writing
// through it.
type Link struct {
	// Path is where the link is placed, relative to the scratch folder and
	// slash-separated: the adapter's resolve_from.
	Path string
	// Dir is the folder it links to, absolute and slash-separated (a
	// drive's folder is written C:/dir), which
	// must be a folder and not itself a link.
	Dir string
}

// Call is one run of an enforcer tool, by a shape its adapter data declared.
type Call struct {
	// Tool is the tool's name as the adapter data names it.
	Tool string
	// Version is the tool version the run expects, major.minor.patch. A call
	// without one is refused: no call takes whatever version was found.
	Version string
	// Shape names one of the argv shapes the adapter data declares.
	Shape string
	// Values fill the shape's placeholders; a key the shape does not name is
	// refused.
	Values map[string]string
	// Dir is the folder the tool runs in, absolute and slash-separated.
	Dir string
	// Scratch is the prepared folder the run is held to: Dir is its Dir,
	// and its limits are the run's. The zero Scratch is a run in no
	// prepared folder, held to the adapter's own deadline alone.
	Scratch Scratch
}

// Output is what a tool printed.
type Output struct {
	// Code is the tool's exit code.
	Code int
	// Stdout is what it printed, cut at the adapter's cap.
	Stdout []byte
	// Cut is true when Stdout was cut at the cap.
	Cut bool
	// JudgedAfter is true when the run was held to a memory limit that this
	// system could not hold while it ran, so its peak was judged against the
	// limit after the run alone: nothing stopped it on the way.
	JudgedAfter bool
}

// Found is a tool that was located.
type Found struct {
	// Path is the absolute path of the executable.
	Path string
	// Script, when set, is the script Path runs: Path is then the interpreter
	// (node) and the script is put first in argv.
	Script string
	// Version is the version the tool reports.
	Version string
	// Step is the step of the locate order that held it: settings,
	// adapter or tools-file.
	Step string
}

// Identity is the name and address a repository commits as.
type Identity struct {
	Name, Email string
}

// Tag is a tag of a repository and the commit it names.
type Tag struct {
	// Name is the tag's name.
	Name string
	// Commit is the full object name of the commit, forty or sixty-four
	// lower-case hexadecimal digits.
	Commit string
}

// Commit is a commit of a repository and the commits it was made on.
type Commit struct {
	// ID is the commit's full object name, forty or sixty-four lower-case
	// hexadecimal digits.
	ID string
	// Parents are the full object names of its parents, in the commit's
	// order: none for a first commit, two or more for a merge.
	Parents []string
}

// Message is one commit's message, as its object stores it.
type Message struct {
	// Commit is the full object name of the commit, forty or sixty-four
	// lower-case hexadecimal digits.
	Commit string
	// Text is the whole message, byte for byte as the commit's object holds
	// it after its header, under a header that names no encoding or UTF-8:
	// its newlines, its blank lines, a line that reads like a header, and a
	// byte not valid in UTF-8 are all kept. It holds no NUL.
	Text string
}
