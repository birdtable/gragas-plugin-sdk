package pointer_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/pointer"
)

func TestATokenEscapesAndReadsBack(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"a": "a", "a/b": "a~1b", "a~b": "a~0b", "~/": "~0~1", "~01": "~001", "": ""} {
		got := pointer.Escape(in)
		back, err := pointer.Unescape(got)
		if got != want || err != nil || back != in {
			t.Errorf("Escape(%q) = %q, %q back, %v; want %q", in, got, back, err, want)
		}
	}
	for _, bad := range []string{"~", "a~", "~2", "~a"} {
		if _, err := pointer.Unescape(bad); err == nil {
			t.Errorf("Unescape(%q) was read", bad)
		}
	}
}

func TestAPointerIsJoinedAndParsedBack(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		tokens []string
		ptr    string
	}{
		{nil, ""},
		{[]string{"have", pointer.Index(0), "stdout"}, "/have/0/stdout"},
		{[]string{"a/b", "~"}, "/a~1b/~0"},
		{[]string{""}, "/"},
		{[]string{"", ""}, "//"},
	} {
		if got := pointer.Join(c.tokens...); got != c.ptr {
			t.Errorf("Join(%q) = %q, want %q", c.tokens, got, c.ptr)
		}
		back, err := pointer.Parse(c.ptr)
		if err != nil || !slices.Equal(back, c.tokens) {
			t.Errorf("Parse(%q) = %q, %v; want %q", c.ptr, back, err, c.tokens)
		}
	}
	for _, bad := range []string{"a", "a/b", "/~", "/a~2"} {
		if _, err := pointer.Parse(bad); err == nil {
			t.Errorf("Parse(%q) was read", bad)
		}
	}
}

func TestGetWalksObjectsAndLists(t *testing.T) {
	t.Parallel()
	var doc any
	if err := json.Unmarshal([]byte(`{"have":[{"stdout":{"text":"x"}}],"list":[10,20,30],"a/b":1,"m~n":2,"":3}`), &doc); err != nil {
		t.Fatal(err)
	}
	for ptr, want := range map[string]any{"/list/1": 20.0, "/a~1b": 1.0, "/m~0n": 2.0, "/": 3.0, "/have/0/stdout/text": "x"} {
		if got, ok := pointer.Get(doc, ptr); !ok || got != want {
			t.Errorf("Get(%q) = %v, %v; want %v", ptr, got, ok, want)
		}
	}
	if whole, ok := pointer.Get(doc, ""); !ok || whole == nil {
		t.Errorf("Get of the whole document = %v, %v", whole, ok)
	}
	for _, none := range []string{"/nope", "/have/1", "/list/01", "/list/+1", "/list/1.0", "/list/ 1", "/list/3", "/have/-1", "/have/x", "/have/0/stdout/text/y", "bad", "/~2"} {
		if got, ok := pointer.Get(doc, none); ok {
			t.Errorf("Get(%q) = %v, want nothing", none, got)
		}
	}
}

// FuzzParse: a pointer that parses is the join of its tokens, byte for byte, and no input panics. Seeds are the
// shapes of the contract's own paths.
func FuzzParse(f *testing.F) {
	for _, s := range []string{"", "/", "//", "/have/0/stdout", "/entries/3/options", "/a~1b/~0", "/~", "x", "/\x00", "/é/\U0001F600"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, ptr string) {
		tokens, err := pointer.Parse(ptr)
		if err != nil {
			return
		}
		if got := pointer.Join(tokens...); got != ptr {
			t.Fatalf("Parse(%q) = %q, and Join gives %q", ptr, tokens, got)
		}
	})
}
