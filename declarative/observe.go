package declarative

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/ordered"
)

// Observe reads the effective config a tool printed (config, as the read of
// the adapter says it is printed) and says, for each entry of the resolved
// preset, how the project's config stands against it: as-set, tighter, weaker,
// absent or excluded, and unmapped for each entry of the config no preset
// entry explains (docs/spec.md 17.4).
//
// It is pure: no file, no tool, no clock. The caller resolves the chain and
// hands the preset entries with their provenance, and the release the preset
// is of is the caller's choice. The comparison reads adapter data alone:
// the severities' order is the engine's (error over warning over info) read
// through the adapter's levels, an entry whose severity is one of the
// adapter's off words is below every level, and options are compared as sets
// of leaves, so options that are a strict subset of the preset's are tighter
// and a strict superset looser; a list item is a leaf by its place in the
// list, so a list in another order is no as-set. No tool is
// named and no language has a collector here.
//
// A value that differs from the preset's is ranked by the adapter's order of
// values, an off value below every value of the order.
//
// It never guesses. A config that does not read as the adapter says gives
// Unknown with the reason and no entries. An entry whose standing cannot be
// established, such as a value that differs where the adapter gives no order
// for values, or a value in neither its order nor its off values, carries a
// Reason and no word. Where one aspect of an entry is
// looser the entry is weaker whatever its other aspects, and where one is
// tighter and none is looser it is tighter. Two preset entries that hold one
// tool entry (an entry conflict) are one observation, made against the
// first that no overlay excluded; the conflict is the rulebook's finding.
func Observe(a model.Adapter, read model.ConfigRead, config []byte, preset []model.PresetEntry) model.Observed {
	if read.Entries.Each == "" {
		return model.Observed{Unknown: fmt.Sprintf("the adapter for %s reads no effective config", a.Tool)}
	}
	entries, err := readEntries(read, config)
	if err != nil {
		return model.Observed{Unknown: err.Error()}
	}
	held := map[string][]entry{}
	for _, e := range entries {
		held[e.Key] = append(held[e.Key], e)
	}
	var keys []string
	groups := map[string][]model.PresetEntry{}
	for _, p := range preset {
		if p.Binding.Tool != a.Tool || p.Binding.Target != read.Target {
			continue
		}
		if _, seen := groups[p.Binding.Key]; !seen {
			keys = append(keys, p.Binding.Key)
		}
		groups[p.Binding.Key] = append(groups[p.Binding.Key], p)
	}
	var out []model.Observation
	for _, key := range keys {
		out = append(out, observeEntry(a, read, key, groups[key], held[key]))
	}
	for _, e := range entries {
		if _, named := groups[e.Key]; named || seenKey(out, e.Key) {
			continue
		}
		out = append(out, model.Observation{Key: e.Key, Word: model.ObservedUnmapped})
	}
	return model.Observed{Entries: out}
}

func seenKey(out []model.Observation, key string) bool {
	return slices.ContainsFunc(out, func(o model.Observation) bool { return o.Key == key })
}

// provenance is where a preset entry came from.
func provenance(p model.PresetEntry) model.Provenance {
	return model.Provenance{Layer: p.Layer, Decision: p.Decision, ExcludedBy: p.Excluded}
}

// observeEntry is the observation of one tool entry: the preset entries that
// hold it, and the config entries with its key.
func observeEntry(a model.Adapter, read model.ConfigRead, key string, preset []model.PresetEntry, held []entry) model.Observation {
	live := slices.IndexFunc(preset, func(p model.PresetEntry) bool { return p.Excluded == nil })
	if live < 0 {
		// Every rule that holds the entry was dropped: the config shows it
		// as unmapped and says who dropped it, or does not hold it.
		o := model.Observation{Key: key, Word: model.ObservedExcluded, Provenance: provenance(preset[0])}
		if len(held) > 0 {
			o.Word = model.ObservedUnmapped
		}
		return o
	}
	p := preset[live]
	o := model.Observation{Key: key, Provenance: provenance(p)}
	switch {
	case len(held) == 0:
		o.Word = model.ObservedAbsent
	case len(held) > 1:
		o.Reason = "the effective config holds the entry " + fmt.Sprint(len(held)) + " times, so no one of them is the one that runs"
	default:
		o.Word, o.Reason = compare(a, read, p.Binding, held[0])
	}
	return o
}

// aspect is how one part of an entry stands against the preset's: looser,
// the same, tighter, or unread with the reason.
type aspect struct {
	rank   int
	reason string
}

const (
	looser  = -1
	same    = 0
	tighter = 1
)

// compare is the word for an entry the config holds once, or the reason none
// can be given.
func compare(a model.Adapter, read model.ConfigRead, b model.Binding, e entry) (model.ObservationWord, string) {
	aspects := []aspect{severityAspect(a, read, b, e), valueAspect(a.Values, read, b, e), optionsAspect(read, b, e)}
	var reason string
	rank := same
	for _, x := range aspects {
		switch {
		case x.reason != "":
			if reason == "" {
				reason = x.reason
			}
		case x.rank == looser:
			return model.ObservedWeaker, ""
		case x.rank == tighter:
			rank = tighter
		}
	}
	switch {
	case reason != "":
		return "", reason
	case rank == tighter:
		return model.ObservedTighter, ""
	}
	return model.ObservedAsSet, ""
}

// severityRank is the engine's order, with off below every severity.
func severityRank(s model.Severity) int {
	switch s {
	case model.SeverityError:
		return 3
	case model.SeverityWarning:
		return 2
	case model.SeverityInfo:
		return 1
	}
	return -1
}

func severityAspect(a model.Adapter, read model.ConfigRead, b model.Binding, e entry) aspect {
	if read.Entries.Severity == "" || b.Severity == "" {
		return aspect{}
	}
	want := severityRank(b.Severity)
	if want < 0 {
		return aspect{reason: fmt.Sprintf("the preset's severity %q is no severity of the engine", b.Severity)}
	}
	var got int
	switch {
	case slices.Contains(a.Severities.Off, e.Severity):
		got = 0
	case e.Severity == "":
		return aspect{reason: "the effective config gives the entry no severity"}
	default:
		i := slices.IndexFunc(a.Severities.Levels, func(l model.SeverityLevelName) bool { return l.Name == e.Severity })
		if i < 0 {
			return aspect{reason: fmt.Sprintf("the severity %q is no level and no off word of the adapter", e.Severity)}
		}
		got = severityRank(a.Severities.Levels[i].Is)
	}
	return aspect{rank: sign(got - want)}
}

func sign(n int) int {
	switch {
	case n < 0:
		return looser
	case n > 0:
		return tighter
	}
	return same
}

// stable is v as one text that equal values share: numbers by their value,
// object members in key order, text quoted. v is a value of the ordered
// package's parse, or a Go value of a binding.
func stable(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case string:
		return strconv.Quote(x)
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		// A whole binary64 inside int64's range is that whole number, so 2
		// and 2.0 are one value, and a whole number is compared exactly,
		// never through binary64 (Fable's remark 6 on 41bd64e9).
		if x >= -(1<<63) && x < 1<<63 && x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = stable(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *ordered.Map:
		return stable(ordered.Plain(x))
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = strconv.Quote(k) + ":" + stable(x[k])
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	return fmt.Sprintf("%T:%v", v, v)
}

// valueAspect is how the entry's value stands against the preset's: the same,
// or, where they differ, by the adapter's order of values, an off
// value below every value of the order. With no order and no off values, or a
// value in neither, it gives the reason and no rank.
//
// The entry's value is compared as the document holds it, typed, never its
// text read again: the text yes is text, not true, and
// "strict # x" keeps its comment. A value the config gives as null, or does
// not print, against a preset that sets one is its own case, with no word.
func valueAspect(scale model.ValueScale, read model.ConfigRead, b model.Binding, e entry) aspect {
	scale = scaleOf(scale, e.Key)
	if read.Entries.Value == "" && read.Entries.As == "" && read.Entries.Key != "" {
		return aspect{}
	}
	if b.Value == nil {
		// A config that sets the value to null where the preset sets none
		// is its own case: the tool's default for a null is the tool's, so
		// it is no word.
		if e.null {
			return aspect{reason: "the entry's value is null, and the preset sets no value, so what the tool does with a null is not known"}
		}
		// A binding with no value is on: it holds the entry, whatever its
		// value, unless the entry reads as off.
		if off(scale, e.value) {
			return aspect{rank: looser}
		}
		return aspect{}
	}
	if e.value == nil {
		return aspect{reason: "the entry's value is null or not printed, and the preset sets " + stable(b.Value) + ", so what the tool does with it is not known"}
	}
	want, got := stable(b.Value), stable(e.value)
	if got == want {
		return aspect{}
	}
	if len(scale.Off) == 0 && len(scale.Order) == 0 {
		return aspect{reason: "the value " + e.Value + " differs from the preset's " + want + ", and the adapter gives no order for values"}
	}
	wr, ok := valueRank(scale, want)
	if !ok {
		return aspect{reason: "the preset's value " + want + " is neither off nor in the adapter's order of values"}
	}
	gr, ok := valueRank(scale, got)
	if !ok {
		return aspect{reason: "the value " + e.Value + " is neither off nor in the adapter's order of values"}
	}
	return aspect{rank: sign(gr - wr)}
}

// valueRank is where a value, as stable text, stands in the scale: its place
// in the order, or below every place where it reads as off.
func valueRank(scale model.ValueScale, v string) (int, bool) {
	if slices.ContainsFunc(scale.Off, func(o any) bool { return stable(o) == v }) {
		return -1, true
	}
	i := slices.IndexFunc(scale.Order, func(o any) bool { return stable(o) == v })
	return i, i >= 0
}

// leaves is the set of leaves of a value, each scalar by its path. A path is
// a run of steps, each one way only: a key is "." and the key quoted, so a
// "/", "[]", "=" or quote in it is text; a list item is "[" and its index
// and "]", so every leaf names exactly one item of its list; "=" then opens
// the leaf's stable text. A map or list with nothing in it, below the root,
// is a leaf of its own, "={}" or "=[]", so a setting made empty is not one
// left out; at the root it is nothing, so empty
// options stand as none do. So two trees give one set of leaves only where
// they are one tree (a key holding / or [] read as nesting, so
// {"b/c": 1} was as-set against {b: {c: 1}}; and a list's items mixed, so a list of maps with fields swapped
// between its items, or [[1],[2]] against [[1,2]], was as-set). A list's
// order counts: that is the safe side, and a tool whose list is a set is for
// its adapter to say.
func leaves(path string, v any, into map[string]bool) {
	switch x := v.(type) {
	case *ordered.Map:
		leaves(path, ordered.Plain(x), into)
	case map[string]any:
		if len(x) == 0 && path != "" {
			into[path+"={}"] = true
		}
		for k, m := range x {
			leaves(path+"."+strconv.Quote(k), m, into)
		}
	case []any:
		if len(x) == 0 && path != "" {
			into[path+"=[]"] = true
		}
		for i, m := range x {
			leaves(path+"["+strconv.Itoa(i)+"]", m, into)
		}
	default:
		into[path+"="+stable(x)] = true
	}
}

func optionsAspect(read model.ConfigRead, b model.Binding, e entry) aspect {
	if read.Entries.Options == "" {
		return aspect{}
	}
	want := map[string]bool{}
	if len(b.Options) > 0 {
		leaves("", b.Options, want)
	}
	// The options as the document holds them, typed, never their text read
	// again.
	got := map[string]bool{}
	if e.options != nil {
		leaves("", e.options, got)
	}
	has := func(s, of map[string]bool) bool {
		for k := range s {
			if !of[k] {
				return false
			}
		}
		return true
	}
	switch {
	case len(want) == 0 && len(got) == 0, has(want, got) && has(got, want):
		return aspect{}
	case len(want) == 0 || len(got) == 0:
		return aspect{reason: "the options are set on one side only, and what the other side's default allows is the tool's"}
	case has(got, want):
		return aspect{rank: tighter}
	case has(want, got):
		return aspect{rank: looser}
	}
	return aspect{reason: "the options neither contain the preset's nor are contained in them"}
}

// off is whether the entry's value, as the config printed it, is one of the
// scale's off values.
func off(scale model.ValueScale, value any) bool {
	return value != nil && slices.ContainsFunc(scale.Off, func(o any) bool { return stable(o) == stable(value) })
}

// scaleOf is the scale that ranks the values of key: the key's own, where the
// adapter gives it one, and else the adapter's.
func scaleOf(scale model.ValueScale, key string) model.ValueScale {
	if i := slices.IndexFunc(scale.Keys, func(k model.KeyScale) bool { return k.Key == key }); i >= 0 {
		return model.ValueScale{Off: scale.Keys[i].Off, Order: scale.Keys[i].Order}
	}
	return model.ValueScale{Off: scale.Off, Order: scale.Order}
}
