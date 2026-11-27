// Package sarif reads the SARIF a tool prints (SARIF 2.1.0, the OASIS static-analysis result format) into the
// findings of the contract. It holds the parts of a result a finding needs, and names no tool.
//
// The mapping: the rule's id is the key (the result's ruleId, else the id of the rule its ruleIndex names); the
// message is the result's message text, else its markdown; the path is the first location's artifact uri, with a
// file: scheme and percent escapes read and the scratch folder cut; line and column are the region's startLine
// and startColumn; the severity is the result's level, else the rule's default level, else warning, which is
// SARIF's own default: error is error, warning is warning, and note and none are info. A rule's helpUri is the
// finding's url.
package sarif

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/birdtable/gragas-plugin-sdk/scratch"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// Log is the part of a SARIF log a finding is read from.
type Log struct {
	Version string `json:"version"`
	Runs    []Run  `json:"runs"`
}

// Run is one run of a tool in a log.
type Run struct {
	Tool    Tool     `json:"tool"`
	Results []Result `json:"results"`
}

// Tool is the tool's driver, which holds its rules.
type Tool struct {
	Driver struct {
		Rules []Rule `json:"rules"`
	} `json:"driver"`
}

// Rule is a rule of the tool.
type Rule struct {
	ID                   string `json:"id"`
	HelpURI              string `json:"helpUri"`
	DefaultConfiguration struct {
		Level string `json:"level"`
	} `json:"defaultConfiguration"`
}

// Result is one result of a run: a finding.
type Result struct {
	RuleID    string `json:"ruleId"`
	RuleIndex *int   `json:"ruleIndex"`
	Level     string `json:"level"`
	Message   struct {
		Text     string `json:"text"`
		Markdown string `json:"markdown"`
	} `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine   int64 `json:"startLine"`
				StartColumn int64 `json:"startColumn"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
}

// Parse reads a SARIF log. It is an error for a document that is not JSON, one with no runs list, and one of a
// version that is not 2.1.0.
func Parse(data []byte) (Log, error) {
	var l Log
	if err := json.Unmarshal(data, &l); err != nil {
		return Log{}, fmt.Errorf("the SARIF does not read: %w", err)
	}
	if l.Version != "2.1.0" {
		return Log{}, fmt.Errorf("the SARIF is version %q, and 2.1.0 is the one read", l.Version)
	}
	if l.Runs == nil {
		return Log{}, errors.New("the SARIF has no runs")
	}
	return l, nil
}

// Findings are the findings of every result of every run, in the order of the log; the SDK sorts an answer. A
// result with no rule id, or no message, is an error that names it by its pointer, such as /runs/0/results/3, so a
// plugin answers unreadable at it. scratchDir is the scratch folder as the tool saw it, cut from each path; "" cuts
// nothing.
func (l Log) Findings(scratchDir string) ([]wire.Finding, error) {
	out := []wire.Finding{}
	for ri, run := range l.Runs {
		for i, r := range run.Results {
			at := fmt.Sprintf("/runs/%d/results/%d", ri, i)
			rule := run.rule(r)
			key := r.RuleID
			if key == "" && rule != nil {
				key = rule.ID
			}
			msg := r.Message.Text
			if msg == "" {
				msg = r.Message.Markdown
			}
			switch {
			case key == "":
				return nil, fmt.Errorf("%s has no rule id", at)
			case msg == "":
				return nil, fmt.Errorf("%s has no message", at)
			}
			f := wire.Finding{Key: clip(key, 512), Message: clip(msg, 8192), Severity: severity(r.Level, rule)}
			if len(r.Locations) > 0 {
				loc := r.Locations[0].PhysicalLocation
				f.Path = Path(loc.ArtifactLocation.URI, scratchDir)
				f.Line, f.Column = within(loc.Region.StartLine), within(loc.Region.StartColumn)
			}
			if rule != nil && len(rule.HelpURI) <= 2048 {
				f.URL = rule.HelpURI
			}
			out = append(out, f)
		}
	}
	return out, nil
}

// rule is the rule a result names by its ruleIndex, or by its ruleId, or nil.
func (run Run) rule(r Result) *Rule {
	rules := run.Tool.Driver.Rules
	if r.RuleIndex != nil && *r.RuleIndex >= 0 && *r.RuleIndex < len(rules) {
		return &rules[*r.RuleIndex]
	}
	for i := range rules {
		if r.RuleID != "" && rules[i].ID == r.RuleID {
			return &rules[i]
		}
	}
	return nil
}

// severity is the contract's word for a SARIF level: the result's, else the rule's default, else SARIF's own
// default, warning. note and none are info.
func severity(level string, rule *Rule) wire.Severity {
	if level == "" && rule != nil {
		level = rule.DefaultConfiguration.Level
	}
	switch level {
	case "error":
		return wire.SeverityError
	case "note", "none":
		return wire.SeverityInfo
	}
	return wire.SeverityWarning
}

// within is a line or a column the contract can hold, from 1 to 4294967295, or 0 for one it cannot. A SARIF file's number is
// read as an int64, so that one past the cap is read, and dropped, on a 32-bit target too, and not a file that does not parse.
// A 32-bit target also drops what its int cannot hold (from 2147483648 to the cap): the contract's whole range is not
// reachable there while a wire.Finding's Line is an int, which is a question for a later contract, and int(n) would wrap negative.
func within(n int64) int {
	if n < 1 || n > math.MaxUint32 || n > math.MaxInt {
		return 0
	}
	return int(n)
}

// Path is the path a SARIF uri names, for a finding: a file: scheme taken off, percent escapes read, a Windows
// drive's leading slash dropped, and the scratch folder cut. A uri that does not read is as it was.
func Path(uri, scratchDir string) string {
	p := uri
	if rest, ok := strings.CutPrefix(uri, "file://"); ok {
		p = rest
		if i := strings.IndexByte(p, '/'); i >= 0 && !strings.HasPrefix(p, "/") {
			p = p[i:] // a host before the path: file://host/a
		}
		if len(p) > 2 && p[0] == '/' && p[2] == ':' {
			p = p[1:] // /C:/a
		}
	}
	if un, err := url.PathUnescape(p); err == nil {
		p = un
	}
	return scratch.Path(p, scratchDir)
}

// clip is s cut to at most n bytes at a character boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
