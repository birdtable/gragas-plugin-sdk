package scratch_test

import (
	"strings"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/scratch"
)

func TestAPathBelowTheScratchFolderIsCutToTheProjectsOwn(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ scratch, in, want string }{
		{"/tmp/s1", "/tmp/s1/src/a.go", "src/a.go"},
		{"/tmp/s1/", "/tmp/s1/src/a.go", "src/a.go"},
		{"/tmp/s1", "/tmp/s1", "."},
		{"/tmp/s1", "/tmp/s1/", "."},
		{"/tmp/s1", "/tmp/s10/a.go", "/tmp/s10/a.go"},
		{"/tmp/s1", "/other/a.go", "/other/a.go"},
		{"/tmp/s1", "src/a.go", "src/a.go"},
		{`C:\s\run1`, `C:\s\run1\src\a.go`, `src\a.go`},
		{`C:\s\run1`, `C:\s\run1/src/a.go`, `src/a.go`},
		{"", "/tmp/s1/a.go", "/tmp/s1/a.go"},
		{"/", "/a.go", "/a.go"},
	} {
		if got := scratch.Path(c.in, c.scratch); got != c.want {
			t.Errorf("Path(%q, %q) = %q, want %q", c.in, c.scratch, got, c.want)
		}
	}
}

func TestTheScratchFolderIsCutFromPrintedTextWhereItIsAWholePath(t *testing.T) {
	t.Parallel()
	const s = "/tmp/s1"
	for _, c := range []struct{ in, want string }{
		{"/tmp/s1/src/a.go:3:1: bad", "src/a.go:3:1: bad"},
		{"error in /tmp/s1/src/a.go and /tmp/s1/b.go", "error in src/a.go and b.go"},
		{"in /tmp/s1: no files", "in .: no files"},
		{"in /tmp/s1/: no files", "in .: no files"},
		{"/tmp/s10/a.go", "/tmp/s10/a.go"},
		{"x/tmp/s1/a.go", "x/tmp/s1/a.go"},
		{"/tmp/s1/.hidden", ".hidden"},
		{"nothing here", "nothing here"},
		{"", ""},
		{"/tmp/s1/tmp/s1/a.go", "tmp/s1/a.go"},
	} {
		if got := scratch.Text(c.in, s); got != c.want {
			t.Errorf("Text(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := scratch.Text("/tmp/s1/a", ""); got != "/tmp/s1/a" {
		t.Errorf("an empty scratch cut %q", got)
	}
}

// FuzzText: no input panics, and a text that does not hold the folder is as it was.
func FuzzText(f *testing.F) {
	for _, s := range []string{"", "/tmp/s1", "/tmp/s1/a.go:3: x", "a /tmp/s1/ b", "/tmp/s1/tmp/s1", "\x00/tmp/s1\xff", `C:\s\run1\a`} {
		f.Add(s, "/tmp/s1")
		f.Add(s, `C:\s\run1`)
	}
	f.Fuzz(func(t *testing.T, s, dir string) {
		got := scratch.Text(s, dir)
		if strings.TrimRight(dir, `/\`) != "" && !strings.Contains(s, strings.TrimRight(dir, `/\`)) && got != s {
			t.Fatalf("Text(%q, %q) = %q: the folder is not in the text", s, dir, got)
		}
	})
}
