package splice_test

import (
	"errors"
	"testing"

	"github.com/birdtable/gragas-plugin-sdk/splice"
)

const (
	begin = "# generated:linters:begin"
	end   = "# generated:linters:end"
)

func TestSpliceReplacesWhatStandsBetweenAtTheBeginIndent(t *testing.T) {
	t.Parallel()
	text := "a: 1\n  enable:\n    # generated:linters:begin\n    - old\n    # generated:linters:end\nb: 2\n"
	got, err := splice.Splice([]byte(text), begin, end, []byte("- x\n\n- y\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "a: 1\n  enable:\n    # generated:linters:begin\n    - x\n\n    - y\n    # generated:linters:end\nb: 2\n"
	if string(got) != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	again, err := splice.Splice(got, begin, end, []byte("- x\n\n- y\n"))
	if err != nil || string(again) != want {
		t.Errorf("a second splice changed the text: %v\n%s", err, again)
	}
}

func TestSpliceWithAnEmptyBlockLeavesTheMarkersSideBySide(t *testing.T) {
	t.Parallel()
	got, err := splice.Splice([]byte(begin+"\nold\n"+end+"\n"), begin, end, nil)
	if err != nil || string(got) != begin+"\n"+end+"\n" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestSpliceRefusesNoMarker(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"neither":    "a\n",
		"no begin":   "a\n" + end + "\n",
		"no end":     begin + "\na\n",
		"not a line": "x " + begin + "\n" + end + "\n",
	} {
		if _, err := splice.Splice([]byte(text), begin, end, nil); !errors.Is(err, splice.ErrNoMarker) {
			t.Errorf("%s: err = %v, want ErrNoMarker", name, err)
		}
	}
}

func TestSpliceRefusesAnEndBeforeItsBegin(t *testing.T) {
	t.Parallel()
	_, err := splice.Splice([]byte(end+"\n"+begin+"\n"), begin, end, nil)
	if !errors.Is(err, splice.ErrMarkerOrder) {
		t.Errorf("err = %v, want ErrMarkerOrder", err)
	}
}

func TestSpliceRefusesAMarkerTwice(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"begin": begin + "\n" + begin + "\n" + end + "\n",
		"end":   begin + "\n" + end + "\n" + end + "\n",
	} {
		if _, err := splice.Splice([]byte(text), begin, end, nil); !errors.Is(err, splice.ErrMarkerTwice) {
			t.Errorf("%s: err = %v, want ErrMarkerTwice", name, err)
		}
	}
}

func TestSpliceFindsAMarkerLineThatEndsCRLF(t *testing.T) {
	t.Parallel()
	text := "a\r\n\t" + begin + "\r\nold\r\n" + end + "\r\nb\r\n"
	got, err := splice.Splice([]byte(text), begin, end, []byte("x\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "a\r\n\t" + begin + "\r\n\tx\n" + end + "\r\nb\r\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
