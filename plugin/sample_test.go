package plugin_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/plugin"
	"github.com/birdtable/gragas-plugin-sdk/sample"
	"github.com/birdtable/gragas-plugin-sdk/wire"
)

// tidy is the sample plugin of the module, the one the rows run.
type tidy = sample.Tidy

// call runs one request through ServeIO, and reads the answer's two shapes.
func call(t *testing.T, impl plugin.Plugin, request string) (out []byte, ok json.RawMessage, e *wire.Error) {
	t.Helper()
	var buf bytes.Buffer
	if code := plugin.ServeIO(strings.NewReader(request), &buf, impl); code != 0 {
		t.Fatalf("ServeIO returned %d", code)
	}
	var resp wire.Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("the answer %q is not a response: %v", buf.String(), err)
	}
	if (resp.OK == nil) == (resp.Error == nil) {
		t.Fatalf("the answer %q is not ok or error, one of the two", buf.String())
	}
	return buf.Bytes(), resp.OK, resp.Error
}

// req is a request of op with body, in contract 1.
func req(op, body string) string {
	return `{"contract":1,"features":[],"op":"` + op + `","body":` + body + `}`
}

const gathered = `"tool_version":"1.2.3","config":[],"have":[]`

// withStdout is a gathered body whose run printed out.
func withStdout(out string) string {
	b, _ := json.Marshal(out)
	return `"tool_version":"1.2.3","config":[],"have":[{"round":1,"step":0,"kind":"run","exit":0,"stdout":{"text":` + string(b) + `},"stderr":{"text":""},"cut":false,"files":[]}]`
}
