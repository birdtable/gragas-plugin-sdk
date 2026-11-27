package declarative

import (
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/port"
)

// Layer is the adapter documents of one layer: each file's path is
// adapters/<tool>/<adapter-version>.yaml from the layer's root.
type Layer struct {
	Layer model.AdapterLayer
	Files []model.File
}

// Table is the adapters a run reads, each (tool, version) from the most
// specific layer that holds it. The zero Table holds none.
type Table struct {
	adapters []model.Adapter
}

// Load reads every layer, from the least specific, and answers the table, or
// every refusal and no table: one document refused refuses the table, so a
// layer that meant to override an adapter never falls back to the one below
// it unseen. The layers come in the order of model.AdapterLayers, each at
// most once; a layer with no adapters folder is left out.
//
// A rulebook's or a project's adapter is fetched data, so it only overrides
// an adapter the engine embeds, at the same tool and version, and only its
// paths: every
// argv stays the embedded one's. Each layer is held to the embedded adapter,
// never to the layer below it.
func Load(s port.Schema, fileName func(id string) string, layers ...Layer) (Table, []Refusal) {
	ld := &loader{s: s, fileName: fileName, embedded: map[[2]string]model.Adapter{}, byKey: map[[2]string]model.Adapter{}}
	order := model.AdapterLayers()
	last := -1
	for _, l := range layers {
		i := slices.Index(order, l.Layer)
		switch {
		case i < 0:
			ld.refuse(string(l.Layer), CheckLayer, "no such layer; the layers are embedded, rulebook and project")
			continue
		case i <= last:
			ld.refuse(string(l.Layer), CheckLayer, "this layer comes after a more specific one, or twice; the order is embedded, rulebook, project")
			continue
		}
		last = i
		if len(l.Files) > MaxDocuments {
			ld.refuse(string(l.Layer), CheckSize, "the layer holds more adapter documents than the cap allows")
			continue
		}
		ld.layer(l)
	}
	if len(ld.out) > 0 {
		return Table{}, ld.out
	}
	t := Table{}
	for _, a := range ld.byKey {
		t.adapters = append(t.adapters, a)
	}
	slices.SortFunc(t.adapters, func(a, b model.Adapter) int {
		if c := strings.Compare(a.Tool, b.Tool); c != 0 {
			return c
		}
		return Compare(a.Version, b.Version)
	})
	return t, nil
}

// loader gathers the table and the refusals of one Load.
type loader struct {
	s        port.Schema
	fileName func(id string) string
	out      []Refusal
	embedded map[[2]string]model.Adapter
	byKey    map[[2]string]model.Adapter
}

func (ld *loader) refuse(path string, c Check, msg string) {
	ld.out = append(ld.out, Refusal{Path: path, Check: c, Msg: msg})
}

// layer reads each file of l. An embedded adapter is the one a more specific
// layer is held to; any other is taken only as an override of one.
func (ld *loader) layer(l Layer) {
	seen := map[string]bool{}
	for _, f := range l.Files {
		if seen[f.Path] {
			ld.refuse(f.Path, CheckLayer, "the file is handed twice in one layer")
			continue
		}
		seen[f.Path] = true
		a, refused := Read(ld.s, l.Layer, f.Path, f.Data)
		if len(refused) > 0 {
			ld.out = append(ld.out, refused...)
			continue
		}
		key := [2]string{a.Tool, a.Version}
		if l.Layer == model.LayerEmbedded {
			ld.embedded[key] = a
		} else if refused := override(a, ld.embedded, ld.fileName); len(refused) > 0 {
			ld.out = append(ld.out, refused...)
			continue
		}
		ld.byKey[key] = a
	}
}

// All is every adapter, by tool and then by version from the oldest.
func (t Table) All() []model.Adapter { return slices.Clone(t.adapters) }

// Get is the adapter of tool at version, and whether the table holds it.
func (t Table) Get(tool, version string) (model.Adapter, bool) {
	i := slices.IndexFunc(t.adapters, func(a model.Adapter) bool { return a.Tool == tool && a.Version == version })
	if i < 0 {
		return model.Adapter{}, false
	}
	return t.adapters[i], true
}
