package model

// Structure is structure.yaml: the layers, parts and concerns that give a
// rule its place, and the kinds that scope it. The order of each list is the
// order of the things it lists.
type Structure struct {
	// Format is the format version the file is written in.
	Format int `json:"format"`
	// Layers are the top level, in order.
	Layers []Layer `json:"layers"`
	// Kinds are the traits of a program the rulebook defines.
	Kinds []ProgramKind `json:"kinds"`
}

// Layer is a group of parts.
type Layer struct {
	// Slug is the layer's key. Decides.
	Slug string `json:"slug"`
	// Name is the layer's name. Display only.
	Name string `json:"name"`
	// Aliases are the slugs the layer had before it was renamed. Decides.
	Aliases []string `json:"aliases,omitempty"`
	// Parts are the layer's parts, in order.
	Parts []Part `json:"parts"`
}

// Part is a group of concerns with one guide.
type Part struct {
	// Slug is the part's key, the first word of its rules' ids. Decides.
	Slug string `json:"slug"`
	// Name is the part's name. Display only.
	Name string `json:"name"`
	// Aliases are the slugs the part had before it was renamed. Decides.
	Aliases []string `json:"aliases,omitempty"`
	// Owners are the people who own the part. Display only.
	Owners []string `json:"owners,omitempty"`
	// Concerns are the part's concerns, in order.
	Concerns []Concern `json:"concerns"`
}

// Concern is one subject of a part, with one rules file.
type Concern struct {
	// Slug is the concern's key, the second word of its rules' ids. Decides.
	Slug string `json:"slug"`
	// Name is the concern's name. Display only.
	Name string `json:"name"`
	// Aliases are the slugs the concern had before it was renamed. Decides.
	Aliases []string `json:"aliases,omitempty"`
}

// ProgramKind is a named trait of a program, such as service or library. A rule is
// scoped by kinds and a project declares several.
type ProgramKind struct {
	// Slug is the kind's key. Decides: a rule's scope and a project's pin name
	// it.
	Slug string `json:"slug"`
	// Name is the kind's name. Display only.
	Name string `json:"name"`
	// Description says what the kind is. Display only.
	Description string `json:"description,omitempty"`
}
