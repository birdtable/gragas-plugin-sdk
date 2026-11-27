package declarative

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/birdtable/gragas-plugin-sdk/declarative/internal/model"
)

// The grammar of a tool version range is closed and small: one comparator,
// or a lower bound and an upper bound joined by one space, each an operator
// (>=, >, <=, <, =) and a version major.minor.patch. Every comparator holds.
// There is no "or", no wildcard, no caret or tilde and no pre-release: a range
// a person cannot read at a glance is refused.

// ErrRange is a tool version range that does not parse or holds no version.
var ErrRange = errors.New("not a version range")

// ParseRange parses a tool version range. An error matches ErrRange and says
// why.
func ParseRange(s string) (model.VersionRange, error) {
	var vr model.VersionRange
	parts := strings.Split(s, " ")
	if s == "" || len(parts) > 2 {
		return vr, fmt.Errorf("%w: write one comparator, or a lower and an upper bound joined by one space", ErrRange)
	}
	var lower, upper, exact bool
	for _, p := range parts {
		op, v, err := comparator(p)
		if err != nil {
			return model.VersionRange{}, err
		}
		switch op {
		case "=":
			exact = true
			vr.Lower = model.VersionBound{Version: v, Included: true}
			vr.Upper = model.VersionBound{Version: v, Included: true}
		case ">=", ">":
			if lower {
				return model.VersionRange{}, fmt.Errorf("%w: two lower bounds", ErrRange)
			}
			lower = true
			vr.Lower = model.VersionBound{Version: v, Included: op == ">="}
		default:
			if upper {
				return model.VersionRange{}, fmt.Errorf("%w: two upper bounds", ErrRange)
			}
			upper = true
			vr.Upper = model.VersionBound{Version: v, Included: op == "<="}
		}
	}
	if exact && len(parts) > 1 {
		return model.VersionRange{}, fmt.Errorf("%w: = stands alone", ErrRange)
	}
	if lower && upper && !holdsAny(vr) {
		return model.VersionRange{}, fmt.Errorf("%w: no version is both %s and %s", ErrRange, parts[0], parts[1])
	}
	return vr, nil
}

// comparator splits one comparator into its operator and version.
func comparator(p string) (op, version string, err error) {
	for _, o := range []string{">=", "<=", ">", "<", "="} {
		if v, ok := strings.CutPrefix(p, o); ok {
			if !semverForm.MatchString(v) {
				return "", "", fmt.Errorf("%w: %q is no version major.minor.patch", ErrRange, v)
			}
			return o, v, nil
		}
	}
	return "", "", fmt.Errorf("%w: %q starts with none of >=, >, <=, <, =", ErrRange, p)
}

// holdsAny is whether a range with both bounds holds a version.
func holdsAny(vr model.VersionRange) bool {
	c := Compare(vr.Lower.Version, vr.Upper.Version)
	return c < 0 || c == 0 && vr.Lower.Included && vr.Upper.Included
}

// InRange is whether version, major.minor.patch, is in vr. A version of
// another form is an error matching ErrRange.
func InRange(vr model.VersionRange, version string) (bool, error) {
	if !semverForm.MatchString(version) {
		return false, fmt.Errorf("%w: %q is no version major.minor.patch", ErrRange, version)
	}
	if b := vr.Lower; b.Version != "" {
		if c := Compare(version, b.Version); c < 0 || c == 0 && !b.Included {
			return false, nil
		}
	}
	if b := vr.Upper; b.Version != "" {
		if c := Compare(version, b.Version); c > 0 || c == 0 && !b.Included {
			return false, nil
		}
	}
	return true, nil
}

// Compare orders two versions of the form major.minor.patch: -1, 0 or 1. The
// order is semver's, golang.org/x/mod/semver's.
func Compare(a, b string) int {
	return semver.Compare("v"+a, "v"+b)
}
