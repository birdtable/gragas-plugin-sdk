package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/birdtable/gragas-plugin-sdk/contract"
	"github.com/birdtable/gragas-plugin-sdk/pointer"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// The caps of the plugin contract: a whole request is at most 32 MiB and a response 16 MiB.
const (
	maxRequest  = 32 << 20
	maxResponse = 16 << 20
)

// versionForm is the form of a tool version on the wire: major, minor and patch, as numbers.
var versionForm = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// Serve runs impl for one call: it reads one request from stdin, calls the operation it names and writes one
// answer to stdout, one JSON document and one newline. A call is a fresh process image, so nothing survives it.
// It exits non-zero only when the answer could not be written.
func Serve(impl Plugin) {
	if code := ServeIO(os.Stdin, os.Stdout, impl); code != 0 {
		os.Exit(code)
	}
}

// ServeIO is Serve over any reader and writer, for a plugin's own tests: it answers a request given as bytes with
// the answer's bytes. It returns 0 when the answer was written, and 1 when it was not.
func ServeIO(in io.Reader, out io.Writer, impl Plugin) int {
	resp := respond(in, impl)
	b, err := encode(resp)
	if err != nil || len(b) > maxResponse {
		why := "the answer is over the 16 MiB a response may be"
		if err != nil {
			why = "the answer could not be written as JSON: " + err.Error()
		}
		b, _ = encode(wire.Response{Error: &wire.Error{Code: wire.CodeBug, Message: why}})
	}
	if _, err := out.Write(b); err != nil {
		return 1
	}
	return 0
}

// encode writes v as one JSON document and one newline, with no HTML escaping, so a message reads as it was
// written.
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fail is the answer of an error.
func fail(e *wire.Error) wire.Response { return wire.Response{Error: e} }

// bug is a request or an answer that cannot be read or kept, at path.
func bug(path, format string, args ...any) *wire.Error {
	return &wire.Error{Code: wire.CodeBug, Message: msg(format, args), Path: path}
}

// respond reads the request and answers it.
func respond(in io.Reader, impl Plugin) wire.Response {
	raw, err := io.ReadAll(io.LimitReader(in, maxRequest+1))
	if err != nil {
		return fail(bug("", "the request could not be read: %v", err))
	}
	if len(raw) > maxRequest {
		return fail(bug("", "the request is over the 32 MiB a request may be"))
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil || env == nil {
		return fail(bug("", "the request is not one JSON object"))
	}
	for _, k := range []string{"contract", "features", "op", "body"} {
		if v, ok := env[k]; !ok || isNull(v) {
			return fail(bug("/"+k, "the request has no %s", k))
		}
	}
	var number int
	if err := json.Unmarshal(env["contract"], &number); err != nil {
		return fail(bug("/contract", "the contract number is not an integer"))
	}
	var features []string
	if err := json.Unmarshal(env["features"], &features); err != nil {
		return fail(bug("/features", "the features are not a list of strings"))
	}
	var op string
	if err := json.Unmarshal(env["op"], &op); err != nil {
		return fail(bug("/op", "the operation is not a string"))
	}
	body := bytes.TrimSpace(env["body"])
	if len(body) == 0 || body[0] != '{' {
		return fail(bug("/body", "the body is not a JSON object"))
	}
	if number != contract.Version {
		return fail(&wire.Error{Code: wire.CodeUnsupported, Message: msg("this plugin is written for contract %d, and the request is for %d", []any{contract.Version, number})})
	}
	return dispatch(wire.Op(op), body, impl)
}

// isNull is whether v is the JSON null, which no field of the contract may be.
func isNull(v json.RawMessage) bool { return string(bytes.TrimSpace(v)) == "null" }

// dispatch decodes the body of op, calls the operation and finishes its answer.
func dispatch(op wire.Op, body json.RawMessage, impl Plugin) wire.Response {
	if !slices.Contains(wire.Ops(), op) {
		return fail(&wire.Error{Code: wire.CodeUnsupported, Message: msg("the operation %q is not one of contract 1", []any{string(op)})})
	}
	switch op {
	case wire.OpPlan:
		return run(op, body, impl.Plan, checkPlan, finishPlan)
	case wire.OpEntries:
		return run(op, body, impl.Entries, checkEntries, finishEntries)
	case wire.OpFindings:
		return run(op, body, impl.Findings, checkFindings, finishFindings)
	case wire.OpRank:
		return run(op, body, impl.Rank, checkRank, finishRank)
	default:
		return run(op, body, impl.Preset, checkPreset, finishPreset)
	}
}

// run decodes the request of one operation, checks it, calls the operation and finishes its answer.
func run[Req, Resp any](op wire.Op, body json.RawMessage, call func(Req) (Resp, error),
	check func(*Req, json.RawMessage) *wire.Error, finish func(*Req, *Resp) *wire.Error) (out wire.Response) {
	var req Req
	if err := json.Unmarshal(body, &req); err != nil {
		return fail(bug("/body", "the request body does not read: %v", err))
	}
	if e := check(&req, body); e != nil {
		return fail(e)
	}
	defer func() {
		if r := recover(); r != nil {
			out = fail(bug("", "the plugin panicked in %s: %v", op, r))
		}
	}()
	resp, err := call(req)
	if err != nil {
		return fail(codeFor(op, round(&req), err))
	}
	if at, bad := utf8Fault(resp); bad {
		return fail(bug(at, "a string of the answer is not valid UTF-8, and JSON would write each bad byte as U+FFFD and answer ok: such bytes go in a wire.Bytes"))
	}
	if e := finish(&req, &resp); e != nil {
		return fail(e)
	}
	b, err := json.Marshal(resp)
	if err != nil {
		return fail(bug("", "the answer could not be written as JSON: %v", err))
	}
	return wire.Response{OK: b}
}

// round is the round of a plan request, and 0 of any other.
func round(req any) int {
	if p, ok := req.(*wire.PlanRequest); ok {
		return p.Round
	}
	return 0
}

// codeFor is the error an operation answers: the plugin's own when its code is valid in that operation (design
// 4.4), and a bug that says so when it is not, since the engine would refuse it. An error that is not a
// *wire.Error is a bug.
func codeFor(op wire.Op, round int, err error) *wire.Error {
	e, ok := err.(*wire.Error)
	if !ok {
		return bug("", "%v", err)
	}
	valid := true
	switch e.Code {
	case wire.CodeUnsupported, wire.CodeBug:
	case wire.CodeUnreadable:
		valid = op == wire.OpEntries || op == wire.OpFindings || (op == wire.OpPlan && round > 0)
	case wire.CodeRefused:
		valid = op == wire.OpPlan || op == wire.OpPreset
	default:
		valid = false
	}
	if e.Message == "" {
		return bug("", "the plugin answered an error with no message in %s", op)
	}
	if !valid {
		return bug("", "the plugin answered %q in %s, which is not valid there: %s", string(e.Code), op, e.Message)
	}
	out := *e
	out.Message = clip(out.Message, 2048, "an error with no message")
	if _, err := pointer.Parse(out.Path); err != nil {
		return bug("", "the plugin's error has the path %q, which is not a JSON pointer (%v): the engine refuses it", e.Path, err)
	}
	if len(out.Path) > 1024 {
		// Cut at the last token that fits, never inside one: a cut inside ~1 would leave a path that ends in ~.
		out.Path = out.Path[:strings.LastIndex(out.Path[:1025], "/")]
	}
	return &out
}

// missing is the first of keys that the JSON object raw does not hold, or holds as null, and the bug that says
// so. Elements of a list are named by their pointer.
func missing(raw json.RawMessage, at string, keys ...string) *wire.Error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return bug(at, "%s is not a JSON object", pointerName(at))
	}
	for _, k := range keys {
		if v, ok := m[k]; !ok || isNull(v) && !valueKey(k) {
			return bug(at+"/"+k, "%s has no %s", pointerName(at), k)
		}
	}
	return nil
}

// valueKey is whether k names a value, in which a JSON null is a value and not an absence.
func valueKey(k string) bool { return k == "preset" || k == "found" || k == "value" || k == "options" }

func pointerName(at string) string {
	if at == "" {
		return "the body"
	}
	return fmt.Sprintf("the item at %s", at)
}

// gathered is the keys every request that carries what the engine gathered must hold.
var gathered = []string{"tool_version", "config", "have"}

func checkVersion(v string) *wire.Error {
	if !versionForm.MatchString(v) {
		return bug("/tool_version", "the tool version %q is not major.minor.patch", v)
	}
	return nil
}

func checkPlan(r *wire.PlanRequest, body json.RawMessage) *wire.Error {
	if e := missing(body, "", append([]string{"for", "target", "round"}, gathered...)...); e != nil {
		return e
	}
	if e := checkVersion(r.ToolVersion); e != nil {
		return e
	}
	if r.Round < 0 || r.Round > 2 {
		return bug("/round", "the round %d is not 0, 1 or 2", r.Round)
	}
	if !slices.Contains(wire.Fors(), r.For) {
		return &wire.Error{Code: wire.CodeUnsupported, Message: msg("the purpose %q is not config or run", []any{string(r.For)}), Path: "/for"}
	}
	return unknownTarget(r.Target)
}

func checkEntries(r *wire.EntriesRequest, body json.RawMessage) *wire.Error {
	if e := missing(body, "", append([]string{"target"}, gathered...)...); e != nil {
		return e
	}
	if e := checkVersion(r.ToolVersion); e != nil {
		return e
	}
	return unknownTarget(r.Target)
}

func checkFindings(r *wire.FindingsRequest, body json.RawMessage) *wire.Error {
	if e := missing(body, "", append([]string{"fixtures"}, gathered...)...); e != nil {
		return e
	}
	return checkVersion(r.ToolVersion)
}

func checkRank(r *wire.RankRequest, body json.RawMessage) *wire.Error {
	if e := missing(body, "", "tool_version", "pairs"); e != nil {
		return e
	}
	if e := checkVersion(r.ToolVersion); e != nil {
		return e
	}
	if len(r.Pairs) == 0 {
		return bug("/pairs", "rank is asked for no pairs")
	}
	var raws struct {
		Pairs []json.RawMessage `json:"pairs"`
	}
	if err := json.Unmarshal(body, &raws); err != nil {
		return bug("/pairs", "the pairs do not read: %v", err)
	}
	for i, raw := range raws.Pairs {
		if e := missing(raw, fmt.Sprintf("/pairs/%d", i), "key", "aspect", "preset", "found"); e != nil {
			return e
		}
	}
	for i, p := range r.Pairs {
		if !slices.Contains(wire.Aspects(), p.Aspect) {
			return &wire.Error{Code: wire.CodeUnsupported, Message: msg("the aspect %q is not value or options", []any{string(p.Aspect)}), Path: fmt.Sprintf("/pairs/%d/aspect", i)}
		}
	}
	return nil
}

func checkPreset(r *wire.PresetRequest, body json.RawMessage) *wire.Error {
	if e := missing(body, "", "tool_version", "entries", "inputs"); e != nil {
		return e
	}
	if e := checkVersion(r.ToolVersion); e != nil {
		return e
	}
	var raws struct {
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(body, &raws); err != nil {
		return bug("/entries", "the entries do not read: %v", err)
	}
	for i, raw := range raws.Entries {
		if e := missing(raw, fmt.Sprintf("/entries/%d", i), "key", "target", "trial", "rules"); e != nil {
			return e
		}
	}
	for i, en := range r.Entries {
		if len(en.Rules) == 0 {
			return bug(fmt.Sprintf("/entries/%d/rules", i), "an entry names no rule")
		}
		if e := unknownTarget(en.Target); e != nil {
			e.Path = fmt.Sprintf("/entries/%d/target", i)
			return e
		}
	}
	return nil
}

// unknownTarget is the unsupported answer for a target the contract does not name, and nil for one it does. A
// target the plugin knows but does not serve is the plugin's to answer.
func unknownTarget(t wire.Target) *wire.Error {
	if slices.Contains(wire.Targets(), t) {
		return nil
	}
	return &wire.Error{Code: wire.CodeUnsupported, Message: msg("the target %q is not one of contract 1", []any{string(t)}), Path: "/target"}
}
