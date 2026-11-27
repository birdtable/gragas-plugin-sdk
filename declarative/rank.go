package declarative

import (
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// Rank answers each pair by the moved comparison of Observe: equal values are same; otherwise the value is ranked by
// the adapter's order of values, an off value below every value of the order, the key's own scale where the adapter names
// one (scaleOf), found against the preset (tighter when found stands higher). A value that is null or absent, or in neither
// the order nor the off values, or an adapter that gives no order, is unknown, and so is every options pair that is not
// equal: no adapter orders options. This is conformance depth: it holds nothing.
func (p *Plugin) Rank(req wire.RankRequest) (wire.RankResponse, error) {
	out := wire.RankResponse{Words: make([]wire.Word, 0, len(req.Pairs))}
	for _, pair := range req.Pairs {
		out.Words = append(out.Words, p.word(pair))
	}
	return out, nil
}

func (p *Plugin) word(pair wire.Pair) wire.Word {
	want, err1 := decode(pair.Preset)
	got, err2 := decode(pair.Found)
	if err1 != nil || err2 != nil || want == nil || got == nil {
		return wire.Unknown
	}
	if stable(want) == stable(got) {
		return wire.Same
	}
	if pair.Aspect != wire.AspectValue {
		return wire.Unknown
	}
	scale := scaleOf(p.ad.Values, pair.Key)
	wr, ok := valueRank(scale, stable(want))
	if !ok {
		return wire.Unknown
	}
	gr, ok := valueRank(scale, stable(got))
	if !ok {
		return wire.Unknown
	}
	switch sign(gr - wr) {
	case tighter:
		return wire.Tighter
	case looser:
		return wire.Weaker
	}
	return wire.Same
}
