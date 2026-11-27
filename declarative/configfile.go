package declarative

// ConfigFile is a config file of a project: its path and its bytes. It is what ReadFileEntries is given, and it is
// defined here as in gragas's configs.go (the engine's side of the plan rounds, which does not move): the same two fields.
type ConfigFile struct {
	Path string
	Data []byte
}
