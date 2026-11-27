// Package sample is the SDK's own sample plugin, written on Serve as a plugin author writes one: a made-up tool,
// tidy, that prints "key=value" lines and "path:line:col: key message" findings. It names no real tool. Its answers
// come out of order on purpose, so the SDK's rows show that the SDK leaves them sorted; and cmd/tidy is its main,
// which builds for wasip1 as the module a gragas "plugin test" runs.
package sample

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// Tidy is the sample plugin: it provides all five operations.
type Tidy struct{ plugin.Unsupported }

func (Tidy) Plan(r wire.PlanRequest) (wire.PlanResponse, error) {
	if r.For == wire.ForConfig {
		return wire.PlanResponse{Final: true}, nil
	}
	switch r.Round {
	case 0:
		return wire.PlanResponse{Steps: []wire.Step{{Read: &wire.Read{Glob: "tidy.cfg"}}}}, nil
	default:
		return wire.PlanResponse{Steps: []wire.Step{{Run: &wire.Run{Program: "tidy", Argv: []string{"--rules", "{scratch}/in"}}}}, Final: true}, nil
	}
}

// stdout is the text of the first run result in have.
func stdout(g wire.Gathered) string {
	for _, r := range g.Have {
		if r.Kind == wire.KindRun {
			return r.Stdout.Text()
		}
	}
	return ""
}

func (Tidy) Entries(r wire.EntriesRequest) (wire.EntriesResponse, error) {
	var out wire.EntriesResponse
	for _, line := range strings.Split(strings.TrimSpace(stdout(r.Gathered)), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return out, plugin.Unreadablef("/have/0/stdout", "the line %q has no =", line)
		}
		e := wire.Entry{Key: key, On: val != "off"}
		if n, err := strconv.Atoi(val); err == nil {
			e.Value = json.RawMessage(strconv.Itoa(n))
		}
		out.Entries = append(out.Entries, e)
	}
	return out, nil
}

func (Tidy) Findings(r wire.FindingsRequest) (wire.FindingsResponse, error) {
	out := wire.FindingsResponse{Ran: true}
	for _, line := range strings.Split(strings.TrimSpace(stdout(r.Gathered)), "\n") {
		if line == "" {
			continue
		}
		head, rest, _ := strings.Cut(line, ": ")
		parts := strings.Split(head, ":")
		key, msg, _ := strings.Cut(rest, " ")
		f := wire.Finding{Key: key, Message: msg, Path: parts[0]}
		if len(parts) == 3 {
			f.Line, _ = strconv.Atoi(parts[1])
			f.Column, _ = strconv.Atoi(parts[2])
		}
		out.Findings = append(out.Findings, f)
	}
	return out, nil
}

// Rank says that the lower number is tighter, as for a limit.
func (Tidy) Rank(r wire.RankRequest) (wire.RankResponse, error) {
	var out wire.RankResponse
	for _, p := range r.Pairs {
		var a, b int
		if json.Unmarshal(p.Preset, &a) != nil || json.Unmarshal(p.Found, &b) != nil {
			out.Words = append(out.Words, wire.Unknown)
			continue
		}
		switch {
		case b < a:
			out.Words = append(out.Words, wire.Tighter)
		case b > a:
			out.Words = append(out.Words, wire.Weaker)
		default:
			out.Words = append(out.Words, wire.Same)
		}
	}
	return out, nil
}

// Preset writes a file of the entries, the mark in its first line, and a second file, given out of order.
func (Tidy) Preset(r wire.PresetRequest) (wire.PresetResponse, error) {
	var b strings.Builder
	b.WriteString("# " + r.Mark + "\n")
	for _, e := range r.Entries {
		if string(e.Options) != "" && e.Key == "refuse-me" {
			return wire.PresetResponse{}, plugin.Refusedf("/entries/0/options", "tidy cannot write options for %s", e.Key)
		}
		b.WriteString(e.Key + "=on\n")
	}
	return wire.PresetResponse{Files: []wire.PresetFile{
		{Path: "z.tidy", Text: "# " + r.Mark + "\nz\n"},
		{Path: "a.tidy", Text: b.String()},
	}}, nil
}
